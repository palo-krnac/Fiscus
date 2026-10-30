package backup

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// S3 ukladá zálohy do bucketu kompatibilného s Amazon S3 (AWS, Wasabi,
// Backblaze B2, MinIO, Cloudflare R2…). Požiadavky podpisuje AWS
// Signature V4; adresovanie je path-style (endpoint/bucket/kľúč).
type S3 struct {
	Endpoint  string // https://s3.eu-central-1.amazonaws.com
	Bucket    string
	Region    string
	Prefix    string // priečinok v buckete bez lomiek na okrajoch
	AccessKey string
	SecretKey string
	Client    *http.Client
	Now       func() time.Time
}

func (s *S3) key(name string) string {
	if s.Prefix == "" {
		return name
	}
	return s.Prefix + "/" + name
}

// s3Escape kóduje časť cesty podľa pravidiel SigV4 (RFC 3986, '/' ostáva).
func s3Escape(p string, keepSlash bool) string {
	var b strings.Builder
	for i := 0; i < len(p); i++ {
		c := p[i]
		switch {
		case 'A' <= c && c <= 'Z', 'a' <= c && c <= 'z', '0' <= c && c <= '9', c == '-', c == '_', c == '.', c == '~':
			b.WriteByte(c)
		case c == '/' && keepSlash:
			b.WriteByte(c)
		default:
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

func hmacSHA256(key []byte, data string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(data))
	return h.Sum(nil)
}

func sha256Hex(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// sign podpíše požiadavku (AWS Signature Version 4, služba s3).
func (s *S3) sign(req *http.Request, payload []byte, now time.Time) {
	now = now.UTC()
	amzDate := now.Format("20060102T150405Z")
	day := now.Format("20060102")
	payloadHash := sha256Hex(payload)
	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)

	// kanonické hlavičky: host + x-amz-*
	hdrs := map[string]string{"host": req.URL.Host}
	for k, v := range req.Header {
		lk := strings.ToLower(k)
		if strings.HasPrefix(lk, "x-amz-") || lk == "content-type" {
			hdrs[lk] = strings.TrimSpace(strings.Join(v, ","))
		}
	}
	keys := make([]string, 0, len(hdrs))
	for k := range hdrs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var canonHdr strings.Builder
	for _, k := range keys {
		canonHdr.WriteString(k + ":" + hdrs[k] + "\n")
	}
	signed := strings.Join(keys, ";")

	// kanonický query string
	q := req.URL.Query()
	qk := make([]string, 0, len(q))
	for k := range q {
		qk = append(qk, k)
	}
	sort.Strings(qk)
	var qs []string
	for _, k := range qk {
		vals := q[k]
		sort.Strings(vals)
		for _, v := range vals {
			qs = append(qs, s3Escape(k, false)+"="+s3Escape(v, false))
		}
	}

	canonical := strings.Join([]string{
		req.Method,
		s3Escape(req.URL.Path, true),
		strings.Join(qs, "&"),
		canonHdr.String(),
		signed,
		payloadHash,
	}, "\n")
	scope := day + "/" + s.Region + "/s3/aws4_request"
	toSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + scope + "\n" + sha256Hex([]byte(canonical))
	k := hmacSHA256([]byte("AWS4"+s.SecretKey), day)
	k = hmacSHA256(k, s.Region)
	k = hmacSHA256(k, "s3")
	k = hmacSHA256(k, "aws4_request")
	sig := hex.EncodeToString(hmacSHA256(k, toSign))
	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+s.AccessKey+"/"+scope+", SignedHeaders="+signed+", Signature="+sig)
}

func (s *S3) do(ctx context.Context, method, key string, query url.Values, body []byte) (*http.Response, error) {
	u, err := url.Parse(s.Endpoint)
	if err != nil {
		return nil, err
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/" + s.Bucket
	if key != "" {
		u.Path += "/" + key
	}
	u.RawPath = s3Escape(u.Path, true)
	u.RawQuery = query.Encode()
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), r)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/octet-stream")
	}
	now := time.Now
	if s.Now != nil {
		now = s.Now
	}
	s.sign(req, body, now())
	resp, err := s.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("S3 %s: %w", method, err)
	}
	return resp, nil
}

// Put nahrá objekt.
func (s *S3) Put(ctx context.Context, name string, data []byte) error {
	if data == nil {
		data = []byte{}
	}
	resp, err := s.do(ctx, http.MethodPut, s.key(name), nil, data)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return httpError("S3 nahratie", resp)
	}
	return nil
}

// Get stiahne objekt.
func (s *S3) Get(ctx context.Context, name string) ([]byte, error) {
	resp, err := s.do(ctx, http.MethodGet, s.key(name), nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, httpError("S3 stiahnutie", resp)
	}
	return io.ReadAll(resp.Body)
}

// Delete zmaže objekt.
func (s *S3) Delete(ctx context.Context, name string) error {
	resp, err := s.do(ctx, http.MethodDelete, s.key(name), nil, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 && resp.StatusCode != http.StatusNotFound {
		return httpError("S3 zmazanie", resp)
	}
	return nil
}

type s3List struct {
	Contents []struct {
		Key  string `xml:"Key"`
		Size int64  `xml:"Size"`
	} `xml:"Contents"`
	IsTruncated           bool   `xml:"IsTruncated"`
	NextContinuationToken string `xml:"NextContinuationToken"`
}

// List vráti zálohy v priečinku (ListObjectsV2, po stránkach).
func (s *S3) List(ctx context.Context) ([]Info, error) {
	prefix := ""
	if s.Prefix != "" {
		prefix = s.Prefix + "/"
	}
	names := map[string]int64{}
	token := ""
	for page := 0; page < 100; page++ {
		q := url.Values{"list-type": {"2"}, "prefix": {prefix + filePrefix}}
		if token != "" {
			q.Set("continuation-token", token)
		}
		resp, err := s.do(ctx, http.MethodGet, "", q, nil)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode/100 != 2 {
			err := httpError("S3 zoznam", resp)
			resp.Body.Close()
			return nil, err
		}
		var l s3List
		err = xml.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(&l)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("S3 zoznam: nečitateľná odpoveď: %w", err)
		}
		for _, c := range l.Contents {
			n := strings.TrimPrefix(c.Key, prefix)
			if !strings.Contains(n, "/") {
				names[n] = c.Size
			}
		}
		if !l.IsTruncated || l.NextContinuationToken == "" {
			break
		}
		token = l.NextContinuationToken
	}
	return remoteBackups(names), nil
}

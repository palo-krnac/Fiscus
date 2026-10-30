package backup

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeCloud je falošný server WebDAV aj S3 nad jednou mapou súborov.
type fakeCloud struct {
	mu     sync.Mutex
	files  map[string][]byte // kľúč = cesta bez úvodnej lomky
	auth   func(*http.Request) bool
	failOn string // metóda, ktorá vráti 500
}

func newFakeCloud() *fakeCloud { return &fakeCloud{files: map[string][]byte{}} }

func (f *fakeCloud) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.auth != nil && !f.auth(r) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, "zlé údaje pre "+r.Header.Get("Authorization")) // telo sa nesmie dostať do chyby
		return
	}
	if r.Method == f.failOn {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	key := strings.TrimPrefix(r.URL.Path, "/")
	switch r.Method {
	case http.MethodPut:
		b, _ := io.ReadAll(r.Body)
		f.files[key] = b
		w.WriteHeader(http.StatusCreated)
	case http.MethodGet:
		if r.URL.Query().Get("list-type") == "2" { // S3 ListObjectsV2
			f.s3List(w, key, r.URL.Query().Get("prefix"))
			return
		}
		b, ok := f.files[key]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write(b)
	case http.MethodDelete:
		if _, ok := f.files[key]; !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		delete(f.files, key)
		w.WriteHeader(http.StatusNoContent)
	case "PROPFIND":
		f.propfind(w, key)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (f *fakeCloud) propfind(w http.ResponseWriter, dir string) {
	dir = strings.TrimSuffix(dir, "/") + "/"
	var b strings.Builder
	b.WriteString(`<?xml version="1.0"?><d:multistatus xmlns:d="DAV:">`)
	fmt.Fprintf(&b, `<d:response><d:href>/%s</d:href><d:propstat><d:prop><d:resourcetype><d:collection/></d:resourcetype></d:prop></d:propstat></d:response>`, dir)
	for k, v := range f.files {
		if strings.HasPrefix(k, dir) && !strings.Contains(strings.TrimPrefix(k, dir), "/") {
			fmt.Fprintf(&b, `<d:response><d:href>/%s</d:href><d:propstat><d:prop><d:getcontentlength>%d</d:getcontentlength><d:resourcetype/></d:prop></d:propstat></d:response>`,
				url.PathEscape(dir)+url.PathEscape(strings.TrimPrefix(k, dir)), len(v))
		}
	}
	b.WriteString(`</d:multistatus>`)
	w.WriteHeader(http.StatusMultiStatus)
	fmt.Fprint(w, b.String())
}

func (f *fakeCloud) s3List(w http.ResponseWriter, bucket, prefix string) {
	type obj struct {
		Key  string
		Size int
	}
	var keys []string
	for k := range f.files {
		if strings.HasPrefix(k, bucket+"/"+prefix) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	out := struct {
		XMLName     xml.Name `xml:"ListBucketResult"`
		Contents    []obj
		IsTruncated bool
	}{}
	for _, k := range keys {
		out.Contents = append(out.Contents, obj{Key: strings.TrimPrefix(k, bucket+"/"), Size: len(f.files[k])})
	}
	_ = xml.NewEncoder(w).Encode(out)
}

func (f *fakeCloud) names() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var n []string
	for k := range f.files {
		n = append(n, k)
	}
	sort.Strings(n)
	return n
}

// remoteCase pripraví falošný server a nastavenie pre daný druh cloudu.
type remoteCase struct {
	name   string
	config func(base string) RemoteConfig
	dir    string // kde server ukladá súbory
}

var remoteCases = []remoteCase{
	{"WebDAV", func(base string) RemoteConfig {
		return RemoteConfig{Kind: RemoteWebDAV, URL: base + "/dav/Fiškus zálohy", User: "jana", Secret: "tajne-heslo"}
	}, "dav/Fiškus zálohy/"},
	{"S3", func(base string) RemoteConfig {
		return RemoteConfig{Kind: RemoteS3, URL: base, User: "AKIAEXAMPLE", Secret: "tajny-kluc", Bucket: "firma", Region: "eu-central-1", Prefix: "/fiskus/"}
	}, "firma/fiskus/"},
}

func authFor(c RemoteConfig) func(*http.Request) bool {
	return func(r *http.Request) bool {
		if c.Kind == RemoteWebDAV {
			u, p, ok := r.BasicAuth()
			return ok && u == c.User && p == c.Secret
		}
		a := r.Header.Get("Authorization")
		return strings.HasPrefix(a, "AWS4-HMAC-SHA256 Credential="+c.User+"/") && r.Header.Get("X-Amz-Content-Sha256") != ""
	}
}

func TestRemoteRoundTrip(t *testing.T) {
	ctx := context.Background()
	for _, tc := range remoteCases {
		t.Run(tc.name, func(t *testing.T) {
			fc := newFakeCloud()
			srv := httptest.NewServer(fc)
			defer srv.Close()
			c := tc.config(srv.URL)
			fc.auth = authFor(c)
			r, err := NewRemote(c, srv.Client())
			if err != nil {
				t.Fatal(err)
			}
			if err := TestRemote(ctx, r); err != nil {
				t.Fatalf("skúška spojenia: %v", err)
			}
			if n := fc.names(); len(n) != 0 {
				t.Fatalf("skúšobný súbor ostal v cloude: %v", n)
			}

			// 10 denných záloh, politika ponechá 3 dni
			start := time.Date(2026, 10, 1, 18, 0, 0, 0, time.UTC)
			var n int
			for i := 0; i < 10; i++ {
				name := filePrefix + start.AddDate(0, 0, i).Format(timeLayout) + fileExt
				if n, err = SyncRemote(ctx, r, Policy{Daily: 3}, name, []byte("záloha "+name)); err != nil {
					t.Fatal(err)
				}
			}
			if n != 3 {
				t.Errorf("v cloude %d záloh, čakám 3", n)
			}
			_ = r.Put(ctx, "cudzi-subor.txt", []byte("x")) // cudzie súbory sa nemažú ani nevypisujú
			list, err := r.List(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if len(list) != 3 || list[0].Path != filePrefix+"20261010-180000"+fileExt || list[0].Size == 0 {
				t.Fatalf("zoznam: %+v", list)
			}
			got, err := r.Get(ctx, list[2].Path)
			if err != nil || string(got) != "záloha "+list[2].Path {
				t.Fatalf("stiahnutie: %q %v", got, err)
			}
			for _, k := range fc.names() {
				if !strings.HasPrefix(k, tc.dir) {
					t.Errorf("súbor %s mimo priečinka %s", k, tc.dir)
				}
			}
			if !contains(fc.names(), tc.dir+"cudzi-subor.txt") {
				t.Error("cudzí súbor bol zmazaný")
			}
		})
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func TestRemoteErrorsHideSecrets(t *testing.T) {
	ctx := context.Background()
	for _, tc := range remoteCases {
		t.Run(tc.name, func(t *testing.T) {
			fc := newFakeCloud()
			srv := httptest.NewServer(fc)
			defer srv.Close()
			c := tc.config(srv.URL)
			fc.auth = func(*http.Request) bool { return false }
			r, _ := NewRemote(c, srv.Client())
			err := TestRemote(ctx, r)
			if err == nil || !strings.Contains(err.Error(), "prístup zamietnutý") {
				t.Fatalf("čakám zamietnutie: %v", err)
			}
			if strings.Contains(err.Error(), c.Secret) || strings.Contains(err.Error(), "Authorization") || strings.Contains(err.Error(), "zlé údaje") {
				t.Errorf("chyba prezrádza údaje: %v", err)
			}
		})
	}
}

func TestRemoteValidate(t *testing.T) {
	ok := RemoteConfig{Kind: RemoteS3, URL: "https://s3.eu-central-1.amazonaws.com", User: "a", Secret: "b", Bucket: "c", Region: "eu-central-1"}
	tests := []struct {
		name string
		mod  func(*RemoteConfig)
		want string // "" = platné
	}{
		{"S3 platné", func(*RemoteConfig) {}, ""},
		{"WebDAV platné", func(c *RemoteConfig) {
			*c = RemoteConfig{Kind: RemoteWebDAV, URL: "https://cloud.firma.sk/remote.php/dav/files/jana/", User: "j", Secret: "h"}
		}, ""},
		{"localhost cez http", func(c *RemoteConfig) { c.URL = "http://127.0.0.1:9000" }, ""},
		{"neznámy druh", func(c *RemoteConfig) { c.Kind = "ftp" }, "vyberte"},
		{"http mimo localhost", func(c *RemoteConfig) { c.URL = "http://s3.example.com" }, "https://"},
		{"heslo v adrese", func(c *RemoteConfig) { c.URL = "https://jana:tajne@cloud.sk/dav" }, "nesmie obsahovať"},
		{"bez hostiteľa", func(c *RemoteConfig) { c.URL = "https://" }, "https://"},
		{"bez kľúča", func(c *RemoteConfig) { c.Secret = "" }, "prihlasovacie"},
		{"S3 bez bucketu", func(c *RemoteConfig) { c.Bucket = "" }, "bucket"},
		{"S3 bez regiónu", func(c *RemoteConfig) { c.Region = "" }, "región"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := ok
			tt.mod(&c)
			err := c.Validate()
			if tt.want == "" {
				if err != nil {
					t.Fatalf("neočakávaná chyba: %v", err)
				}
				return
			}
			if !errors.Is(err, ErrRemoteConfig) || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("chyba %v, čakám %q", err, tt.want)
			}
		})
	}
}

// TestS3SignatureV4 overuje podpis na vzoroch z dokumentácie Amazon S3
// („Signature Calculations for the Authorization Header").
func TestS3SignatureV4(t *testing.T) {
	s := &S3{Region: "us-east-1", AccessKey: "AKIAIOSFODNN7EXAMPLE", SecretKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"}
	now := time.Date(2013, 5, 24, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name, url, sig string
	}{
		{"GET Bucket Lifecycle", "https://examplebucket.s3.amazonaws.com/?lifecycle",
			"fea454ca298b7da1c68078a5d1bdbfbbe0d65c699e0f91ac7a200a0136783543"},
		{"GET Bucket (List Objects)", "https://examplebucket.s3.amazonaws.com/?max-keys=2&prefix=J",
			"34b48302e7b5fa45bde8084f4b7868a86f0a534bc59db6670ed5711ef69dc6f7"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest(http.MethodGet, tt.url, nil)
			s.sign(req, nil, now)
			a := req.Header.Get("Authorization")
			want := "AWS4-HMAC-SHA256 Credential=AKIAIOSFODNN7EXAMPLE/20130524/us-east-1/s3/aws4_request, SignedHeaders=host;x-amz-content-sha256;x-amz-date, Signature=" + tt.sig
			if a != want {
				t.Errorf("podpis\n got %s\nwant %s", a, want)
			}
		})
	}
}

func TestS3Escape(t *testing.T) {
	tests := []struct{ in, want string }{
		{"/firma/fiskus/fiskus-20261029-180000.fbk", "/firma/fiskus/fiskus-20261029-180000.fbk"},
		{"/firma/Fiškus zálohy/a+b", "/firma/Fi%C5%A1kus%20z%C3%A1lohy/a%2Bb"},
	}
	for _, tt := range tests {
		if got := s3Escape(tt.in, true); got != tt.want {
			t.Errorf("s3Escape(%q) = %q, čakám %q", tt.in, got, tt.want)
		}
	}
}

func TestManagerUploadsAndFetches(t *testing.T) {
	ctx := context.Background()
	for _, tc := range remoteCases {
		t.Run(tc.name, func(t *testing.T) {
			fc := newFakeCloud()
			srv := httptest.NewServer(fc)
			defer srv.Close()
			c := tc.config(srv.URL)
			fc.auth = authFor(c)

			src := &fakeSource{content: []byte("DB:firma")}
			m := &Manager{Dir: filepath.Join(t.TempDir(), "zalohy"), Password: "heslo-zaloh", Policy: DefaultPolicy, Source: src, Client: srv.Client()}
			m.SetRemote(c)
			now := time.Date(2026, 10, 29, 18, 0, 0, 0, time.UTC)
			b, err := m.Create(ctx, now)
			if err != nil {
				t.Fatal(err)
			}
			name := filepath.Base(b.Path)
			if !contains(fc.names(), tc.dir+name) {
				t.Fatalf("záloha nie je v cloude: %v", fc.names())
			}
			if st := m.RemoteState(); st.LastName != name || st.Count != 1 || st.LastError != "" {
				t.Errorf("stav cloudu: %+v", st)
			}

			// nový počítač: prázdny priečinok, rovnaké heslo a cloud
			m2 := &Manager{Dir: filepath.Join(t.TempDir(), "nove"), Password: "heslo-zaloh", Policy: DefaultPolicy, Source: src, Client: srv.Client()}
			m2.SetRemote(c)
			got, err := m2.FetchRemote(ctx, name)
			if err != nil {
				t.Fatal(err)
			}
			if got.Time != now || got.Size != b.Size {
				t.Errorf("stiahnutá záloha: %+v", got)
			}
			if _, err := m2.Find(name); err != nil {
				t.Errorf("stiahnutá záloha nie je v zozname: %v", err)
			}
			// zlé heslo: záloha sa nestiahne
			m3 := &Manager{Dir: t.TempDir(), Password: "ine-heslo", Client: srv.Client()}
			m3.SetRemote(c)
			if _, err := m3.FetchRemote(ctx, name); !errors.Is(err, ErrBadPassword) {
				t.Errorf("zlé heslo: %v", err)
			}
			for _, bad := range []string{"../" + name, "iny.txt", "fiskus-zle.fbk"} {
				if _, err := m2.FetchRemote(ctx, bad); !errors.Is(err, ErrNoBackup) {
					t.Errorf("názov %q: %v", bad, err)
				}
			}

			// výpadok cloudu: lokálna záloha je hotová, chyba sa vráti a zapamätá
			fc.mu.Lock()
			fc.failOn = http.MethodPut
			fc.mu.Unlock()
			b2, err := m.Create(ctx, now.Add(time.Hour))
			if !errors.Is(err, ErrRemoteCopy) || b2.Path == "" {
				t.Fatalf("výpadok cloudu: %+v %v", b2, err)
			}
			if _, err := m.Find(filepath.Base(b2.Path)); err != nil {
				t.Errorf("lokálna záloha chýba: %v", err)
			}
			if st := m.RemoteState(); st.LastError == "" || st.LastName != name {
				t.Errorf("stav po výpadku: %+v", st)
			}
		})
	}
}

func TestManagerWithoutRemote(t *testing.T) {
	m := &Manager{Dir: t.TempDir(), Password: "h", Policy: DefaultPolicy, Source: &fakeSource{content: []byte("DB:x")}}
	if _, err := m.Create(context.Background(), time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := m.FetchRemote(context.Background(), filePrefix+"20261029-180000"+fileExt); !errors.Is(err, ErrNoRemote) {
		t.Errorf("bez cloudu: %v", err)
	}
}

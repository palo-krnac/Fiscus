package backup

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
)

// WebDAV ukladá zálohy do priečinka WebDAV (Nextcloud, ownCloud, Synology,
// Disk Google cez bránu a pod.). Base je URL priečinka končiaca lomkou.
type WebDAV struct {
	Base     string
	User     string
	Password string
	Client   *http.Client
}

func (w *WebDAV) do(ctx context.Context, method, name string, body []byte, hdr map[string]string) (*http.Response, error) {
	u := w.Base + url.PathEscape(name)
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, r)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(w.User, w.Password)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := w.Client.Do(req)
	if err != nil {
		// chyba môže obsahovať URL, ale nie heslo (to ide v hlavičke)
		return nil, fmt.Errorf("WebDAV %s: %w", method, err)
	}
	return resp, nil
}

// Put nahrá súbor (prepíše existujúci).
func (w *WebDAV) Put(ctx context.Context, name string, data []byte) error {
	resp, err := w.do(ctx, http.MethodPut, name, data, map[string]string{"Content-Type": "application/octet-stream"})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return httpError("WebDAV nahratie", resp)
	}
	return nil
}

// Get stiahne súbor.
func (w *WebDAV) Get(ctx context.Context, name string) ([]byte, error) {
	resp, err := w.do(ctx, http.MethodGet, name, nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, httpError("WebDAV stiahnutie", resp)
	}
	return io.ReadAll(resp.Body)
}

// Delete zmaže súbor; neexistujúci súbor nie je chyba.
func (w *WebDAV) Delete(ctx context.Context, name string) error {
	resp, err := w.do(ctx, http.MethodDelete, name, nil, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 && resp.StatusCode != http.StatusNotFound {
		return httpError("WebDAV zmazanie", resp)
	}
	return nil
}

const propfindBody = `<?xml version="1.0" encoding="utf-8"?><d:propfind xmlns:d="DAV:"><d:prop><d:getcontentlength/><d:resourcetype/></d:prop></d:propfind>`

type davMultistatus struct {
	Responses []struct {
		Href     string `xml:"href"`
		Propstat []struct {
			Prop struct {
				Length       string `xml:"getcontentlength"`
				ResourceType struct {
					Collection *struct{} `xml:"collection"`
				} `xml:"resourcetype"`
			} `xml:"prop"`
		} `xml:"propstat"`
	} `xml:"response"`
}

// List vráti zálohy v priečinku (PROPFIND s hĺbkou 1).
func (w *WebDAV) List(ctx context.Context) ([]Info, error) {
	resp, err := w.do(ctx, "PROPFIND", "", []byte(propfindBody), map[string]string{"Depth": "1", "Content-Type": "application/xml; charset=utf-8"})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMultiStatus && resp.StatusCode/100 != 2 {
		return nil, httpError("WebDAV zoznam", resp)
	}
	var ms davMultistatus
	if err := xml.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(&ms); err != nil {
		return nil, fmt.Errorf("WebDAV zoznam: nečitateľná odpoveď: %w", err)
	}
	names := map[string]int64{}
	for _, r := range ms.Responses {
		h, err := url.PathUnescape(r.Href)
		if err != nil {
			h = r.Href
		}
		h = strings.TrimRight(h, "/")
		var size int64
		dir := false
		for _, ps := range r.Propstat {
			if ps.Prop.ResourceType.Collection != nil {
				dir = true
			}
			if n, err := strconv.ParseInt(strings.TrimSpace(ps.Prop.Length), 10, 64); err == nil {
				size = n
			}
		}
		if !dir {
			names[path.Base(h)] = size
		}
	}
	return remoteBackups(names), nil
}

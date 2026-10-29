package postman

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// HTTP je všeobecné API pre adaptér poskytovateľa (jednoduchá zmluva, ktorú
// adaptér preloží na API konkrétneho poštára):
//
//	POST {base}/documents            telo: UBL XML; hlavičky X-Peppol-Sender, X-Peppol-Receiver,
//	                                 X-File-Name → {"messageId": "…", "status": "…"}
//	GET  {base}/inbox                → [{"id": "…", "name": "…", "received": "RFC 3339"}]
//	GET  {base}/inbox/{id}           → UBL XML
//	POST {base}/inbox/{id}/ack       → 204
//
// Autorizácia: Authorization: Bearer <token>.
type HTTP struct {
	BaseURL string
	Token   string
	Client  *http.Client
}

func (h HTTP) client() *http.Client {
	if h.Client != nil {
		return h.Client
	}
	return &http.Client{Timeout: 60 * time.Second}
}

func (h HTTP) do(ctx context.Context, method, path string, body []byte, hdr map[string]string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, h.BaseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+h.Token)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	res, err := h.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("poštár neodpovedá: %w", err)
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, 20<<20))
	if err != nil {
		return nil, err
	}
	if res.StatusCode >= 300 {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(data, &e)
		if e.Error == "" {
			e.Error = res.Status
		}
		return nil, fmt.Errorf("poštár: %s", e.Error)
	}
	return data, nil
}

func (h HTTP) Send(ctx context.Context, o Outgoing) (Sent, error) {
	data, err := h.do(ctx, http.MethodPost, "/documents", o.Data, map[string]string{
		"Content-Type": "application/xml", "X-Peppol-Sender": o.Sender, "X-Peppol-Receiver": o.Receiver, "X-File-Name": o.FileName,
	})
	if err != nil {
		return Sent{}, err
	}
	var s Sent
	if err := json.Unmarshal(data, &s); err != nil || s.MessageID == "" {
		return Sent{}, fmt.Errorf("poštár: neplatná odpoveď na odoslanie")
	}
	return s, nil
}

func (h HTTP) Inbox(ctx context.Context) ([]Message, error) {
	data, err := h.do(ctx, http.MethodGet, "/inbox", nil, nil)
	if err != nil {
		return nil, err
	}
	var list []Message
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("poštár: neplatný zoznam prijatých")
	}
	for i := range list {
		if list[i].Data, err = h.do(ctx, http.MethodGet, "/inbox/"+url.PathEscape(list[i].ID), nil, nil); err != nil {
			return nil, err
		}
	}
	if list == nil {
		list = []Message{}
	}
	return list, nil
}

func (h HTTP) Ack(ctx context.Context, id string) error {
	_, err := h.do(ctx, http.MethodPost, "/inbox/"+url.PathEscape(id)+"/ack", nil, nil)
	return err
}

package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/edition"
)

func TestMCPEndpoint(t *testing.T) {
	for _, mode := range []edition.Mode{edition.Desktop, edition.Server} {
		e := setup(t, mode)
		var created struct {
			Token string `json:"token"`
			Key   struct {
				ID     int64  `json:"id"`
				Prefix string `json:"prefix"`
			} `json:"key"`
		}
		e.do("POST", "/api/me/keys", `{"name":"Claude"}`, 201, &created)
		if !strings.HasPrefix(created.Token, "fk_") || created.Key.Prefix != created.Token[:10] {
			t.Fatalf("%s: kľúč %+v", mode, created)
		}
		var ro struct{ Token string }
		e.do("POST", "/api/me/keys", `{"name":"Len čítanie","readOnly":true}`, 201, &ro)
		var keys []struct{ Name string }
		e.do("GET", "/api/me/keys", nil, 200, &keys)
		if len(keys) != 2 {
			t.Fatalf("%s: zoznam kľúčov %+v", mode, keys)
		}

		call := func(token, origin, method, body string) (int, string) {
			req, _ := http.NewRequest(method, e.srv.URL+"/mcp", bytes.NewBufferString(body))
			if token != "" {
				req.Header.Set("Authorization", "Bearer "+token)
			}
			if origin != "" {
				req.Header.Set("Origin", origin)
			}
			req.Header.Set("Content-Type", "application/json")
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			b, _ := io.ReadAll(res.Body)
			return res.StatusCode, string(b)
		}
		list := `{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"list_companies","arguments":{}}}`
		if st, _ := call("", "", "POST", list); st != 401 {
			t.Errorf("%s: bez kľúča %d", mode, st)
		}
		if st, _ := call("fk_zly", "", "POST", list); st != 401 {
			t.Errorf("%s: zlý kľúč %d", mode, st)
		}
		if st, _ := call(created.Token, "", "GET", ""); st != 405 {
			t.Errorf("%s: GET %d", mode, st)
		}
		if st, _ := call(created.Token, "https://zly.example", "POST", list); st != 403 {
			t.Errorf("%s: cudzí pôvod %d", mode, st)
		}
		st, body := call(created.Token, "", "POST", list)
		var resp struct {
			ID     int
			Result struct {
				IsError bool `json:"isError"`
				Content []struct{ Text string }
			}
		}
		_ = json.Unmarshal([]byte(body), &resp)
		if st != 200 || resp.ID != 7 || resp.Result.IsError || !strings.Contains(resp.Result.Content[0].Text, "Moja firma") {
			t.Fatalf("%s: volanie %d %s", mode, st, body)
		}
		if st, _ := call(created.Token, "", "POST", `{"jsonrpc":"2.0","method":"notifications/initialized"}`); st != 202 {
			t.Errorf("%s: notifikácia %d", mode, st)
		}
		_, body = call(ro.Token, "", "POST", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"create_contact","arguments":{"contact":{"name":"X"}}}}`)
		if !strings.Contains(body, `"isError":true`) {
			t.Errorf("%s: zápis kľúčom na čítanie: %s", mode, body)
		}
		// zrušený kľúč
		e.do("DELETE", "/api/me/keys/"+itoa(created.Key.ID), nil, 204, nil)
		if st, _ := call(created.Token, "", "POST", list); st != 401 {
			t.Errorf("%s: zrušený kľúč %d", mode, st)
		}
	}
}

func itoa(n int64) string {
	b, _ := json.Marshal(n)
	return string(b)
}

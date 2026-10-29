// fiskus-mcp prepojí AI asistenta, ktorý spúšťa MCP servery cez stdio
// (napr. Claude Desktop), s bežiacim Fiškusom (POST /mcp).
//
//	FISKUS_API_KEY=fk_… fiskus-mcp -url http://127.0.0.1:8080/mcp
//
// Každý riadok zo vstupu je jedna správa JSON-RPC; odpoveď sa vypíše ako
// jeden riadok na výstup. Kľúč je vhodnejšie zadať v premennej prostredia
// než ako parameter (parametre vidno v zozname procesov).
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

func main() {
	url := flag.String("url", "http://127.0.0.1:8080/mcp", "adresa MCP servera Fiškusa")
	key := flag.String("key", "", "API kľúč (radšej premenná FISKUS_API_KEY)")
	flag.Parse()
	if *key == "" {
		*key = os.Getenv("FISKUS_API_KEY")
	}
	if *key == "" {
		fmt.Fprintln(os.Stderr, "fiskus-mcp: chýba API kľúč — nastavte FISKUS_API_KEY (vytvoríte ho vo Fiškuse: Prístup pre AI)")
		os.Exit(2)
	}
	if err := run(os.Stdin, os.Stdout, *url, *key, &http.Client{Timeout: 2 * time.Minute}); err != nil {
		fmt.Fprintln(os.Stderr, "fiskus-mcp:", err)
		os.Exit(1)
	}
}

func run(in io.Reader, out io.Writer, url, key string, client *http.Client) error {
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 1<<20), 8<<20)
	w := bufio.NewWriter(out)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		resp, err := forward(client, url, key, line)
		if err != nil {
			resp = errorReply(line, err)
		}
		if len(resp) > 0 {
			w.Write(bytes.TrimSpace(resp))
			w.WriteByte('\n')
			if err := w.Flush(); err != nil {
				return err
			}
		}
	}
	return sc.Err()
}

func forward(client *http.Client, url, key string, msg []byte) ([]byte, error) {
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(msg))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "Bearer "+key)
	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Fiškus neodpovedá na %s — beží program? (%v)", url, err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	switch {
	case res.StatusCode == http.StatusAccepted:
		return nil, nil // notifikácia
	case res.StatusCode != http.StatusOK:
		var e struct{ Error string }
		_ = json.Unmarshal(body, &e)
		if e.Error == "" {
			e.Error = res.Status
		}
		return nil, fmt.Errorf("%s", e.Error)
	}
	return body, nil
}

// errorReply vytvorí chybovú odpoveď JSON-RPC (iba ak správa mala id).
func errorReply(msg []byte, err error) []byte {
	var req struct {
		ID json.RawMessage `json:"id"`
	}
	if json.Unmarshal(msg, &req) != nil || len(req.ID) == 0 {
		return nil
	}
	b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": req.ID, "error": map[string]any{"code": -32000, "message": err.Error()}})
	return b
}

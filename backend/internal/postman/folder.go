package postman

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Folder vymieňa e-faktúry cez priečinky, ktoré synchronizuje aplikácia
// poskytovateľa (alebo ich poskytovateľ číta cez SFTP): odosielané sa
// ukladajú do Out, prijaté sa čítajú z In a po prevzatí sa presunú do
// In/prevzate.
type Folder struct {
	Out, In string
}

// DoneDir je podpriečinok prevzatých e-faktúr.
const DoneDir = "prevzate"

// Send uloží e-faktúru do priečinka odoslaných (bezpečne: dočasný súbor
// a premenovanie, aby poskytovateľ nečítal neúplný súbor).
func (f Folder) Send(_ context.Context, o Outgoing) (Sent, error) {
	if err := os.MkdirAll(f.Out, 0o755); err != nil {
		return Sent{}, fmt.Errorf("priečinok odoslaných: %w", err)
	}
	name := safeName(o.FileName)
	target := filepath.Join(f.Out, name)
	if _, err := os.Stat(target); err == nil {
		// rovnaký doklad poslaný znova — nový súbor s časom
		ext := filepath.Ext(name)
		name = strings.TrimSuffix(name, ext) + time.Now().Format("-20060102-150405") + ext
		target = filepath.Join(f.Out, name)
	}
	tmp := target + ".tmp"
	if err := os.WriteFile(tmp, o.Data, 0o644); err != nil {
		return Sent{}, fmt.Errorf("zápis e-faktúry: %w", err)
	}
	if err := os.Rename(tmp, target); err != nil {
		_ = os.Remove(tmp)
		return Sent{}, fmt.Errorf("zápis e-faktúry: %w", err)
	}
	return Sent{MessageID: name, Status: "v priečinku na odoslanie"}, nil
}

// Inbox vráti súbory XML v priečinku prijatých (najstaršie prvé).
func (f Folder) Inbox(_ context.Context) ([]Message, error) {
	entries, err := os.ReadDir(f.In)
	if errors.Is(err, os.ErrNotExist) {
		return []Message{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("priečinok prijatých: %w", err)
	}
	out := []Message{}
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".xml") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		data, err := os.ReadFile(filepath.Join(f.In, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("čítanie %s: %w", e.Name(), err)
		}
		out = append(out, Message{ID: e.Name(), Name: e.Name(), Received: info.ModTime(), Data: data})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Received.Before(out[j].Received) })
	return out, nil
}

// Ack presunie súbor do In/prevzate.
func (f Folder) Ack(_ context.Context, id string) error {
	if id != filepath.Base(id) || id == "." || id == ".." {
		return fmt.Errorf("neplatný názov súboru %q", id)
	}
	done := filepath.Join(f.In, DoneDir)
	if err := os.MkdirAll(done, 0o755); err != nil {
		return err
	}
	return os.Rename(filepath.Join(f.In, id), filepath.Join(done, id))
}

// safeName odstráni z názvu súboru znaky, ktoré systémy nemajú radi.
func safeName(s string) string {
	s = filepath.Base(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '/' || r == '\\' || r == ':' || r == '*' || r == '?' || r == '"' || r == '<' || r == '>' || r == '|' || r < 32:
			b.WriteRune('-')
		default:
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 || s == "." {
		return "e-faktura.xml"
	}
	return b.String()
}

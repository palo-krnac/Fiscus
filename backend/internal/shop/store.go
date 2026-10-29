package shop

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"time"
)

// Record je vydaná licencia (evidencia predaja; kľúč sa dá poslať znova).
type Record struct {
	TransactionID string    `json:"transactionId"`
	Customer      string    `json:"customer"`
	Email         string    `json:"email"`
	Companies     int       `json:"companies"`
	Issued        time.Time `json:"issued"`
	Expires       time.Time `json:"expires"`
	Key           string    `json:"key"`
	Emailed       bool      `json:"emailed"`
}

// Store je evidencia vydaných licencií.
type Store interface {
	Get(transactionID string) (Record, bool)
	// LastFor vráti poslednú licenciu zákazníka (podľa e-mailu) — pri obnove
	// sa nová platnosť počíta od jej konca.
	LastFor(email string) (Record, bool)
	Add(Record) error
}

// FileStore je evidencia v súbore JSON Lines (jeden riadok = jedna licencia,
// iba pripisuje — história sa nemení).
type FileStore struct {
	Path string

	mu      sync.Mutex
	loaded  bool
	records []Record
}

func (f *FileStore) load() error {
	if f.loaded {
		return nil
	}
	file, err := os.Open(f.Path)
	if errors.Is(err, os.ErrNotExist) {
		f.loaded = true
		return nil
	}
	if err != nil {
		return err
	}
	defer file.Close()
	sc := bufio.NewScanner(file)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var r Record
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			return err
		}
		f.records = append(f.records, r)
	}
	if err := sc.Err(); err != nil {
		return err
	}
	f.loaded = true
	return nil
}

func (f *FileStore) Get(id string) (Record, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.load() != nil {
		return Record{}, false
	}
	// posledný záznam transakcie (po odoslaní e-mailu pribudne riadok Emailed)
	var out Record
	found := false
	for _, r := range f.records {
		if r.TransactionID == id {
			out, found = r, true
		}
	}
	return out, found
}

func (f *FileStore) LastFor(email string) (Record, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.load() != nil {
		return Record{}, false
	}
	var last Record
	found := false
	for _, r := range f.records {
		if strings.EqualFold(r.Email, email) && (!found || r.Expires.After(last.Expires)) {
			last, found = r, true
		}
	}
	return last, found
}

func (f *FileStore) Add(r Record) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.load(); err != nil {
		return err
	}
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(f.Path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	if _, err := file.Write(append(b, '\n')); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	f.records = append(f.records, r)
	return nil
}

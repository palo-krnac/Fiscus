package api

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/palo-krnac/fiscus/backend/internal/backup"
	"github.com/palo-krnac/fiscus/backend/internal/edition"
)

type fakeDB struct{}

func (fakeDB) Snapshot(_ context.Context, dst string) error {
	return os.WriteFile(dst, []byte("DB:x"), 0o600)
}
func (fakeDB) Verify(context.Context, string) error { return nil }

func TestBackupsAPI(t *testing.T) {
	e := setup(t, edition.Server)
	var st struct {
		Enabled bool
		Backups []struct{ Name string }
	}
	e.do("GET", "/api/backups", nil, 200, &st)
	if st.Enabled {
		t.Fatal("zálohy by mali byť vypnuté")
	}
	e.do("POST", "/api/backups", nil, 409, nil)

	e.app.Backups = &backup.Manager{Dir: filepath.Join(t.TempDir(), "z"), Password: "h", Policy: backup.DefaultPolicy, Source: fakeDB{}}
	var it struct{ Name string }
	e.do("POST", "/api/backups", nil, 201, &it)
	e.do("GET", "/api/backups", nil, 200, &st)
	if !st.Enabled || len(st.Backups) != 1 {
		t.Fatalf("stav: %+v", st)
	}
	res, err := e.client.Get(e.srv.URL + "/api/backups/" + it.Name)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 || len(body) == 0 || res.Header.Get("Content-Type") != "application/octet-stream" {
		t.Fatalf("stiahnutie: %d %d", res.StatusCode, len(body))
	}
	e.do("GET", "/api/backups/neexistuje.fbk", nil, 404, nil)
	e.do("POST", "/api/backups/"+it.Name+"/restore", nil, 204, nil)
	e.do("DELETE", "/api/backups/restore", nil, 204, nil)

	// bežný používateľ zálohy nevidí
	e.do("POST", "/api/companies/"+itoa(e.cid)+"/users", map[string]string{"email": "u@firma.sk", "name": "U", "password": "heslo-12345", "role": "accountant"}, 201, nil)
	u := e.anonymous()
	u.login("u@firma.sk", "heslo-12345", 200)
	u.do("GET", "/api/backups", nil, 403, nil)
}

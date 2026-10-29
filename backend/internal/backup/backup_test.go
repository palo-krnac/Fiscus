package backup

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEncryptRoundTrip(t *testing.T) {
	plain := []byte("účtovné dáta 123,45 €")
	var buf bytes.Buffer
	if err := Encrypt(&buf, plain, "tajne"); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(buf.Bytes(), []byte("123,45")) {
		t.Fatal("záloha nie je zašifrovaná")
	}
	got, err := Decrypt(buf.Bytes(), "tajne")
	if err != nil || !bytes.Equal(got, plain) {
		t.Fatalf("dešifrovanie: %v", err)
	}
}

func TestDecryptRejects(t *testing.T) {
	var buf bytes.Buffer
	_ = Encrypt(&buf, []byte("dáta"), "heslo")
	if _, err := Decrypt(buf.Bytes(), "zle"); !errors.Is(err, ErrBadPassword) {
		t.Errorf("zlé heslo: %v", err)
	}
	corrupt := append([]byte(nil), buf.Bytes()...)
	corrupt[len(corrupt)-1] ^= 0xff
	if _, err := Decrypt(corrupt, "heslo"); !errors.Is(err, ErrBadPassword) {
		t.Errorf("poškodené dáta: %v", err)
	}
	if _, err := Decrypt([]byte("niečo iné"), "heslo"); !errors.Is(err, ErrNotBackup) {
		t.Errorf("cudzí súbor: %v", err)
	}
	if err := Encrypt(&buf, []byte("x"), ""); err == nil {
		t.Error("prázdne heslo malo zlyhať")
	}
}

func TestPolicyKeep(t *testing.T) {
	// zálohy každý deň o 18:00 za posledných 400 dní
	now := time.Date(2026, 10, 29, 18, 0, 0, 0, time.UTC)
	var times []time.Time
	for i := 0; i < 400; i++ {
		times = append(times, now.AddDate(0, 0, -i))
	}
	keep := DefaultPolicy.Keep(times)
	// 7 denných + týždenné/mesačné, ktoré sa s nimi prekrývajú
	if len(keep) < 12 || len(keep) > 7+4+12 {
		t.Fatalf("ponechaných %d záloh", len(keep))
	}
	for i := 0; i < 7; i++ {
		if !keep[times[i]] {
			t.Fatalf("chýba denná záloha spred %d dní", i)
		}
	}
	if keep[times[399]] {
		t.Fatal("záloha spred 399 dní sa nemala ponechať")
	}
	// najstaršia ponechaná má byť z 12. mesiaca dozadu
	oldest := now
	for tm := range keep {
		if tm.Before(oldest) {
			oldest = tm
		}
	}
	if oldest.Before(now.AddDate(-1, 0, 0)) {
		t.Fatalf("ponechaná príliš stará záloha %s", oldest)
	}
}

func TestPolicyKeepsNewestInDay(t *testing.T) {
	day := time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC)
	times := []time.Time{day.Add(9 * time.Hour), day.Add(17 * time.Hour), day.Add(12 * time.Hour)}
	keep := Policy{Daily: 1}.Keep(times)
	if len(keep) != 1 || !keep[day.Add(17*time.Hour)] {
		t.Fatalf("ponechané: %v", keep)
	}
}

// fakeSource simuluje databázu: Snapshot zapíše obsah, Verify kontroluje hlavičku.
type fakeSource struct{ content []byte }

func (f *fakeSource) Snapshot(_ context.Context, dst string) error {
	return os.WriteFile(dst, f.content, 0o600)
}

func (f *fakeSource) Verify(_ context.Context, path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if !bytes.HasPrefix(b, []byte("DB:")) {
		return errors.New("integrity_check zlyhal")
	}
	return nil
}

func TestManagerCreateListRestore(t *testing.T) {
	dir := t.TempDir()
	src := &fakeSource{content: []byte("DB:verzia1")}
	m := &Manager{Dir: filepath.Join(dir, "zalohy"), Password: "heslo", Policy: DefaultPolicy, Source: src}
	ctx := context.Background()
	t0 := time.Date(2026, 10, 28, 18, 0, 0, 0, time.UTC)

	if _, err := m.Create(ctx, t0); err != nil {
		t.Fatal(err)
	}
	src.content = []byte("DB:verzia2")
	info, err := m.Create(ctx, t0.Add(24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	list, _ := m.List()
	if len(list) != 2 || list[0].Path != info.Path {
		t.Fatalf("zoznam: %+v", list)
	}

	// obnova staršej zálohy
	db := filepath.Join(dir, "fiskus.db")
	_ = os.WriteFile(db, []byte("DB:aktuálna"), 0o600)
	if err := m.Restore(ctx, list[1].Path, db); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(db); string(b) != "DB:verzia1" {
		t.Fatalf("obnovené: %s", b)
	}
	if b, _ := os.ReadFile(db + ".pred-obnovou"); string(b) != "DB:aktuálna" {
		t.Fatal("pôvodná databáza sa pri obnove stratila")
	}
}

func TestManagerRefusesCorruptSnapshot(t *testing.T) {
	m := &Manager{Dir: t.TempDir(), Password: "h", Policy: DefaultPolicy, Source: &fakeSource{content: []byte("rozbité")}}
	if _, err := m.Create(context.Background(), time.Now()); err == nil {
		t.Fatal("poškodená kópia sa nesmie uložiť ako záloha")
	}
	if list, _ := m.List(); len(list) != 0 {
		t.Fatal("vznikla záloha z poškodenej kópie")
	}
}

func TestManagerPrunes(t *testing.T) {
	m := &Manager{Dir: t.TempDir(), Password: "h", Policy: Policy{Daily: 3}, Source: &fakeSource{content: []byte("DB:x")}}
	start := time.Date(2026, 10, 1, 18, 0, 0, 0, time.UTC)
	for i := 0; i < 6; i++ {
		if _, err := m.Create(context.Background(), start.AddDate(0, 0, i)); err != nil {
			t.Fatal(err)
		}
	}
	list, _ := m.List()
	if len(list) != 3 {
		t.Fatalf("po rotácii %d záloh, chcem 3", len(list))
	}
}

func TestScheduledRestore(t *testing.T) {
	dir := t.TempDir()
	src := &fakeSource{content: []byte("DB:stará")}
	m := &Manager{Dir: filepath.Join(dir, "zalohy"), Password: "heslo", Policy: DefaultPolicy, Source: src}
	b, err := m.Create(context.Background(), time.Date(2026, 10, 29, 8, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	name := filepath.Base(b.Path)
	for _, bad := range []string{"", "../" + name, "iny.fbk", "fiskus-20260101-000000.fbk"} {
		if _, err := m.Find(bad); !errors.Is(err, ErrNoBackup) {
			t.Errorf("%q: %v", bad, err)
		}
		if err := m.RequestRestore(bad); err == nil {
			t.Errorf("%q: obnova naplánovaná", bad)
		}
	}
	db := filepath.Join(dir, "fiskus.db")
	if err := os.WriteFile(db, []byte("DB:nová"), 0o600); err != nil {
		t.Fatal(err)
	}
	// bez plánu sa nič nedeje
	if _, ok, err := m.ApplyPending(context.Background(), db); ok || err != nil {
		t.Fatalf("bez plánu: %v %v", ok, err)
	}
	if err := m.RequestRestore(name); err != nil || m.PendingRestore() != name {
		t.Fatalf("plán: %v %q", err, m.PendingRestore())
	}
	if err := m.CancelRestore(); err != nil || m.PendingRestore() != "" {
		t.Fatal("zrušenie plánu")
	}
	_ = m.RequestRestore(name)
	got, ok, err := m.ApplyPending(context.Background(), db)
	if err != nil || !ok || got.Path != b.Path {
		t.Fatalf("obnova: %+v %v %v", got, ok, err)
	}
	if data, _ := os.ReadFile(db); string(data) != "DB:stará" {
		t.Fatalf("obnovená databáza: %q", data)
	}
	if prev, _ := os.ReadFile(db + ".pred-obnovou"); string(prev) != "DB:nová" {
		t.Fatal("pôvodná databáza sa nezachovala")
	}
	if m.PendingRestore() != "" {
		t.Fatal("plán zostal po obnove")
	}
}

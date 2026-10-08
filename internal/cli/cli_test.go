package cli

import (
	"bytes"
	"context"
	"database/sql"
	"github.com/hidxt/miskoai/internal/storage"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVersionAndUnknownCommand(t *testing.T) {
	var out bytes.Buffer
	if err := Run([]string{"version"}, &out); err != nil || !strings.Contains(out.String(), "MiskoAI") {
		t.Fatalf("version failed: %v %q", err, out.String())
	}
	if err := Run([]string{"unknown"}, &out); err == nil {
		t.Fatal("unknown command accepted")
	}
}

func TestRestoreRejectsUnrelatedSQLite(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MISKOAI_DATA_DIR", dir)
	path := filepath.Join(dir, "miskoai.db")
	os.WriteFile(path, []byte("existing synthetic data"), 0600)
	foreign := filepath.Join(dir, "unrelated.db")
	db, err := sql.Open("sqlite", foreign)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("CREATE TABLE unrelated (content TEXT)"); err != nil {
		t.Fatal(err)
	}
	db.Close()
	var out bytes.Buffer
	if err = Run([]string{"restore", foreign}, &out); err == nil {
		t.Fatal("unrelated SQLite backup accepted")
	}
	data, _ := os.ReadFile(path)
	if string(data) != "existing synthetic data" {
		t.Fatal("unrelated database replaced existing data")
	}
}

func TestBackupRestoreRoundTrip(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private")
	t.Setenv("MISKOAI_DATA_DIR", dir)
	s, err := storage.Open(filepath.Join(dir, "miskoai.db"))
	if err != nil {
		t.Fatal(err)
	}
	scope := storage.Scope{Account: "fixture", User: "alice"}
	id, err := s.AddFact(context.Background(), scope, storage.Fact{Content: "explicit fixture fact", Source: "explicit_user", Confidence: 1, Importance: 5})
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	backup := filepath.Join(t.TempDir(), "private", "snapshot.db")
	var out bytes.Buffer
	if err = Run([]string{"backup", backup}, &out); err != nil {
		t.Fatal(err)
	}
	s, err = storage.Open(filepath.Join(dir, "miskoai.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err = s.DeleteFact(context.Background(), scope, id); err != nil {
		t.Fatal(err)
	}
	s.Close()
	if err = Run([]string{"restore", backup}, &out); err != nil {
		t.Fatal(err)
	}
	s, err = storage.Open(filepath.Join(dir, "miskoai.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	facts, err := s.ExportFacts(context.Background(), scope)
	if err != nil || len(facts) != 1 || facts[0].Content != "explicit fixture fact" {
		t.Fatalf("restore failed %#v %v", facts, err)
	}
}

func TestInitNeverOverwritesAndDoctorRedacts(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private")
	t.Setenv("MISKOAI_DATA_DIR", dir)
	t.Setenv("DEEPSEEK_API_KEY", "synthetic-private-credential")
	var out bytes.Buffer
	if err := Run([]string{"init"}, &out); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "settings.json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := Run([]string{"init"}, &out); err == nil {
		t.Fatal("second init overwrote configuration")
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Fatal("configuration changed")
	}
	if err := Run([]string{"doctor"}, &out); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "synthetic-private-credential") || strings.Contains(string(after), "synthetic-private-credential") {
		t.Fatal("credential leaked")
	}
}

func TestRestoreRejectsCorruptBackupWithoutReplacingData(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MISKOAI_DATA_DIR", dir)
	path := filepath.Join(dir, "miskoai.db")
	if err := os.WriteFile(path, []byte("existing synthetic data"), 0600); err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(dir, "bad.db")
	if err := os.WriteFile(bad, []byte("corrupt backup"), 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Run([]string{"restore", bad}, &out); err == nil {
		t.Fatal("corrupt restore accepted")
	}
	data, _ := os.ReadFile(path)
	if string(data) != "existing synthetic data" {
		t.Fatal("existing data destroyed")
	}
}

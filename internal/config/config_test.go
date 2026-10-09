package config

import (
	"github.com/hidxt/miskoai/internal/privatefs"
	"os"
	"path/filepath"
	"testing"
)

func cleanEnv(t *testing.T) string {
	t.Helper()
	for _, name := range []string{"MISKOAI_LISTEN", "MISKOAI_DATA_DIR", "MISKOAI_DEEPSEEK_URL", "MISKOAI_MODEL", "MISKOAI_VISION_MODEL", "MISKOAI_WEIXIN_URL", "MISKOAI_MAX_OUTPUT_TOKENS", "MISKOAI_CONTEXT_BYTES", "MISKOAI_CONTEXT_TOKENS", "MISKOAI_CREDENTIALS_FILE", "DEEPSEEK_API_KEY", "OLLAMA_API_KEY", "MISKOAI_ADMIN_PASSWORD"} {
		old, ok := os.LookupEnv(name)
		os.Unsetenv(name)
		t.Cleanup(func() {
			if ok {
				os.Setenv(name, old)
			} else {
				os.Unsetenv(name)
			}
		})
	}
	dir := filepath.Join(t.TempDir(), "private")
	if err := privatefs.EnsureDir(dir); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MISKOAI_DATA_DIR", dir)
	return dir
}
func fixture(t *testing.T, dir, name, data string) {
	t.Helper()
	f, err := privatefs.Create(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.WriteString(data); err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
}
func TestStoredSettingsAreRuntimeConfiguration(t *testing.T) {
	dir := cleanEnv(t)
	fixture(t, dir, "settings.json", `{"model":"saved-model"}`)
	c, err := Load()
	if err != nil || c.Model != "saved-model" {
		t.Fatalf("stored model ignored: %v %q", err, c.Model)
	}
}
func TestPresentEmptyRuntimeValuesReject(t *testing.T) {
	for _, name := range []string{"MISKOAI_MAX_OUTPUT_TOKENS", "DEEPSEEK_API_KEY", "MISKOAI_CREDENTIALS_FILE", "MISKOAI_ADMIN_PASSWORD"} {
		t.Run(name, func(t *testing.T) {
			cleanEnv(t)
			t.Setenv(name, "")
			if _, err := Load(); err == nil {
				t.Fatal("explicit empty value accepted")
			}
		})
	}
}

func TestEndpointAndListenerValidation(t *testing.T) {
	cleanEnv(t)
	t.Setenv("MISKOAI_LISTEN", "0.0.0.0:8787")
	if _, err := Load(); err == nil {
		t.Fatal("public listener accepted")
	}
	t.Setenv("MISKOAI_LISTEN", "127.0.0.1:8787")
	t.Setenv("MISKOAI_DEEPSEEK_URL", "http://api.deepseek.com")
	if _, err := Load(); err == nil {
		t.Fatal("insecure endpoint accepted")
	}
	t.Setenv("MISKOAI_DEEPSEEK_URL", "https://api.deepseek.com")
	c, err := Load()
	if err != nil || c.Model == "" {
		t.Fatal("valid configuration rejected", err)
	}
}

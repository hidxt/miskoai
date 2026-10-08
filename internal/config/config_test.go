package config

import "testing"

func TestEndpointAndListenerValidation(t *testing.T) {
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

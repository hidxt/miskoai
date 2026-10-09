package config

import (
	"errors"
	"github.com/hidxt/miskoai/internal/privatefs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCredentialFileExplicitOnly(t *testing.T) {
	dir := cleanEnv(t)
	fixture(t, dir, "api-keys.json", `{"DEEPSEEK_API_KEY":"synthetic-key","OLLAMA_API_KEY":"synthetic-search"}`)
	c, err := Load()
	if err != nil || c.DeepSeekKey != "" {
		t.Fatal("guessed credential discovery", err)
	}
	t.Setenv("MISKOAI_CREDENTIALS_FILE", filepath.Join(dir, "api-keys.json"))
	c, err = Load()
	if err != nil || c.DeepSeekKey != "synthetic-key" || c.OllamaKey != "synthetic-search" {
		t.Fatal("explicit credentials not loaded", err)
	}
	t.Setenv("DEEPSEEK_API_KEY", "")
	if _, err = Load(); err == nil {
		t.Fatal("empty environment fell back")
	}
	t.Setenv("DEEPSEEK_API_KEY", "env-key")
	t.Setenv("OLLAMA_API_KEY", "env-search")
	t.Setenv("MISKOAI_CREDENTIALS_FILE", filepath.Join(dir, "missing"))
	c, err = Load()
	if err != nil || c.DeepSeekKey != "env-key" {
		t.Fatal("unnecessary file read", err)
	}
	t.Setenv("MISKOAI_CREDENTIALS_FILE", "")
	if _, err = Load(); err == nil {
		t.Fatal("empty file path accepted")
	}
}
func TestCredentialStrictSchema(t *testing.T) {
	cases := []string{`{}`, `{"DEEPSEEK_API_KEY":null,"OLLAMA_API_KEY":"s"}`, `{"deepseek_api_key":"k","OLLAMA_API_KEY":"s"}`, `{"DEEPSEEK_API_KEY":"a","DEEPSEEK_API_KEY":"b","OLLAMA_API_KEY":"s"}`, `{"DEEPSEEK_API_KEY":"k","OLLAMA_API_KEY":"s"}`, `{"DEEPSEEK_API_KEY":"k","OLLAMA_API_KEY":"s"} {}`, `{"DEEPSEEK_API_KEY":"\u0001","OLLAMA_API_KEY":"s"}`, `{"DEEPSEEK_API_KEY":"","OLLAMA_API_KEY":"s"}`, strings.Repeat(" ", 8<<10) + `{}`}
	for i, data := range cases {
		t.Run(string(rune('A'+i)), func(t *testing.T) {
			dir := cleanEnv(t)
			fixture(t, dir, "keys.json", data)
			t.Setenv("MISKOAI_CREDENTIALS_FILE", filepath.Join(dir, "keys.json"))
			if _, err := Load(); err == nil {
				t.Fatal("invalid credential file accepted")
			}
		})
	}
}
func TestAuthorizationMissingVersusInvalid(t *testing.T) {
	dir := cleanEnv(t)
	if _, err := LoadAuthorization(dir); !errors.Is(err, ErrAuthorizationMissing) {
		t.Fatal(err)
	}
	fixture(t, dir, "weixin-auth.json", `{"bot_token":"synthetic-token","account":"fixture","allowed_user":"alice","base_url":"https://ilinkai.weixin.qq.com"}`)
	a, err := LoadAuthorization(dir)
	if err != nil || a.Account != "fixture" {
		t.Fatal(err)
	}
	os.Remove(filepath.Join(dir, "weixin-auth.json"))
	fixture(t, dir, "weixin-auth.json", `{"bot_token":"synthetic-secret-canary","account":"fixture","allowed_user":"alice","base_url":"https://ilinkai.weixin.qq.com"} {}`)
	if _, err = LoadAuthorization(dir); err == nil || errors.Is(err, ErrAuthorizationMissing) || strings.Contains(err.Error(), "synthetic-secret-canary") {
		t.Fatal("invalid authorization confused with absence or leaked", err)
	}
}
func TestAuthorizationStrictSchema(t *testing.T) {
	valid := `{"bot_token":"synthetic-token","account":"fixture","allowed_user":"alice","base_url":"https://ilinkai.weixin.qq.com"}`
	cases := []string{strings.Replace(valid, "bot_token", "Bot_token", 1), strings.Replace(valid, "account", "acCount", 1), strings.Replace(valid, `"fixture"`, `null`, 1), strings.Replace(valid, `"fixture"`, `""`, 1), strings.Replace(valid, "https://ilinkai.weixin.qq.com", "https://example.com", 1), strings.Replace(valid, `"bot_token":"synthetic-token"`, `"bot_token":"a","bot_token":"b"`, 1), strings.Replace(valid, "synthetic-token", strings.Repeat("x", (16<<10)+1), 1), strings.Repeat(" ", 64<<10) + valid}
	for i, data := range cases {
		t.Run(string(rune('A'+i)), func(t *testing.T) {
			dir := cleanEnv(t)
			fixture(t, dir, "weixin-auth.json", data)
			if _, err := LoadAuthorization(dir); err == nil || errors.Is(err, ErrAuthorizationMissing) {
				t.Fatal("invalid auth accepted", err)
			}
		})
	}
}

func TestPrivateInputsRejectBroadFileAndSafeErrors(t *testing.T) {
	for _, kind := range []string{"credentials", "authorization"} {
		t.Run(kind, func(t *testing.T) {
			dir := cleanEnv(t)
			name := "keys.json"
			payload := `{"DEEPSEEK_API_KEY":"synthetic-secret-canary","OLLAMA_API_KEY":"synthetic-search"}`
			if kind == "authorization" {
				name = "weixin-auth.json"
				payload = `{"bot_token":"synthetic-secret-canary","account":"fixture","allowed_user":"alice","base_url":"https://ilinkai.weixin.qq.com"}`
			}
			broad := filepath.Join(t.TempDir(), "broad")
			if err := os.WriteFile(broad, []byte(payload), 0644); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(broad, 0644); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, name)
			if err := os.Rename(broad, path); err != nil {
				t.Fatal(err)
			}
			if privatefs.CheckFile(path, 64<<10) == nil {
				t.Fatal("unsafe fixture private")
			}
			var err error
			if kind == "authorization" {
				_, err = LoadAuthorization(dir)
			} else {
				t.Setenv("MISKOAI_CREDENTIALS_FILE", path)
				_, err = Load()
			}
			if err == nil || errors.Is(err, ErrAuthorizationMissing) || strings.Contains(err.Error(), "synthetic-secret-canary") {
				t.Fatal("unsafe private input accepted, missing or leaked", err)
			}
			if privatefs.CheckFile(path, 64<<10) == nil {
				t.Fatal("permissions repaired")
			}
			got, _ := os.ReadFile(path)
			if string(got) != payload {
				t.Fatal("payload changed")
			}
		})
	}
}
func TestCredentialValuesAreBoundedAndExact(t *testing.T) {
	for _, value := range []string{" ", "x\t", "x\x7f", strings.Repeat("x", 1025)} {
		t.Run("invalid", func(t *testing.T) {
			cleanEnv(t)
			t.Setenv("DEEPSEEK_API_KEY", value)
			if _, err := Load(); err == nil {
				t.Fatal("invalid environment key accepted")
			}
		})
	}
	for _, value := range []string{`\u0000`, string([]byte{255})} {
		t.Run("invalid-file-key", func(t *testing.T) {
			dir := cleanEnv(t)
			fixture(t, dir, "keys.json", `{"DEEPSEEK_API_KEY":"`+value+`","OLLAMA_API_KEY":"s"}`)
			t.Setenv("MISKOAI_CREDENTIALS_FILE", filepath.Join(dir, "keys.json"))
			if _, err := Load(); err == nil {
				t.Fatal("invalid file key accepted")
			}
		})
	}
	for _, escape := range []string{`\ud800`, `\udfff`} {
		t.Run(escape, func(t *testing.T) {
			dir := cleanEnv(t)
			fixture(t, dir, "keys.json", `{"DEEPSEEK_API_KEY":"`+escape+`","OLLAMA_API_KEY":"s"}`)
			t.Setenv("MISKOAI_CREDENTIALS_FILE", filepath.Join(dir, "keys.json"))
			if _, err := Load(); err == nil {
				t.Fatal("private credential silently rewritten")
			}
		})
	}
	cleanEnv(t)
	t.Setenv("DEEPSEEK_API_KEY", strings.Repeat("x", 1024))
	t.Setenv("MISKOAI_ADMIN_PASSWORD", "short")
	if _, err := Load(); err != nil {
		t.Fatal("diagnostic readiness wrongly requires16bytes", err)
	}
	t.Setenv("MISKOAI_ADMIN_PASSWORD", strings.Repeat("x", 257))
	if _, err := Load(); err == nil {
		t.Fatal("oversized password accepted")
	}
}

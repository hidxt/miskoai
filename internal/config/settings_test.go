package config

import (
	"encoding/json"
	"errors"
	"github.com/hidxt/miskoai/internal/privatefs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSettingsCompleteStrictObject(t *testing.T) {
	cases := []string{`{} {}`, `null`, `[]`, `{"model":null}`, `{"model":4}`, `{"Model":"x"}`, `{"model":"x","mo\u0064el":"y"}`, `{"liſten":"127.0.0.1:8787"}`, `{"model":"x","model":"y"}`, `{"DEEPSEEK_API_KEY":"secret-canary"}`, `{"context_bytes":16383}`, `{"context_tokens":65537}`, `{"max_output_tokens":0}`, `{"model":"` + string([]byte{255}) + `"}`, strings.Repeat(" ", 16<<10) + `{}`}
	for i, data := range cases {
		t.Run(string(rune('A'+i)), func(t *testing.T) {
			dir := cleanEnv(t)
			fixture(t, dir, "settings.json", data)
			if _, err := LoadSettings(dir); err == nil {
				t.Fatal("invalid settings accepted")
			} else if strings.Contains(err.Error(), "secret-canary") {
				t.Fatal("payload leaked")
			}
		})
	}
}
func TestSettingsLegacyDefaultsAndExplicitOverrides(t *testing.T) {
	dir := cleanEnv(t)
	fixture(t, dir, "settings.json", `{"listen":"127.0.0.1:8787","deepseek_url":"https://api.deepseek.com","model":"saved","vision_model":"saved-vision","weixin_url":"https://ilinkai.weixin.qq.com","max_output_tokens":700}`)
	t.Setenv("MISKOAI_MODEL", "effective")
	t.Setenv("MISKOAI_CONTEXT_BYTES", "32768")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.DesiredSettings().Model != "saved" || c.EffectiveSettings().Model != "effective" || c.ContextBytes != 32768 || c.ContextTokens != 32768 || !c.Overrides["model"] {
		t.Fatal("incorrect precedence or defaults")
	}
	copy := c.DesiredSettings()
	copy.Model = "changed"
	if c.DesiredSettings().Model != "saved" {
		t.Fatal("desired settings mutable")
	}
	t.Setenv("MISKOAI_MAX_OUTPUT_TOKENS", "")
	if _, err = Load(); err == nil {
		t.Fatal("empty override ignored")
	}
}
func TestInitSettingsNeverOverwrites(t *testing.T) {
	dir := cleanEnv(t)
	s := DefaultSettings()
	if err := InitSettings(dir, s); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	s.Model = "replacement"
	if err = InitSettings(dir, s); err == nil {
		t.Fatal("init overwrote")
	}
	after, _ := os.ReadFile(filepath.Join(dir, "settings.json"))
	if string(before) != string(after) {
		t.Fatal("existing settings changed")
	}
	if err = SaveSettings(dir, s); err != nil {
		t.Fatal(err)
	}
	got, err := LoadSettings(dir)
	if err != nil || got.Model != "replacement" {
		t.Fatal(got, err)
	}
	s.ContextBytes = 0
	if err = SaveSettings(dir, s); err == nil {
		t.Fatal("invalid save accepted")
	}
	got, _ = LoadSettings(dir)
	if got.Model != "replacement" {
		t.Fatal("failed save changed old settings")
	}
}
func TestSettingsMissingVersusUnsafe(t *testing.T) {
	dir := cleanEnv(t)
	if _, err := LoadSettings(dir); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(dir, "absent")
	if _, err := LoadSettings(missing); err != nil {
		t.Fatal(err)
	}
	fixture(t, dir, "parent", "x")
	if _, err := LoadSettings(filepath.Join(dir, "parent", "child")); err == nil {
		t.Fatal("invalid parent treated as missing")
	}
	broad := filepath.Join(t.TempDir(), "broad")
	if err := os.Mkdir(broad, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(broad, 0755); err != nil {
		t.Fatal(err)
	}
	if privatefs.CheckDir(broad) == nil {
		t.Fatal("broad fixture private")
	}
	if _, err := LoadSettings(broad); err == nil {
		t.Fatal("broad directory became defaults")
	}
}

func TestSettingsMissingWindowsAliasesAreUnsafe(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows path aliases only")
	}
	dir := cleanEnv(t)
	for _, name := range []string{"missing.", "missing ", "NUL", "CON.json", "missing:stream"} {
		if _, err := LoadSettings(filepath.Join(dir, name)); err == nil {
			t.Fatal("ambiguous Windows missing path accepted", name)
		}
		if err := InitSettings(filepath.Join(dir, name), DefaultSettings()); err == nil {
			t.Fatal("ambiguous Windows init path accepted", name)
		}
		if err := SaveSettings(filepath.Join(dir, name), DefaultSettings()); err == nil {
			t.Fatal("ambiguous Windows save path accepted", name)
		}
	}
}

func TestDefaultAndRelativeDataPathsUseSyntheticCWD(t *testing.T) {
	cleanEnv(t)
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	cwd := t.TempDir()
	if err = os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(previous); err != nil {
			t.Fatal(err)
		}
	})
	if err = os.Unsetenv("MISKOAI_DATA_DIR"); err != nil {
		t.Fatal(err)
	}
	c, err := Load()
	if err != nil || c.DataDir != filepath.Join(cwd, "data") {
		t.Fatal("default data path failed", c.DataDir, err)
	}
	relative := []string{"./data"}
	if runtime.GOOS == "windows" {
		relative = append(relative, `.\data`)
	}
	for _, path := range relative {
		t.Setenv("MISKOAI_DATA_DIR", path)
		c, err = Load()
		if err != nil || c.DataDir != filepath.Join(cwd, "data") {
			t.Fatal("relative data path failed", path, c.DataDir, err)
		}
		if _, err = LoadSettings(path); err != nil {
			t.Fatal("relative missing settings failed", path, err)
		}
	}
	if err = InitSettings(relative[0], DefaultSettings()); err != nil {
		t.Fatal("relative init failed", err)
	}
	s := DefaultSettings()
	s.Model = "relative-saved"
	if err = SaveSettings(relative[len(relative)-1], s); err != nil {
		t.Fatal("relative save failed", err)
	}
	c, err = Load()
	if err != nil || c.Model != "relative-saved" {
		t.Fatal("relative persisted settings failed", err)
	}
}
func TestPublicSettingsNeverContainSecrets(t *testing.T) {
	cleanEnv(t)
	for _, n := range []string{"DEEPSEEK_API_KEY", "OLLAMA_API_KEY", "MISKOAI_ADMIN_PASSWORD"} {
		t.Setenv(n, "synthetic-secret-canary")
	}
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range []any{c, c.DesiredSettings(), c.EffectiveSettings(), c.Overrides} {
		b, err := json.Marshal(v)
		if err != nil || strings.Contains(string(b), "synthetic-secret-canary") {
			t.Fatal("public leak", err)
		}
	}
}

func TestStrictEscapedStringsPreserveValues(t *testing.T) {
	for _, escaped := range []string{`\ud800`, `\udfff`, `\ud800x`, `\ud800\ud800`} {
		t.Run(escaped, func(t *testing.T) {
			dir := cleanEnv(t)
			fixture(t, dir, "settings.json", `{"model":"`+escaped+`"}`)
			if _, err := LoadSettings(dir); err == nil {
				t.Fatal("malformed UTF16 silently rewritten")
			}
		})
	}
	dir := cleanEnv(t)
	fixture(t, dir, "settings.json", `{"model":"a\ud83d\ude00\\ud800\""}`)
	s, err := LoadSettings(dir)
	if err != nil || s.Model != "a😀\\ud800\"" {
		t.Fatal("valid escapes changed", s.Model, err)
	}
}
func TestConfigurationRejectsSubstitutionAndPreservesTarget(t *testing.T) {
	target := cleanEnv(t)
	fixture(t, target, "settings.json", `{"model":"unchanged"}`)
	link := filepath.Join(t.TempDir(), "link")
	if runtime.GOOS == "windows" {
		if out, err := exec.Command("cmd.exe", "/c", "mklink", "/J", link, target).CombinedOutput(); err != nil {
			t.Skipf("host denies synthetic junction: %v %s", err, out)
		}
	} else if err := os.Symlink(target, link); err != nil {
		t.Skipf("host denies synthetic symlink: %v", err)
	}
	defer os.Remove(link)
	for _, dir := range []string{link, filepath.Join(link, "missing")} {
		if _, err := LoadSettings(dir); err == nil {
			t.Fatal("substituted path treated as defaults")
		}
		if _, err := LoadAuthorization(dir); err == nil || errors.Is(err, ErrAuthorizationMissing) {
			t.Fatal("substituted path treated as missing auth")
		}
	}
	if err := SaveSettings(link, DefaultSettings()); err == nil {
		t.Fatal("substituted directory written")
	}
	s, err := LoadSettings(target)
	if err != nil || s.Model != "unchanged" {
		t.Fatal("target changed", err)
	}
	if _, err := os.Lstat(filepath.Join(target, "missing")); !os.IsNotExist(err) {
		t.Fatal("missing directory created", err)
	}
}
func TestSettingsRejectBroadFileWithoutRepair(t *testing.T) {
	dir := cleanEnv(t)
	broad := filepath.Join(t.TempDir(), "broad-settings")
	if err := os.WriteFile(broad, []byte(`{"model":"secret-canary"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(broad, 0644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "settings.json")
	if err := os.Rename(broad, path); err != nil {
		t.Fatal(err)
	}
	if privatefs.CheckFile(path, 16<<10) == nil {
		t.Fatal("broad fixture unexpectedly private")
	}
	if _, err := LoadSettings(dir); err == nil || strings.Contains(err.Error(), "secret-canary") {
		t.Fatal("unsafe load accepted or leaked", err)
	}
	if err := SaveSettings(dir, DefaultSettings()); err == nil {
		t.Fatal("unsafe settings replaced")
	}
	if privatefs.CheckFile(path, 16<<10) == nil {
		t.Fatal("permissions repaired")
	}
	data, _ := os.ReadFile(path)
	if string(data) != `{"model":"secret-canary"}` {
		t.Fatal("unsafe target changed")
	}
}

func TestSaveSettingsFailedReplacementKeepsOldWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows open handle replacement failure only")
	}
	dir := cleanEnv(t)
	s := DefaultSettings()
	if err := InitSettings(dir, s); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "settings.json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Windows os.Open does not share delete, so the final Rename must fail.
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	s.Model = "replacement"
	if err = SaveSettings(dir, s); err == nil {
		t.Fatal("replacement unexpectedly succeeded with delete sharing denied")
	}
	after, err := os.ReadFile(path)
	if err != nil || string(before) != string(after) {
		t.Fatal("old settings lost on replacement failure", err)
	}
	names, err := filepath.Glob(filepath.Join(dir, ".settings-*.tmp"))
	if err != nil || len(names) != 0 {
		t.Fatal("temporary settings leaked after failure", err)
	}
}

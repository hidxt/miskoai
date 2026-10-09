package config

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/hidxt/miskoai/internal/channel/weixin"
	"github.com/hidxt/miskoai/internal/privatefs"
	"github.com/hidxt/miskoai/internal/provider"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"unicode/utf8"
)

var ErrSettings = errors.New("invalid or unsafe settings")

type Settings struct {
	Listen        string `json:"listen"`
	DeepSeekURL   string `json:"deepseek_url"`
	Model         string `json:"model"`
	VisionModel   string `json:"vision_model"`
	WeixinURL     string `json:"weixin_url"`
	MaxOutput     int    `json:"max_output_tokens"`
	ContextBytes  int    `json:"context_bytes"`
	ContextTokens int    `json:"context_tokens"`
}

func DefaultSettings() Settings {
	return Settings{Listen: "127.0.0.1:8787", DeepSeekURL: "https://api.deepseek.com", Model: "deepseek-flash", VisionModel: "deepseek-flash", WeixinURL: weixin.DefaultBaseURL, MaxOutput: 512, ContextBytes: 24576, ContextTokens: 32768}
}
func validText(s string, max int) bool {
	if !utf8.ValidString(s) || len(s) > max || strings.TrimSpace(s) == "" {
		return false
	}
	for _, r := range s {
		if r < 32 || r == 127 {
			return false
		}
	}
	return true
}
func validateSettings(s Settings) error {
	host, port, err := net.SplitHostPort(s.Listen)
	p, e := strconv.Atoi(port)
	if err != nil || e != nil || p < 1 || p > 65535 || (host != "127.0.0.1" && host != "::1") {
		return ErrSettings
	}
	if !validText(s.Model, 128) || !validText(s.VisionModel, 128) || !validText(s.DeepSeekURL, 2048) || !validText(s.WeixinURL, 2048) || s.MaxOutput < 1 || s.MaxOutput > 4096 || s.ContextBytes < 16384 || s.ContextBytes > 49152 || s.ContextTokens < 4096 || s.ContextTokens > 65536 {
		return ErrSettings
	}
	if _, err = provider.NewDeepSeek(s.DeepSeekURL, "", s.Model); err != nil {
		return ErrSettings
	}
	if _, err = provider.NewDeepSeek(s.DeepSeekURL, "", s.VisionModel); err != nil {
		return ErrSettings
	}
	if _, err = weixin.New(s.WeixinURL, ""); err != nil {
		return ErrSettings
	}
	return nil
}

// absentPrivateFile distinguishes absence from unsafe parents before reading.
// Missing chains are inspected without creating or repairing them.
func absentPrivateFile(dir, name string) (bool, error) {
	if !validText(dir, 32768) {
		return false, privatefs.ErrUnsafe
	}
	abs, err := privatefs.Resolve(dir)
	if err != nil {
		return false, privatefs.ErrUnsafe
	}
	for p := abs; ; p = filepath.Dir(p) {
		info, e := os.Lstat(p)
		if e == nil {
			if !info.IsDir() || info.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
				return false, privatefs.ErrUnsafe
			}
			resolved, e := filepath.EvalSymlinks(p)
			if e != nil {
				return false, privatefs.ErrUnsafe
			}
			equal := filepath.Clean(resolved) == filepath.Clean(p)
			if runtime.GOOS == "windows" {
				equal = strings.EqualFold(filepath.Clean(resolved), filepath.Clean(p))
			}
			if !equal {
				return false, privatefs.ErrUnsafe
			}
		} else if !errors.Is(e, os.ErrNotExist) {
			return false, privatefs.ErrUnsafe
		}
		if next := filepath.Dir(p); next == p {
			break
		}
	}
	if _, err = os.Lstat(abs); errors.Is(err, os.ErrNotExist) {
		return true, nil
	} else if err != nil {
		return false, privatefs.ErrUnsafe
	}
	if err = privatefs.CheckDir(abs); err != nil {
		return false, err
	}
	if _, err = os.Lstat(filepath.Join(abs, name)); errors.Is(err, os.ErrNotExist) {
		return true, nil
	} else if err != nil {
		return false, privatefs.ErrUnsafe
	}
	return false, nil
}

// The standard decoder replaces unpaired UTF-16 escapes; reject them first
// so private strings and settings are never silently rewritten.
func validEscapes(data []byte) bool {
	inString := false
	for i := 0; i < len(data); i++ {
		if data[i] == '"' {
			inString = !inString
			continue
		}
		if !inString || data[i] != '\\' {
			continue
		}
		i++
		if i >= len(data) {
			return false
		}
		if data[i] != 'u' {
			continue
		}
		if i+4 >= len(data) {
			return false
		}
		n, err := strconv.ParseUint(string(data[i+1:i+5]), 16, 16)
		if err != nil {
			return false
		}
		i += 4
		if n >= 0xdc00 && n <= 0xdfff {
			return false
		}
		if n >= 0xd800 && n <= 0xdbff {
			if i+6 >= len(data) || data[i+1] != '\\' || data[i+2] != 'u' {
				return false
			}
			low, e := strconv.ParseUint(string(data[i+3:i+7]), 16, 16)
			if e != nil || low < 0xdc00 || low > 0xdfff {
				return false
			}
			i += 6
		}
	}
	return !inString
}
func strictObject(data []byte, fields map[string]any) error {
	if !utf8.Valid(data) || !validEscapes(data) {
		return ErrSettings
	}
	d := json.NewDecoder(bytes.NewReader(data))
	tok, err := d.Token()
	if err != nil || tok != json.Delim('{') {
		return ErrSettings
	}
	seen := make(map[string]bool, len(fields))
	for d.More() {
		tok, err = d.Token()
		if err != nil {
			return ErrSettings
		}
		key, ok := tok.(string)
		dest, known := fields[key]
		if !ok || !known || seen[key] {
			return ErrSettings
		}
		seen[key] = true
		var raw json.RawMessage
		if d.Decode(&raw) != nil || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) || json.Unmarshal(raw, dest) != nil {
			return ErrSettings
		}
	}
	tok, err = d.Token()
	if err != nil || tok != json.Delim('}') {
		return ErrSettings
	}
	if _, err = d.Token(); err != io.EOF {
		return ErrSettings
	}
	return nil
}
func LoadSettings(dataDir string) (Settings, error) {
	missing, err := absentPrivateFile(dataDir, "settings.json")
	if err != nil {
		return Settings{}, ErrSettings
	}
	s := DefaultSettings()
	if missing {
		return s, nil
	}
	data, err := privatefs.Read(filepath.Join(dataDir, "settings.json"), 16<<10)
	if err != nil {
		return Settings{}, ErrSettings
	}
	fields := map[string]any{"listen": &s.Listen, "deepseek_url": &s.DeepSeekURL, "model": &s.Model, "vision_model": &s.VisionModel, "weixin_url": &s.WeixinURL, "max_output_tokens": &s.MaxOutput, "context_bytes": &s.ContextBytes, "context_tokens": &s.ContextTokens}
	if strictObject(data, fields) != nil || validateSettings(s) != nil {
		return Settings{}, ErrSettings
	}
	return s, nil
}
func settingsBytes(s Settings) ([]byte, error) {
	if validateSettings(s) != nil {
		return nil, ErrSettings
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return nil, ErrSettings
	}
	return append(b, '\n'), nil
}
func writeSettingsFile(f *os.File, data []byte) error {
	_, err := f.Write(data)
	syncErr := f.Sync()
	closeErr := f.Close()
	if err != nil || syncErr != nil || closeErr != nil {
		return ErrSettings
	}
	return nil
}
func InitSettings(dataDir string, s Settings) error {
	if !validText(dataDir, 32768) {
		return ErrSettings
	}
	dataDir, err := privatefs.Resolve(dataDir)
	if err != nil {
		return ErrSettings
	}
	data, err := settingsBytes(s)
	if err != nil {
		return err
	}
	if privatefs.EnsureDir(dataDir) != nil {
		return ErrSettings
	}
	path := filepath.Join(dataDir, "settings.json")
	f, err := privatefs.Create(path)
	if err != nil {
		return ErrSettings
	}
	if err = writeSettingsFile(f, data); err != nil {
		os.Remove(path)
		return err
	}
	return nil
}
func SaveSettings(dataDir string, s Settings) error {
	if !validText(dataDir, 32768) {
		return ErrSettings
	}
	dataDir, err := privatefs.Resolve(dataDir)
	if err != nil {
		return ErrSettings
	}
	data, err := settingsBytes(s)
	if err != nil {
		return err
	}
	if privatefs.EnsureDir(dataDir) != nil {
		return ErrSettings
	}
	path := filepath.Join(dataDir, "settings.json")
	missing, err := absentPrivateFile(dataDir, "settings.json")
	if err != nil || (!missing && privatefs.CheckFile(path, 16<<10) != nil) {
		return ErrSettings
	}
	var id [16]byte
	if _, err = rand.Read(id[:]); err != nil {
		return ErrSettings
	}
	temp := filepath.Join(dataDir, ".settings-"+hex.EncodeToString(id[:])+".tmp")
	f, err := privatefs.Create(temp)
	if err != nil {
		return ErrSettings
	}
	defer os.Remove(temp)
	if err = writeSettingsFile(f, data); err != nil {
		return err
	}
	// Recheck the destination immediately before atomic replacement.
	missing, err = absentPrivateFile(dataDir, "settings.json")
	if err != nil || (!missing && privatefs.CheckFile(path, 16<<10) != nil) {
		return ErrSettings
	}
	// Same-directory Rename is atomic on the native Linux deployment targets.
	// Go does not guarantee atomic replacement on Windows.
	if os.Rename(temp, path) != nil {
		return ErrSettings
	}
	return nil
}

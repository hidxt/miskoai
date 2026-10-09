package config

import (
	"errors"
	"os"
	"strconv"

	"github.com/hidxt/miskoai/internal/privatefs"
)

type Config struct {
	Listen        string          `json:"listen"`
	DataDir       string          `json:"-"`
	DeepSeekURL   string          `json:"deepseek_url"`
	Model         string          `json:"model"`
	VisionModel   string          `json:"vision_model"`
	WeixinURL     string          `json:"weixin_url"`
	MaxOutput     int             `json:"max_output_tokens"`
	ContextBytes  int             `json:"context_bytes"`
	ContextTokens int             `json:"context_tokens"`
	Overrides     map[string]bool `json:"overrides"`
	DeepSeekKey   string          `json:"-"`
	OllamaKey     string          `json:"-"`
	AdminPassword string          `json:"-"`
	desired       Settings
	effective     Settings
}

func (c Config) DesiredSettings() Settings   { return c.desired }
func (c Config) EffectiveSettings() Settings { return c.effective }
func Load() (Config, error) {
	dir := "./data"
	if value, ok := os.LookupEnv("MISKOAI_DATA_DIR"); ok {
		dir = value
	}
	if !validText(dir, 32768) {
		return Config{}, errors.New("invalid data directory")
	}
	abs, err := privatefs.Resolve(dir)
	if err != nil {
		return Config{}, errors.New("invalid data directory")
	}
	desired, err := LoadSettings(abs)
	if err != nil {
		return Config{}, err
	}
	s := desired
	overrides := make(map[string]bool)
	for _, field := range []struct {
		name, key string
		dest      *string
	}{{"MISKOAI_LISTEN", "listen", &s.Listen}, {"MISKOAI_DEEPSEEK_URL", "deepseek_url", &s.DeepSeekURL}, {"MISKOAI_MODEL", "model", &s.Model}, {"MISKOAI_VISION_MODEL", "vision_model", &s.VisionModel}, {"MISKOAI_WEIXIN_URL", "weixin_url", &s.WeixinURL}} {
		if v, ok := os.LookupEnv(field.name); ok {
			*field.dest = v
			overrides[field.key] = true
		}
	}
	for _, field := range []struct {
		name, key string
		dest      *int
	}{{"MISKOAI_MAX_OUTPUT_TOKENS", "max_output_tokens", &s.MaxOutput}, {"MISKOAI_CONTEXT_BYTES", "context_bytes", &s.ContextBytes}, {"MISKOAI_CONTEXT_TOKENS", "context_tokens", &s.ContextTokens}} {
		if v, ok := os.LookupEnv(field.name); ok {
			*field.dest, err = strconv.Atoi(v)
			if err != nil {
				return Config{}, ErrSettings
			}
			overrides[field.key] = true
		}
	}
	if validateSettings(s) != nil {
		return Config{}, ErrSettings
	}
	deep, search, err := loadKeys()
	if err != nil {
		return Config{}, err
	}
	password, present := os.LookupEnv("MISKOAI_ADMIN_PASSWORD")
	if present && !validText(password, 256) {
		return Config{}, ErrCredentials
	}
	return Config{Listen: s.Listen, DataDir: abs, DeepSeekURL: s.DeepSeekURL, Model: s.Model, VisionModel: s.VisionModel, WeixinURL: s.WeixinURL, MaxOutput: s.MaxOutput, ContextBytes: s.ContextBytes, ContextTokens: s.ContextTokens, Overrides: overrides, DeepSeekKey: deep, OllamaKey: search, AdminPassword: password, desired: desired, effective: s}, nil
}

package config

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/hidxt/miskoai/internal/channel/weixin"
	"github.com/hidxt/miskoai/internal/provider"
)

type Config struct {
	Listen        string `json:"listen"`
	DataDir       string `json:"-"`
	DeepSeekURL   string `json:"deepseek_url"`
	Model         string `json:"model"`
	VisionModel   string `json:"vision_model"`
	WeixinURL     string `json:"weixin_url"`
	MaxOutput     int    `json:"max_output_tokens"`
	DeepSeekKey   string `json:"-"`
	OllamaKey     string `json:"-"`
	AdminPassword string `json:"-"`
}

func env(name, fallback string) string {
	if value, ok := os.LookupEnv(name); ok {
		return value
	}
	return fallback
}
func Load() (Config, error) {
	c := Config{Listen: env("MISKOAI_LISTEN", "127.0.0.1:8787"), DataDir: env("MISKOAI_DATA_DIR", "./data"), DeepSeekURL: env("MISKOAI_DEEPSEEK_URL", "https://api.deepseek.com"), Model: env("MISKOAI_MODEL", "deepseek-flash"), VisionModel: env("MISKOAI_VISION_MODEL", "deepseek-flash"), WeixinURL: env("MISKOAI_WEIXIN_URL", weixin.DefaultBaseURL), DeepSeekKey: os.Getenv("DEEPSEEK_API_KEY"), OllamaKey: os.Getenv("OLLAMA_API_KEY"), AdminPassword: os.Getenv("MISKOAI_ADMIN_PASSWORD"), MaxOutput: 512}
	host, port, err := net.SplitHostPort(c.Listen)
	p, e := strconv.Atoi(port)
	if err != nil || e != nil || p < 1 || p > 65535 || (host != "127.0.0.1" && host != "::1") {
		return Config{}, errors.New("management listener must be a loopback IP and valid port")
	}
	if strings.TrimSpace(c.DataDir) == "" {
		return Config{}, errors.New("data directory is required")
	}
	c.DataDir, err = filepath.Abs(c.DataDir)
	if err != nil {
		return Config{}, errors.New("invalid data directory")
	}
	if value := os.Getenv("MISKOAI_MAX_OUTPUT_TOKENS"); value != "" {
		c.MaxOutput, err = strconv.Atoi(value)
		if err != nil || c.MaxOutput < 1 || c.MaxOutput > 4096 {
			return Config{}, errors.New("invalid output token budget")
		}
	}
	if _, err = provider.NewDeepSeek(c.DeepSeekURL, c.DeepSeekKey, c.Model); err != nil {
		return Config{}, err
	}
	if _, err = provider.NewDeepSeek(c.DeepSeekURL, c.DeepSeekKey, c.VisionModel); err != nil {
		return Config{}, err
	}
	if _, err = provider.NewSearch(c.OllamaKey); err != nil {
		return Config{}, err
	}
	if _, err = weixin.New(c.WeixinURL, ""); err != nil {
		return Config{}, err
	}
	return c, nil
}

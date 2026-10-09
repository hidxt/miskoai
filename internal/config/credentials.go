package config

import (
	"errors"
	"github.com/hidxt/miskoai/internal/channel/weixin"
	"github.com/hidxt/miskoai/internal/privatefs"
	"os"
	"path/filepath"
)

var (
	ErrCredentials          = errors.New("invalid or unsafe credentials")
	ErrAuthorizationMissing = errors.New("private WeChat authorization is missing")
	ErrAuthorization        = errors.New("invalid or unsafe private WeChat authorization")
)

func loadKeys() (string, string, error) {
	deep, deepPresent := os.LookupEnv("DEEPSEEK_API_KEY")
	search, searchPresent := os.LookupEnv("OLLAMA_API_KEY")
	path, pathPresent := os.LookupEnv("MISKOAI_CREDENTIALS_FILE")
	if (deepPresent && !validText(deep, 1024)) || (searchPresent && !validText(search, 1024)) || (pathPresent && !validText(path, 32768)) {
		return "", "", ErrCredentials
	}
	if pathPresent && (!deepPresent || !searchPresent) {
		data, err := privatefs.Read(path, 8<<10)
		if err != nil {
			return "", "", ErrCredentials
		}
		var fileDeep, fileSearch string
		if strictObject(data, map[string]any{"DEEPSEEK_API_KEY": &fileDeep, "OLLAMA_API_KEY": &fileSearch}) != nil || !validText(fileDeep, 1024) || !validText(fileSearch, 1024) {
			return "", "", ErrCredentials
		}
		if !deepPresent {
			deep = fileDeep
		}
		if !searchPresent {
			search = fileSearch
		}
	}
	return deep, search, nil
}

// Authorization is private input only. It cannot marshal into public settings.
type Authorization struct {
	Token       string `json:"-"`
	Account     string `json:"-"`
	AllowedUser string `json:"-"`
	BaseURL     string `json:"-"`
}

func LoadAuthorization(dataDir string) (Authorization, error) {
	missing, err := absentPrivateFile(dataDir, "weixin-auth.json")
	if err != nil {
		return Authorization{}, ErrAuthorization
	}
	if missing {
		return Authorization{}, ErrAuthorizationMissing
	}
	data, err := privatefs.Read(filepath.Join(dataDir, "weixin-auth.json"), 64<<10)
	if err != nil {
		return Authorization{}, ErrAuthorization
	}
	var a Authorization
	if strictObject(data, map[string]any{"bot_token": &a.Token, "account": &a.Account, "allowed_user": &a.AllowedUser, "base_url": &a.BaseURL}) != nil || !validText(a.Token, 16<<10) || !validText(a.Account, 256) || !validText(a.AllowedUser, 256) || !validText(a.BaseURL, 2048) {
		return Authorization{}, ErrAuthorization
	}
	if _, err = weixin.New(a.BaseURL, a.Token); err != nil {
		return Authorization{}, ErrAuthorization
	}
	return a, nil
}

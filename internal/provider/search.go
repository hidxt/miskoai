package provider

import (
	"context"
	"errors"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/hidxt/miskoai/internal/netx"
)

type SearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Content string `json:"content"`
}
type Search struct {
	transport *netx.Client
	key       string
}

func NewSearch(key string) (*Search, error) {
	if strings.ContainsAny(key, "\r\n") || len(key) > 1024 {
		return nil, errors.New("invalid search configuration")
	}
	c, err := netx.New("https://ollama.com", []string{"ollama.com"}, 30*time.Second, 2<<20)
	if err != nil {
		return nil, err
	}
	return &Search{c, key}, nil
}
func (s *Search) Search(ctx context.Context, query string, max int) ([]SearchResult, error) {
	query = strings.TrimSpace(query)
	if s.key == "" {
		return nil, errors.New("Ollama key is not configured")
	}
	if query == "" || len(query) > 512 || max < 1 || max > 10 {
		return nil, errors.New("invalid search bounds")
	}
	var result struct {
		Results []SearchResult `json:"results"`
	}
	err := retrySafeStatus(ctx, func(attemptCtx context.Context) error {
		return s.transport.JSON(attemptCtx, "POST", "/api/web_search", s.key, map[string]any{"query": query, "max_results": max}, &result)
	})
	if err != nil {
		return nil, err
	}
	if len(result.Results) > max {
		return nil, errors.New("search result count exceeded")
	}
	filtered := make([]SearchResult, 0, len(result.Results))
	for _, r := range result.Results {
		if !SafeSourceURL(r.URL) {
			continue
		}
		if len(r.Title) > 1024 || len(r.Content) > 16<<10 {
			return nil, netx.ErrLimit
		}
		filtered = append(filtered, r)
	}
	return filtered, nil
}

// Source links are displayed as citations only; MiskoAI does not fetch them.
func SafeSourceURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || len(raw) > 4096 || u.Scheme != "https" || u.User != nil || u.Port() != "" && u.Port() != "443" {
		return false
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if host == "" || host == "localhost" || !strings.Contains(host, ".") || strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".localhost") {
		return false
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		return netx.PublicIP(ip)
	}
	return true
}

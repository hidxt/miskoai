package config

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTrialAdminUsername(t *testing.T) {
	cleanEnv(t)
	c, e := Load()
	if e != nil || c.AdminUsername != "admin" {
		t.Fatal("missing username must default to admin", e)
	}
	t.Setenv("MISKOAI_ADMIN_USERNAME", "owner-canary")
	c, e = Load()
	if e != nil {
		t.Fatal(e)
	}
	if c.AdminUsername != "owner-canary" {
		t.Fatal("private owner username missing")
	}
	b, _ := json.Marshal(c)
	if strings.Contains(string(b), "owner-canary") {
		t.Fatal("username exposed")
	}
	for _, v := range []string{"", " owner", "owner ", "x\n", "x\u0085", strings.Repeat("界", 22)} {
		t.Setenv("MISKOAI_ADMIN_USERNAME", v)
		if _, e := Load(); e == nil {
			t.Fatal("bad username accepted")
		}
	}
}

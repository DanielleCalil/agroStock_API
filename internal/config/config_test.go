package config

import "testing"

func TestCookieSecureAutomatico(t *testing.T) {
	casos := []struct {
		nome         string
		origem       string
		cookieSecure string // "" = não definido
		quero        bool
	}{
		{"front https sem COOKIE_SECURE", "https://app.vercel.app", "", true},
		{"front http sem COOKIE_SECURE", "http://localhost:5173", "", false},
		{"COOKIE_SECURE=false explícito vence", "https://app.vercel.app", "false", false},
		{"COOKIE_SECURE=true explícito", "http://localhost:5173", "true", true},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			t.Setenv("DATABASE_URL", "sqlserver://x")
			t.Setenv("ALLOWED_ORIGIN", c.origem)
			t.Setenv("COOKIE_SECURE", c.cookieSecure)
			cfg, err := Load()
			if err != nil {
				t.Fatal(err)
			}
			if cfg.CookieSecure != c.quero {
				t.Errorf("CookieSecure = %v, quero %v", cfg.CookieSecure, c.quero)
			}
		})
	}
}

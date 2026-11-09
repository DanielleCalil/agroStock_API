// Package config lê as configurações da API a partir de variáveis de ambiente.
package config

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	// Endereço onde o servidor HTTP escuta (ex: ":8080").
	Addr string
	// String de conexão do SQL Server, ex:
	// sqlserver://usuario:senha@localhost:1433?database=agrostock
	DatabaseURL string
	// Origens do frontend autorizadas a chamar a API com cookies (CORS).
	// Lidas de ALLOWED_ORIGIN, separadas por vírgula.
	// Vazio = CORS desativado (frontend servido pela mesma origem).
	AllowedOrigins []string
	// Marca o cookie de sessão como Secure (exige HTTPS). Use true em produção.
	CookieSecure bool
	// Tempo de vida de uma sessão.
	SessionTTL time.Duration
	// Devolve o token de redefinição de senha na resposta do POST
	// /senha/esqueci. Só para desenvolvimento: a API ainda não envia e-mail,
	// e em produção isso permitiria trocar a senha de qualquer conta.
	ResetTokenNaResposta bool
}

func Load() (Config, error) {
	cfg := Config{
		Addr:        getenv("API_ADDR", ":8080"),
		DatabaseURL: os.Getenv("DATABASE_URL"),
		SessionTTL:  8 * time.Hour,
	}
	for _, o := range strings.Split(os.Getenv("ALLOWED_ORIGIN"), ",") {
		if o = strings.TrimSpace(o); o != "" {
			cfg.AllowedOrigins = append(cfg.AllowedOrigins, o)
		}
	}
	if cfg.DatabaseURL == "" {
		return cfg, errors.New("DATABASE_URL não definida (veja .env.example)")
	}
	if v := os.Getenv("COOKIE_SECURE"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return cfg, errors.New("COOKIE_SECURE deve ser true ou false")
		}
		cfg.CookieSecure = b
	}
	if v := os.Getenv("SESSION_TTL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			return cfg, errors.New("SESSION_TTL inválido (ex: 8h, 30m)")
		}
		cfg.SessionTTL = d
	}
	if v := os.Getenv("RESET_TOKEN_NA_RESPOSTA"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return cfg, errors.New("RESET_TOKEN_NA_RESPOSTA deve ser true ou false")
		}
		cfg.ResetTokenNaResposta = b
	}
	return cfg, nil
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

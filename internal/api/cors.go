package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

// CORSConfig define quais origens podem chamar a API pelo navegador.
type CORSConfig struct {
	// Origens autorizadas (ex: "http://localhost:5173"). Vazio = CORS desativado.
	Origens []string
	// Métodos aceitos no preflight.
	Metodos []string
	// Cabeçalhos que o frontend pode enviar.
	Cabecalhos []string
	// Envia Access-Control-Allow-Credentials (necessário para o cookie de sessão).
	Credenciais bool
	// Por quanto tempo o navegador pode guardar a resposta do preflight.
	MaxAge time.Duration
}

// CORS devolve um middleware que libera as origens configuradas a chamar a
// API, inclusive enviando cookies. Requisições de origens não autorizadas
// seguem sem os cabeçalhos CORS, e o próprio navegador bloqueia a resposta.
func CORS(cfg CORSConfig) func(http.Handler) http.Handler {
	permitidas := make(map[string]struct{}, len(cfg.Origens))
	for _, o := range cfg.Origens {
		o = strings.TrimRight(strings.TrimSpace(o), "/")
		if o != "" {
			permitidas[o] = struct{}{}
		}
	}
	metodos := strings.Join(cfg.Metodos, ", ")
	cabecalhos := strings.Join(cfg.Cabecalhos, ", ")
	maxAge := strconv.Itoa(int(cfg.MaxAge.Seconds()))

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origem := r.Header.Get("Origin")
			if origem == "" {
				next.ServeHTTP(w, r) // não é uma requisição CORS
				return
			}

			h := w.Header()
			h.Add("Vary", "Origin")

			preflight := r.Method == http.MethodOptions &&
				r.Header.Get("Access-Control-Request-Method") != ""
			if preflight {
				h.Add("Vary", "Access-Control-Request-Method")
				h.Add("Vary", "Access-Control-Request-Headers")
			}

			_, ok := permitidas[origem]
			if !ok {
				if preflight {
					// Sem cabeçalhos CORS: o navegador recusa a requisição real.
					w.WriteHeader(http.StatusNoContent)
					return
				}
				next.ServeHTTP(w, r)
				return
			}

			h.Set("Access-Control-Allow-Origin", origem)
			if cfg.Credenciais {
				h.Set("Access-Control-Allow-Credentials", "true")
			}

			if preflight {
				h.Set("Access-Control-Allow-Methods", metodos)
				if cabecalhos != "" {
					h.Set("Access-Control-Allow-Headers", cabecalhos)
				}
				if cfg.MaxAge > 0 {
					h.Set("Access-Control-Max-Age", maxAge)
				}
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

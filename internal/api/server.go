// Package api contém as rotas HTTP da agroStock API.
package api

import (
	"database/sql"
	"log"
	"net/http"
	"time"

	"github.com/DanielleCalil/agroStock_API/internal/config"
	"github.com/DanielleCalil/agroStock_API/internal/sessao"
)

type Server struct {
	db      *sql.DB
	sessoes *sessao.Store
	cfg     config.Config
	hoje    func() time.Time // injetável para testes do motor de alertas
}

func NewServer(db *sql.DB, sessoes *sessao.Store, cfg config.Config) *Server {
	return &Server{db: db, sessoes: sessoes, cfg: cfg, hoje: time.Now}
}

// Handler monta todas as rotas com os middlewares de log e recuperação.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// Públicas
	mux.HandleFunc("GET /health", s.health)
	mux.HandleFunc("POST /sessao", s.login)
	mux.HandleFunc("POST /usuarios", s.cadastrarUsuario)
	mux.HandleFunc("POST /senha/esqueci", s.esqueciSenha)
	mux.HandleFunc("POST /senha/redefinir", s.redefinirSenha)

	// Protegidas: sem o cookie de sessão válido => 401
	mux.Handle("GET /sessao", s.autenticado(s.sessaoAtual))
	mux.Handle("DELETE /sessao", s.autenticado(s.logout))

	mux.Handle("POST /insumos", s.autenticado(s.criarInsumo))
	mux.Handle("GET /insumos", s.autenticado(s.listarInsumos))

	mux.Handle("POST /lotes", s.autenticado(s.criarLote))
	mux.Handle("GET /lotes", s.autenticado(s.listarLotes))

	mux.Handle("POST /movimentacoes", s.autenticado(s.registrarMovimentacao))
	mux.Handle("GET /movimentacoes", s.autenticado(s.listarMovimentacoes))

	mux.Handle("GET /saldos", s.autenticado(s.listarSaldos))
	mux.Handle("GET /alertas", s.autenticado(s.listarAlertas))

	// O CORS é aplicado por fora, no main().
	return recuperar(logRequisicoes(mux))
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	if err := s.db.PingContext(r.Context()); err != nil {
		escreverErro(w, http.StatusServiceUnavailable, "banco de dados indisponível")
		return
	}
	escreverJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func logRequisicoes(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		inicio := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		log.Printf("%s %s -> %d (%s)", r.Method, r.URL.Path, rec.status, time.Since(inicio).Round(time.Millisecond))
	})
}

func recuperar(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				if v == http.ErrAbortHandler {
					panic(v)
				}
				log.Printf("panic em %s %s: %v", r.Method, r.URL.Path, v)
				escreverErro(w, http.StatusInternalServerError, "erro interno")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

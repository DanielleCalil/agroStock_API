package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func handlerCORSTeste() http.Handler {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	return CORS(CORSConfig{
		Origens:     []string{"http://localhost:5173", "https://app.exemplo.com/"},
		Metodos:     []string{"GET", "POST", "DELETE", "OPTIONS"},
		Cabecalhos:  []string{"Content-Type"},
		Credenciais: true,
		MaxAge:      10 * time.Minute,
	})(ok)
}

func TestCORSOrigemPermitida(t *testing.T) {
	for _, origem := range []string{"http://localhost:5173", "https://app.exemplo.com"} {
		req := httptest.NewRequest(http.MethodGet, "/insumos", nil)
		req.Header.Set("Origin", origem)
		rec := httptest.NewRecorder()
		handlerCORSTeste().ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status = %d, quero 200", origem, rec.Code)
		}
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != origem {
			t.Errorf("%s: Allow-Origin = %q", origem, got)
		}
		if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
			t.Errorf("%s: Allow-Credentials = %q", origem, got)
		}
	}
}

func TestCORSOrigemNegada(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/insumos", nil)
	req.Header.Set("Origin", "https://malicioso.com")
	rec := httptest.NewRecorder()
	handlerCORSTeste().ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Allow-Origin = %q, quero vazio", got)
	}
}

func TestCORSPreflight(t *testing.T) {
	req := httptest.NewRequest(http.MethodOptions, "/insumos", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	req.Header.Set("Access-Control-Request-Method", "POST")
	rec := httptest.NewRecorder()
	handlerCORSTeste().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, quero 204", rec.Code)
	}
	h := rec.Header()
	if h.Get("Access-Control-Allow-Methods") != "GET, POST, DELETE, OPTIONS" {
		t.Errorf("Allow-Methods = %q", h.Get("Access-Control-Allow-Methods"))
	}
	if h.Get("Access-Control-Allow-Headers") != "Content-Type" {
		t.Errorf("Allow-Headers = %q", h.Get("Access-Control-Allow-Headers"))
	}
	if h.Get("Access-Control-Max-Age") != "600" {
		t.Errorf("Max-Age = %q", h.Get("Access-Control-Max-Age"))
	}
}

func TestCORSPreflightOrigemNegada(t *testing.T) {
	req := httptest.NewRequest(http.MethodOptions, "/insumos", nil)
	req.Header.Set("Origin", "https://malicioso.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	rec := httptest.NewRecorder()
	handlerCORSTeste().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, quero 204", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Allow-Origin = %q, quero vazio", got)
	}
}

func TestCORSSemOrigin(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	handlerCORSTeste().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK || rec.Header().Get("Vary") != "" {
		t.Errorf("requisição sem Origin não deveria ser alterada")
	}
}

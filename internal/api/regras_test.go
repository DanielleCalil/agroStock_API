package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DanielleCalil/agroStock_API/internal/config"
	"github.com/DanielleCalil/agroStock_API/internal/sessao"
)

func TestClassificarValidade(t *testing.T) {
	casos := []struct {
		dias  int
		nivel string
		ok    bool
	}{
		{-10, nivelVermelho, true}, // vencido
		{0, nivelVermelho, true},
		{7, nivelVermelho, true},
		{8, nivelAmarelo, true},
		{30, nivelAmarelo, true},
		{31, "", false},
	}
	for _, c := range casos {
		nivel, ok := classificarValidade(c.dias)
		if nivel != c.nivel || ok != c.ok {
			t.Errorf("classificarValidade(%d) = %q,%v; quer %q,%v", c.dias, nivel, ok, c.nivel, c.ok)
		}
	}
}

func TestDiasEntreIgnoraHorario(t *testing.T) {
	// 23h do dia 6 no fuso de São Paulo ainda é dia 6 para o usuário.
	sp := time.FixedZone("BRT", -3*3600)
	hoje := time.Date(2026, 11, 6, 23, 30, 0, 0, sp)
	validade := time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC) // DATE vindo do banco
	if d := diasEntre(hoje, validade); d != 7 {
		t.Fatalf("diasEntre = %d; quer 7", d)
	}
	if d := diasEntre(hoje, time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)); d != -1 {
		t.Fatalf("diasEntre (vencido) = %d; quer -1", d)
	}
}

func TestValidarInsumo(t *testing.T) {
	neg, zero := -1.0, 0.0
	base := func() novoInsumo {
		return novoInsumo{Nome: "Glifosato", Categoria: "Herbicida", Unidade: "L", Minimo: &zero}
	}
	if msg := (func() string { n := base(); return n.validar() })(); msg != "" {
		t.Fatalf("insumo válido rejeitado: %s", msg)
	}
	n := base()
	n.Minimo = &neg
	if msg := n.validar(); msg != "minimo deve ser maior ou igual a 0" {
		t.Fatalf("minimo negativo: %q", msg)
	}
	n = base()
	n.Minimo = nil
	if msg := n.validar(); msg == "" {
		t.Fatal("minimo ausente deveria ser rejeitado")
	}
	n = base()
	n.Nome = "   "
	if msg := n.validar(); msg == "" {
		t.Fatal("nome em branco deveria ser rejeitado")
	}
}

func TestValidarLote(t *testing.T) {
	for _, v := range []string{"2026-02-30", "06/11/2026", "2026-13-01", ""} {
		n := novoLote{InsumoID: 1, Codigo: "L1", Validade: v}
		if msg := n.validar(); msg == "" {
			t.Errorf("validade %q deveria ser rejeitada", v)
		}
	}
	n := novoLote{InsumoID: 1, Codigo: " L1 ", Validade: "2027-01-31"}
	if msg := n.validar(); msg != "" || n.Codigo != "L1" {
		t.Fatalf("lote válido rejeitado: %q (codigo=%q)", msg, n.Codigo)
	}
}

func TestValidarMovimentacao(t *testing.T) {
	ok := novaMovimentacao{LoteID: 1, Tipo: "saida", Quantidade: 50.12345, ChaveRequisicao: "abc"}
	if msg := ok.validar(); msg != "" {
		t.Fatalf("movimentação válida rejeitada: %s", msg)
	}
	if ok.Tipo != "S" || ok.Quantidade != 50.123 {
		t.Fatalf("normalização errada: tipo=%q qtd=%v", ok.Tipo, ok.Quantidade)
	}
	ruins := []novaMovimentacao{
		{LoteID: 1, Tipo: "X", Quantidade: 1, ChaveRequisicao: "a"},
		{LoteID: 1, Tipo: "E", Quantidade: 0, ChaveRequisicao: "a"},
		{LoteID: 1, Tipo: "E", Quantidade: -5, ChaveRequisicao: "a"},
		{LoteID: 1, Tipo: "E", Quantidade: 0.0001, ChaveRequisicao: "a"}, // arredonda para 0
		{LoteID: 1, Tipo: "E", Quantidade: 1, ChaveRequisicao: ""},
		{LoteID: 0, Tipo: "E", Quantidade: 1, ChaveRequisicao: "a"},
	}
	for i, m := range ruins {
		if msg := m.validar(); msg == "" {
			t.Errorf("caso %d deveria ser rejeitado: %+v", i, m)
		}
	}
}

func novoServidorTeste() *Server {
	return NewServer(nil, sessao.NewStore(time.Hour), config.Config{AllowedOrigin: "http://localhost:5173"})
}

func TestRotasProtegidasExigemSessao(t *testing.T) {
	h := novoServidorTeste().Handler()
	rotas := []struct{ metodo, caminho string }{
		{"GET", "/saldos"}, {"GET", "/alertas"}, {"GET", "/movimentacoes"},
		{"POST", "/movimentacoes"}, {"POST", "/insumos"}, {"POST", "/lotes"},
		{"GET", "/insumos"}, {"GET", "/lotes"}, {"GET", "/sessao"}, {"DELETE", "/sessao"},
	}
	for _, rt := range rotas {
		for _, cookie := range []string{"", "token-inventado"} {
			req := httptest.NewRequest(rt.metodo, rt.caminho, strings.NewReader(`{}`))
			req.Header.Set("Content-Type", "application/json")
			if cookie != "" {
				req.AddCookie(&http.Cookie{Name: nomeCookie, Value: cookie})
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("%s %s (cookie=%q) = %d; quer 401", rt.metodo, rt.caminho, cookie, rec.Code)
			}
		}
	}
}

func TestLoginExigeJSON(t *testing.T) {
	h := novoServidorTeste().Handler()
	req := httptest.NewRequest("POST", "/sessao", strings.NewReader("login=a&senha=b"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d; quer 415", rec.Code)
	}
}

func TestValidarUsuario(t *testing.T) {
	ok := novoUsuario{Login: "  maria ", Email: " Maria@Fazenda.com.br ", Senha: "12345678"}
	if msg := ok.validar(); msg != "" || ok.Login != "maria" || ok.Email != "maria@fazenda.com.br" {
		t.Fatalf("usuário válido rejeitado: %q (login=%q email=%q)", msg, ok.Login, ok.Email)
	}
	const e = "maria@fazenda.com"
	ruins := []novoUsuario{
		{Login: "", Email: e, Senha: "12345678"},
		{Login: "   ", Email: e, Senha: "12345678"},
		{Login: "maria silva", Email: e, Senha: "12345678"},
		{Login: "maria@x", Email: e, Senha: "12345678"},
		{Login: strings.Repeat("a", 51), Email: e, Senha: "12345678"},
		{Login: "maria", Email: "", Senha: "12345678"},
		{Login: "maria", Email: "nao-e-email", Senha: "12345678"},
		{Login: "maria", Email: "Maria <maria@fazenda.com>", Senha: "12345678"},
		{Login: "maria", Email: e, Senha: "1234567"},
		{Login: "maria", Email: e, Senha: strings.Repeat("x", 73)},
	}
	for i, u := range ruins {
		if msg := u.validar(); msg == "" {
			t.Errorf("caso %d deveria ser rejeitado: %+v", i, u)
		}
	}
}

func TestCadastroUsuarioValidaAntesDoBanco(t *testing.T) {
	h := novoServidorTeste().Handler() // db nil: só passa se barrar antes de consultar
	casos := []struct {
		ct, corpo string
		quer      int
	}{
		{"text/plain", `{"login":"a","email":"a@b.com","senha":"12345678"}`, http.StatusUnsupportedMediaType},
		{"application/json", `{"login":"a","email":"a@b.com","senha":"curta"}`, http.StatusUnprocessableEntity},
		{"application/json", `{"login":"a","senha":"12345678"}`, http.StatusUnprocessableEntity},
		{"application/json", `{"login":"a","email":"a@b.com","senha":"12345678","admin":true}`, http.StatusBadRequest},
	}
	for _, c := range casos {
		req := httptest.NewRequest("POST", "/usuarios", strings.NewReader(c.corpo))
		req.Header.Set("Content-Type", c.ct)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != c.quer {
			t.Errorf("%s %s = %d; quer %d", c.ct, c.corpo, rec.Code, c.quer)
		}
	}
}

func TestPreflightCORS(t *testing.T) {
	h := novoServidorTeste().Handler()
	req := httptest.NewRequest("OPTIONS", "/movimentacoes", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	req.Header.Set("Access-Control-Request-Method", "POST")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent ||
		rec.Header().Get("Access-Control-Allow-Origin") != "http://localhost:5173" ||
		rec.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Fatalf("preflight inesperado: %d %v", rec.Code, rec.Header())
	}

	// Origem desconhecida não recebe cabeçalhos CORS.
	req = httptest.NewRequest("GET", "/alertas", nil)
	req.Header.Set("Origin", "http://malicioso.example")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("origem não autorizada recebeu Access-Control-Allow-Origin")
	}
}

func TestRotasDeSenhaSaoPublicasEValidam(t *testing.T) {
	h := novoServidorTeste().Handler() // db nil: só passa se barrar antes de consultar
	casos := []struct {
		caminho, corpo string
		quer           int
	}{
		{"/senha/esqueci", `{"email":"invalido"}`, http.StatusUnprocessableEntity},
		{"/senha/redefinir", `{"token":"","novaSenha":"12345678"}`, http.StatusUnprocessableEntity},
		{"/senha/redefinir", `{"token":"abc","novaSenha":"curta"}`, http.StatusUnprocessableEntity},
	}
	for _, c := range casos {
		req := httptest.NewRequest("POST", c.caminho, strings.NewReader(c.corpo))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != c.quer {
			t.Errorf("%s %s = %d; quer %d", c.caminho, c.corpo, rec.Code, c.quer)
		}
	}
}

func TestTokenSenhaGuardaSoOHash(t *testing.T) {
	token, hash, err := gerarTokenSenha()
	if err != nil {
		t.Fatal(err)
	}
	if len(token) < 40 || len(hash) != 64 || hash == token || hashTokenSenha(token) != hash {
		t.Fatalf("token=%q hash=%q", token, hash)
	}
	outro, _, _ := gerarTokenSenha()
	if outro == token {
		t.Fatal("tokens repetidos")
	}
}

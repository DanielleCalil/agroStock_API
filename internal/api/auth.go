package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"github.com/DanielleCalil/agroStock_API/internal/sessao"
)

const nomeCookie = "agrostock_sessao"

// hashFalso é usado quando o login não existe, para que a resposta demore o
// mesmo tempo que um login real (evita descobrir usuários válidos pelo tempo).
var hashFalso, _ = bcrypt.GenerateFromPassword([]byte("senha-que-nunca-sera-usada"), bcrypt.DefaultCost)

type chaveCtx struct{}

func sessaoDoContexto(ctx context.Context) sessao.Sessao {
	ses, _ := ctx.Value(chaveCtx{}).(sessao.Sessao)
	return ses
}

// autenticado barra com 401 qualquer requisição sem um cookie de sessão válido.
func (s *Server) autenticado(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(nomeCookie)
		if err != nil || c.Value == "" {
			escreverErro(w, http.StatusUnauthorized, "não autenticado")
			return
		}
		ses, ok := s.sessoes.Buscar(c.Value)
		if !ok {
			s.apagarCookie(w)
			escreverErro(w, http.StatusUnauthorized, "sessão inválida ou expirada")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), chaveCtx{}, ses)))
	})
}

type credenciais struct {
	Login string `json:"login"`
	Senha string `json:"senha"`
}

type usuarioResp struct {
	ID    int    `json:"id"`
	Login string `json:"login"`
}

// POST /sessao
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var req credenciais
	if !lerJSON(w, r, &req) {
		return
	}
	req.Login = strings.TrimSpace(req.Login)
	if req.Login == "" || req.Senha == "" {
		escreverErro(w, http.StatusBadRequest, "login e senha são obrigatórios")
		return
	}

	var (
		id    int
		login string
		hash  string
		ativo bool
	)
	// Aceita o login ou o e-mail. Novos logins não podem ter "@", então não há
	// ambiguidade; ainda assim, um login exato tem prioridade.
	err := s.db.QueryRowContext(r.Context(), `
		SELECT TOP (1) Id, Login, HashSenha, Ativo FROM Usuario
		WHERE Login = @login OR Email = LOWER(@login)
		ORDER BY CASE WHEN Login = @login THEN 0 ELSE 1 END`,
		sql.Named("login", req.Login),
	).Scan(&id, &login, &hash, &ativo)

	switch {
	case errors.Is(err, sql.ErrNoRows):
		_ = bcrypt.CompareHashAndPassword(hashFalso, []byte(req.Senha))
		escreverErro(w, http.StatusUnauthorized, "login ou senha inválidos")
		return
	case err != nil:
		erroInterno(w, "login: consulta usuário", err)
		return
	}

	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Senha)) != nil || !ativo {
		escreverErro(w, http.StatusUnauthorized, "login ou senha inválidos")
		return
	}

	if err := s.iniciarSessao(w, r, id, login); err != nil {
		erroInterno(w, "login: criar sessão", err)
		return
	}
	escreverJSON(w, http.StatusOK, usuarioResp{ID: id, Login: login})
}

// iniciarSessao cria uma sessão para o usuário e grava o cookie na resposta.
func (s *Server) iniciarSessao(w http.ResponseWriter, r *http.Request, id int, login string) error {
	// Descarta sessão anterior deste navegador (evita fixação de sessão).
	if c, err := r.Cookie(nomeCookie); err == nil {
		s.sessoes.Remover(c.Value)
	}

	token, ses, err := s.sessoes.Criar(id, login)
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     nomeCookie,
		Value:    token,
		Path:     "/",
		Expires:  ses.Expira,
		HttpOnly: true, // inacessível ao JavaScript
		Secure:   s.cfg.CookieSecure,
		SameSite: s.sameSiteCookie(),
	})
	return nil
}

// GET /sessao — útil para o frontend saber se ainda está logado.
func (s *Server) sessaoAtual(w http.ResponseWriter, r *http.Request) {
	ses := sessaoDoContexto(r.Context())
	escreverJSON(w, http.StatusOK, usuarioResp{ID: ses.UsuarioID, Login: ses.Login})
}

// DELETE /sessao — logout.
func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(nomeCookie); err == nil {
		s.sessoes.Remover(c.Value)
	}
	s.apagarCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) apagarCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     nomeCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.cfg.CookieSecure,
		SameSite: s.sameSiteCookie(),
	})
}

// sameSiteCookie define o SameSite do cookie de sessão.
// Em produção (COOKIE_SECURE=true) o frontend fica em outro domínio
// (ex: Vercel -> Render), então o cookie precisa de SameSite=None para ser
// enviado entre sites — e o navegador só aceita None junto com Secure.
// Em desenvolvimento via http, usa Lax (None sem Secure seria rejeitado).
func (s *Server) sameSiteCookie() http.SameSite {
	if s.cfg.CookieSecure {
		return http.SameSiteNoneMode
	}
	return http.SameSiteLaxMode
}

package api

import (
	"database/sql"
	"net/http"
	"net/mail"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

// Mesmo custo usado pelo comando criar-usuario.
const custoBcrypt = 12

type novoUsuario struct {
	Login string `json:"login"`
	Email string `json:"email"`
	Senha string `json:"senha"`
}

// validarSenha: mesmas regras do comando criar-usuario.
func validarSenha(senha string) string {
	switch {
	case len(senha) < 8:
		return "a senha deve ter pelo menos 8 caracteres"
	case len(senha) > 72:
		return "a senha deve ter no máximo 72 bytes"
	}
	return ""
}

// normalizarEmail devolve o e-mail em minúsculas, ou "" se for inválido.
func normalizarEmail(email string) string {
	email = strings.ToLower(strings.TrimSpace(email))
	if len(email) > 255 {
		return ""
	}
	end, err := mail.ParseAddress(email)
	if err != nil || end.Address != email || end.Name != "" {
		return ""
	}
	return email
}

func (n *novoUsuario) validar() string {
	n.Login = strings.TrimSpace(n.Login)
	email := normalizarEmail(n.Email)
	switch {
	case n.Login == "":
		return "login é obrigatório"
	case utf8.RuneCountInString(n.Login) > 50:
		return "login deve ter no máximo 50 caracteres"
	case strings.IndexFunc(n.Login, unicode.IsSpace) >= 0:
		return "login não pode conter espaços"
	case strings.Contains(n.Login, "@"):
		return `login não pode conter "@"` // o login aceita usuário ou e-mail
	case strings.TrimSpace(n.Email) == "":
		return "e-mail é obrigatório"
	case email == "":
		return "e-mail inválido"
	}
	n.Email = email
	return validarSenha(n.Senha)
}

// POST /usuarios — cadastro público. Só cria o usuário; para entrar, o
// frontend leva ao login (POST /sessao).
func (s *Server) cadastrarUsuario(w http.ResponseWriter, r *http.Request) {
	var req novoUsuario
	if !lerJSON(w, r, &req) {
		return
	}
	if msg := req.validar(); msg != "" {
		escreverErro(w, http.StatusUnprocessableEntity, msg)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Senha), custoBcrypt)
	if err != nil {
		erroInterno(w, "cadastrar usuário: hash", err)
		return
	}

	var id int
	err = s.db.QueryRowContext(r.Context(),
		`INSERT INTO Usuario (Login, Email, HashSenha) OUTPUT INSERTED.Id VALUES (@login, @email, @hash)`,
		sql.Named("login", req.Login), sql.Named("email", req.Email), sql.Named("hash", string(hash)),
	).Scan(&id)
	if err != nil {
		if ehDuplicidade(err) {
			if strings.Contains(err.Error(), "UX_Usuario_Email") {
				escreverErro(w, http.StatusConflict, "já existe uma conta com este e-mail")
			} else {
				escreverErro(w, http.StatusConflict, "já existe um usuário com este login")
			}
			return
		}
		erroInterno(w, "cadastrar usuário: inserir", err)
		return
	}

	escreverJSON(w, http.StatusCreated, usuarioResp{ID: id, Login: req.Login})
}

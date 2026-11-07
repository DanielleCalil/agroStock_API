package api

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// Validade do token de redefinição de senha.
const minutosTokenSenha = 30

const msgRecuperacao = "Se o e-mail estiver cadastrado, você receberá instruções para redefinir sua senha."

// gerarTokenSenha devolve o token (para o usuário) e o hash SHA-256 dele (para
// o banco). Só o hash fica gravado: quem lê o banco não consegue usar o token.
func gerarTokenSenha() (token, hash string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", err
	}
	token = base64.RawURLEncoding.EncodeToString(b)
	return token, hashTokenSenha(token), nil
}

func hashTokenSenha(token string) string {
	soma := sha256.Sum256([]byte(token))
	return hex.EncodeToString(soma[:])
}

// POST /senha/esqueci {email}
//
// A resposta é sempre a mesma, exista ou não o e-mail, para não revelar quais
// e-mails têm conta. A API ainda não envia e-mail: com RESET_TOKEN_NA_RESPOSTA
// (só em desenvolvimento) o token volta em "tokenRedefinicao".
func (s *Server) esqueciSenha(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
	}
	if !lerJSON(w, r, &req) {
		return
	}
	email := normalizarEmail(req.Email)
	if email == "" {
		escreverErro(w, http.StatusUnprocessableEntity, "e-mail inválido")
		return
	}

	token, hash, err := gerarTokenSenha()
	if err != nil {
		erroInterno(w, "esqueci senha: gerar token", err)
		return
	}
	res, err := s.db.ExecContext(r.Context(), `
		UPDATE Usuario
		SET ResetTokenHash = @hash,
		    ResetTokenExpira = DATEADD(MINUTE, @minutos, SYSUTCDATETIME())
		WHERE Email = @email AND Ativo = 1`,
		sql.Named("hash", hash), sql.Named("minutos", minutosTokenSenha), sql.Named("email", email),
	)
	if err != nil {
		erroInterno(w, "esqueci senha: gravar token", err)
		return
	}

	resp := map[string]any{"mensagem": msgRecuperacao}
	if n, _ := res.RowsAffected(); n > 0 && s.cfg.ResetTokenNaResposta {
		resp["tokenRedefinicao"] = token
		resp["expiraEmMinutos"] = minutosTokenSenha
	}
	escreverJSON(w, http.StatusOK, resp)
}

// POST /senha/redefinir {token, novaSenha}
func (s *Server) redefinirSenha(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token     string `json:"token"`
		NovaSenha string `json:"novaSenha"`
	}
	if !lerJSON(w, r, &req) {
		return
	}
	req.Token = strings.TrimSpace(req.Token)
	if req.Token == "" {
		escreverErro(w, http.StatusUnprocessableEntity, "token é obrigatório")
		return
	}
	if msg := validarSenha(req.NovaSenha); msg != "" {
		escreverErro(w, http.StatusUnprocessableEntity, msg)
		return
	}

	hashSenha, err := bcrypt.GenerateFromPassword([]byte(req.NovaSenha), custoBcrypt)
	if err != nil {
		erroInterno(w, "redefinir senha: hash", err)
		return
	}

	// Confere e consome o token numa única instrução: o mesmo token não serve
	// duas vezes, nem em requisições simultâneas.
	res, err := s.db.ExecContext(r.Context(), `
		UPDATE Usuario
		SET HashSenha = @senha, ResetTokenHash = NULL, ResetTokenExpira = NULL
		WHERE ResetTokenHash = @token AND ResetTokenExpira > SYSUTCDATETIME() AND Ativo = 1`,
		sql.Named("senha", string(hashSenha)), sql.Named("token", hashTokenSenha(req.Token)),
	)
	if err != nil {
		erroInterno(w, "redefinir senha: atualizar", err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		escreverErro(w, http.StatusBadRequest, "token inválido ou expirado")
		return
	}
	escreverJSON(w, http.StatusOK, map[string]string{"mensagem": "Senha redefinida com sucesso."})
}

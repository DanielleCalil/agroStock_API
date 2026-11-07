package api

import (
	"database/sql"
	"net/http"
	"strings"
	"unicode/utf8"
)

type insumo struct {
	ID        int     `json:"id"`
	Nome      string  `json:"nome"`
	Categoria string  `json:"categoria"`
	Unidade   string  `json:"unidade"`
	Minimo    float64 `json:"minimo"`
}

type novoInsumo struct {
	Nome      string   `json:"nome"`
	Categoria string   `json:"categoria"`
	Unidade   string   `json:"unidade"`
	Minimo    *float64 `json:"minimo"` // ponteiro: distingue "ausente" de 0
}

// validar devolve a primeira regra violada, ou "" se o insumo é válido.
func (n *novoInsumo) validar() string {
	n.Nome = strings.TrimSpace(n.Nome)
	n.Categoria = strings.TrimSpace(n.Categoria)
	n.Unidade = strings.TrimSpace(n.Unidade)

	switch {
	case n.Nome == "":
		return "nome é obrigatório"
	case utf8.RuneCountInString(n.Nome) > 100:
		return "nome deve ter no máximo 100 caracteres"
	case n.Categoria == "":
		return "categoria é obrigatória"
	case utf8.RuneCountInString(n.Categoria) > 50:
		return "categoria deve ter no máximo 50 caracteres"
	case n.Unidade == "":
		return "unidade é obrigatória"
	case utf8.RuneCountInString(n.Unidade) > 20:
		return "unidade deve ter no máximo 20 caracteres"
	case n.Minimo == nil:
		return "minimo é obrigatório"
	case !valorDecimalValido(*n.Minimo):
		return "minimo fora do intervalo permitido"
	case *n.Minimo < 0:
		return "minimo deve ser maior ou igual a 0"
	}
	*n.Minimo = arredondar3(*n.Minimo)
	return ""
}

// POST /insumos
func (s *Server) criarInsumo(w http.ResponseWriter, r *http.Request) {
	var req novoInsumo
	if !lerJSON(w, r, &req) {
		return
	}
	if msg := req.validar(); msg != "" {
		escreverErro(w, http.StatusUnprocessableEntity, msg)
		return
	}

	ins := insumo{Nome: req.Nome, Categoria: req.Categoria, Unidade: req.Unidade, Minimo: *req.Minimo}
	err := s.db.QueryRowContext(r.Context(), `
		INSERT INTO Insumo (Nome, Categoria, Unidade, Minimo)
		OUTPUT INSERTED.Id
		VALUES (@nome, @categoria, @unidade, CAST(@minimo AS DECIMAL(14,3)))`,
		sql.Named("nome", ins.Nome),
		sql.Named("categoria", ins.Categoria),
		sql.Named("unidade", ins.Unidade),
		sql.Named("minimo", ins.Minimo),
	).Scan(&ins.ID)
	if err != nil {
		erroInterno(w, "criar insumo", err)
		return
	}
	escreverJSON(w, http.StatusCreated, ins)
}

// GET /insumos
func (s *Server) listarInsumos(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(),
		`SELECT Id, Nome, Categoria, Unidade, Minimo FROM Insumo ORDER BY Nome, Id`)
	if err != nil {
		erroInterno(w, "listar insumos", err)
		return
	}
	defer rows.Close()

	lista := []insumo{}
	for rows.Next() {
		var i insumo
		if err := rows.Scan(&i.ID, &i.Nome, &i.Categoria, &i.Unidade, &i.Minimo); err != nil {
			erroInterno(w, "listar insumos: scan", err)
			return
		}
		lista = append(lista, i)
	}
	if err := rows.Err(); err != nil {
		erroInterno(w, "listar insumos: rows", err)
		return
	}
	escreverJSON(w, http.StatusOK, lista)
}

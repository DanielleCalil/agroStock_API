package api

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const formatoData = "2006-01-02"

// data serializa um DATE do SQL Server como "AAAA-MM-DD".
type data time.Time

func (d data) MarshalJSON() ([]byte, error) {
	return []byte(`"` + time.Time(d).Format(formatoData) + `"`), nil
}

// instante serializa um DATETIME do SQL Server ("AAAA-MM-DDTHH:MM:SS").
// A coluna não guarda fuso; o valor é a hora local do servidor SQL (GETDATE()),
// por isso não acrescentamos "Z" nem offset.
type instante time.Time

func (i instante) MarshalJSON() ([]byte, error) {
	return []byte(`"` + time.Time(i).Format("2006-01-02T15:04:05") + `"`), nil
}

func (i *instante) Scan(v any) error {
	t, ok := v.(time.Time)
	if !ok {
		return fmt.Errorf("instante: tipo inesperado %T", v)
	}
	*i = instante(t)
	return nil
}

type lote struct {
	ID       int     `json:"id"`
	InsumoID int     `json:"insumoId"`
	Codigo   string  `json:"codigo"`
	Validade data    `json:"validade"`
	Saldo    float64 `json:"saldo"`
}

type novoLote struct {
	InsumoID int    `json:"insumoId"`
	Codigo   string `json:"codigo"`
	Validade string `json:"validade"` // "AAAA-MM-DD"

	validade time.Time
}

func (n *novoLote) validar() string {
	n.Codigo = strings.TrimSpace(n.Codigo)
	switch {
	case n.InsumoID <= 0:
		return "insumoId é obrigatório"
	case n.Codigo == "":
		return "codigo é obrigatório"
	case utf8.RuneCountInString(n.Codigo) > 50:
		return "codigo deve ter no máximo 50 caracteres"
	case n.Validade == "":
		return "validade é obrigatória"
	}
	v, err := time.Parse(formatoData, strings.TrimSpace(n.Validade))
	if err != nil {
		return "validade deve estar no formato AAAA-MM-DD e ser uma data real"
	}
	if v.Year() < 1900 || v.Year() > 9999 {
		return "validade fora do intervalo permitido"
	}
	n.validade = v
	return ""
}

// POST /lotes
//
// O lote nasce com saldo 0; o estoque entra via POST /movimentacoes (tipo E),
// para que todo saldo tenha uma movimentação correspondente no histórico.
func (s *Server) criarLote(w http.ResponseWriter, r *http.Request) {
	var req novoLote
	if !lerJSON(w, r, &req) {
		return
	}
	if msg := req.validar(); msg != "" {
		escreverErro(w, http.StatusUnprocessableEntity, msg)
		return
	}

	l := lote{InsumoID: req.InsumoID, Codigo: req.Codigo, Validade: data(req.validade)}
	err := s.db.QueryRowContext(r.Context(), `
		INSERT INTO Lote (InsumoId, Codigo, Validade)
		OUTPUT INSERTED.Id, INSERTED.Saldo
		VALUES (@insumoId, @codigo, @validade)`,
		sql.Named("insumoId", l.InsumoID),
		sql.Named("codigo", l.Codigo),
		sql.Named("validade", req.validade.Format(formatoData)),
	).Scan(&l.ID, &l.Saldo)

	if err != nil {
		if ehDuplicidade(err) {
			escreverErro(w, http.StatusConflict, "já existe um lote com este código para este insumo")
			return
		}
		if n, ok := codigoErroSQL(err); ok && n == sqlErrFK {
			escreverErro(w, http.StatusUnprocessableEntity, "insumo não encontrado")
			return
		}
		erroInterno(w, "criar lote", err)
		return
	}
	escreverJSON(w, http.StatusCreated, l)
}

// GET /lotes?insumoId=1
func (s *Server) listarLotes(w http.ResponseWriter, r *http.Request) {
	query := `SELECT Id, InsumoId, Codigo, Validade, Saldo FROM Lote`
	var args []any
	if v := r.URL.Query().Get("insumoId"); v != "" {
		id, err := strconv.Atoi(v)
		if err != nil || id <= 0 {
			escreverErro(w, http.StatusBadRequest, "insumoId inválido")
			return
		}
		query += ` WHERE InsumoId = @insumoId`
		args = append(args, sql.Named("insumoId", id))
	}
	query += ` ORDER BY Validade, Id`

	rows, err := s.db.QueryContext(r.Context(), query, args...)
	if err != nil {
		erroInterno(w, "listar lotes", err)
		return
	}
	defer rows.Close()

	lista := []lote{}
	for rows.Next() {
		var l lote
		var validade time.Time
		if err := rows.Scan(&l.ID, &l.InsumoID, &l.Codigo, &validade, &l.Saldo); err != nil {
			erroInterno(w, "listar lotes: scan", err)
			return
		}
		l.Validade = data(validade)
		lista = append(lista, l)
	}
	if err := rows.Err(); err != nil && !errors.Is(err, sql.ErrNoRows) {
		erroInterno(w, "listar lotes: rows", err)
		return
	}
	escreverJSON(w, http.StatusOK, lista)
}

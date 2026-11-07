package api

import (
	"database/sql"
	"net/http"
	"strconv"
)

type saldoLote struct {
	LoteID   int     `json:"loteId"`
	Codigo   string  `json:"codigo"`
	Validade data    `json:"validade"`
	Saldo    float64 `json:"saldo"`
}

type saldoInsumo struct {
	InsumoID     int         `json:"insumoId"`
	Nome         string      `json:"nome"`
	Categoria    string      `json:"categoria"`
	Unidade      string      `json:"unidade"`
	Minimo       float64     `json:"minimo"`
	SaldoTotal   float64     `json:"saldoTotal"`
	AbaixoMinimo bool        `json:"abaixoMinimo"`
	Lotes        []saldoLote `json:"lotes"`
}

// GET /saldos?insumoId=1
//
// Saldo consolidado por insumo (soma dos lotes), com o detalhe de cada lote e
// a indicação de estoque abaixo do mínimo cadastrado.
func (s *Server) listarSaldos(w http.ResponseWriter, r *http.Request) {
	query := `
		SELECT i.Id, i.Nome, i.Categoria, i.Unidade, i.Minimo,
		       l.Id, l.Codigo, l.Validade, l.Saldo
		FROM Insumo i
		LEFT JOIN Lote l ON l.InsumoId = i.Id`
	var args []any
	if v := r.URL.Query().Get("insumoId"); v != "" {
		id, err := strconv.Atoi(v)
		if err != nil || id <= 0 {
			escreverErro(w, http.StatusBadRequest, "insumoId inválido")
			return
		}
		query += ` WHERE i.Id = @insumoId`
		args = append(args, sql.Named("insumoId", id))
	}
	query += ` ORDER BY i.Nome, i.Id, l.Validade, l.Id`

	rows, err := s.db.QueryContext(r.Context(), query, args...)
	if err != nil {
		erroInterno(w, "listar saldos", err)
		return
	}
	defer rows.Close()

	lista := []saldoInsumo{}
	for rows.Next() {
		var (
			ins      saldoInsumo
			loteID   sql.NullInt64
			codigo   sql.NullString
			validade sql.NullTime
			saldo    sql.NullFloat64
		)
		if err := rows.Scan(&ins.InsumoID, &ins.Nome, &ins.Categoria, &ins.Unidade, &ins.Minimo,
			&loteID, &codigo, &validade, &saldo); err != nil {
			erroInterno(w, "listar saldos: scan", err)
			return
		}
		// As linhas vêm ordenadas por insumo: abre um grupo novo quando muda.
		if len(lista) == 0 || lista[len(lista)-1].InsumoID != ins.InsumoID {
			ins.Lotes = []saldoLote{}
			lista = append(lista, ins)
		}
		atual := &lista[len(lista)-1]
		if loteID.Valid {
			atual.Lotes = append(atual.Lotes, saldoLote{
				LoteID: int(loteID.Int64), Codigo: codigo.String,
				Validade: data(validade.Time), Saldo: saldo.Float64,
			})
			atual.SaldoTotal = arredondar3(atual.SaldoTotal + saldo.Float64)
		}
	}
	if err := rows.Err(); err != nil {
		erroInterno(w, "listar saldos: rows", err)
		return
	}
	for i := range lista {
		lista[i].AbaixoMinimo = lista[i].SaldoTotal < lista[i].Minimo
	}
	escreverJSON(w, http.StatusOK, lista)
}

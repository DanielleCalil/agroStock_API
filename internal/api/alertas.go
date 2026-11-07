package api

import (
	"database/sql"
	"net/http"
	"time"
)

// Faixas do motor de alertas de validade.
const (
	diasAlertaVermelho = 7  // até 7 dias (inclui vencidos)
	diasAlertaAmarelo  = 30 // de 8 a 30 dias
)

const (
	nivelVermelho = "vermelho"
	nivelAmarelo  = "amarelo"
)

// classificarValidade decide a cor do alerta a partir dos dias restantes.
// ok=false significa que o lote não precisa de alerta (mais de 30 dias).
func classificarValidade(diasRestantes int) (nivel string, ok bool) {
	switch {
	case diasRestantes <= diasAlertaVermelho:
		return nivelVermelho, true
	case diasRestantes <= diasAlertaAmarelo:
		return nivelAmarelo, true
	}
	return "", false
}

// diaCivil zera o horário, mantendo só ano/mês/dia (em UTC, para comparar
// com colunas DATE que o driver devolve como meia-noite UTC).
func diaCivil(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// diasEntre conta dias de calendário de "de" até "ate" (negativo se já passou).
func diasEntre(de, ate time.Time) int {
	return int(diaCivil(ate).Sub(diaCivil(de)).Hours() / 24)
}

type alerta struct {
	LoteID        int     `json:"loteId"`
	LoteCodigo    string  `json:"loteCodigo"`
	InsumoID      int     `json:"insumoId"`
	InsumoNome    string  `json:"insumoNome"`
	Unidade       string  `json:"unidade"`
	Saldo         float64 `json:"saldo"`
	Validade      data    `json:"validade"`
	DiasRestantes int     `json:"diasRestantes"`
	Vencido       bool    `json:"vencido"`
	Nivel         string  `json:"nivel"` // "vermelho" | "amarelo"
}

type respostaAlertas struct {
	DataReferencia data     `json:"dataReferencia"`
	Alertas        []alerta `json:"alertas"`
}

// GET /alertas
//
// Lotes com saldo > 0 cuja validade está a 30 dias ou menos da data de hoje
// (data do servidor da API). Lotes zerados não geram alerta.
func (s *Server) listarAlertas(w http.ResponseWriter, r *http.Request) {
	hoje := diaCivil(s.hoje())
	limite := hoje.AddDate(0, 0, diasAlertaAmarelo)

	rows, err := s.db.QueryContext(r.Context(), `
		SELECT l.Id, l.Codigo, i.Id, i.Nome, i.Unidade, l.Saldo, l.Validade
		FROM Lote l
		JOIN Insumo i ON i.Id = l.InsumoId
		WHERE l.Saldo > 0 AND l.Validade <= @limite
		ORDER BY l.Validade, l.Id`,
		sql.Named("limite", limite.Format(formatoData)),
	)
	if err != nil {
		erroInterno(w, "listar alertas", err)
		return
	}
	defer rows.Close()

	resp := respostaAlertas{DataReferencia: data(hoje), Alertas: []alerta{}}
	for rows.Next() {
		var a alerta
		var validade time.Time
		if err := rows.Scan(&a.LoteID, &a.LoteCodigo, &a.InsumoID, &a.InsumoNome, &a.Unidade, &a.Saldo, &validade); err != nil {
			erroInterno(w, "listar alertas: scan", err)
			return
		}
		a.Validade = data(validade)
		a.DiasRestantes = diasEntre(hoje, validade)
		a.Vencido = a.DiasRestantes < 0
		nivel, ok := classificarValidade(a.DiasRestantes)
		if !ok {
			continue
		}
		a.Nivel = nivel
		resp.Alertas = append(resp.Alertas, a)
	}
	if err := rows.Err(); err != nil {
		erroInterno(w, "listar alertas: rows", err)
		return
	}
	escreverJSON(w, http.StatusOK, resp)
}

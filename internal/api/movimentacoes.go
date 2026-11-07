package api

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	mssql "github.com/microsoft/go-mssqldb"
)

type movimentacao struct {
	ID              int      `json:"id"`
	LoteID          int      `json:"loteId"`
	LoteCodigo      string   `json:"loteCodigo,omitempty"`
	InsumoID        int      `json:"insumoId,omitempty"`
	InsumoNome      string   `json:"insumoNome,omitempty"`
	Unidade         string   `json:"unidade,omitempty"`
	UsuarioID       int      `json:"usuarioId"`
	UsuarioLogin    string   `json:"usuarioLogin,omitempty"`
	Tipo            string   `json:"tipo"` // "E" ou "S"
	Quantidade      float64  `json:"quantidade"`
	Instante        instante `json:"instante"`
	Motivo          *string  `json:"motivo"`
	ChaveRequisicao string   `json:"chaveRequisicao"`
	SaldoLote       *float64 `json:"saldoLote,omitempty"` // saldo do lote após a operação
}

type novaMovimentacao struct {
	LoteID          int     `json:"loteId"`
	Tipo            string  `json:"tipo"`
	Quantidade      float64 `json:"quantidade"`
	Motivo          *string `json:"motivo"`
	ChaveRequisicao string  `json:"chaveRequisicao"`
}

// normalizarTipo aceita "E"/"S" ou "entrada"/"saida" (sem diferenciar maiúsculas).
func normalizarTipo(t string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "e", "entrada":
		return "E", true
	case "s", "saida", "saída":
		return "S", true
	}
	return "", false
}

func (n *novaMovimentacao) validar() string {
	n.ChaveRequisicao = strings.TrimSpace(n.ChaveRequisicao)
	if n.Motivo != nil {
		m := strings.TrimSpace(*n.Motivo)
		if m == "" {
			n.Motivo = nil
		} else {
			n.Motivo = &m
		}
	}
	tipo, ok := normalizarTipo(n.Tipo)
	switch {
	case n.LoteID <= 0:
		return "loteId é obrigatório"
	case !ok:
		return `tipo deve ser "E" (entrada) ou "S" (saída)`
	case !valorDecimalValido(n.Quantidade):
		return "quantidade fora do intervalo permitido"
	case arredondar3(n.Quantidade) <= 0:
		return "quantidade deve ser maior que 0"
	case n.ChaveRequisicao == "":
		return "chaveRequisicao é obrigatória"
	case len(n.ChaveRequisicao) > 100:
		return "chaveRequisicao deve ter no máximo 100 caracteres"
	case n.Motivo != nil && utf8.RuneCountInString(*n.Motivo) > 255:
		return "motivo deve ter no máximo 255 caracteres"
	}
	n.Tipo = tipo
	n.Quantidade = arredondar3(n.Quantidade)
	return ""
}

// mesmaOperacao diz se uma movimentação já gravada corresponde ao pedido
// (mesma chave reenviada por duplo clique / retry).
func (n novaMovimentacao) mesmaOperacao(m movimentacao, usuarioID int) bool {
	return m.LoteID == n.LoteID && m.Tipo == n.Tipo &&
		arredondar3(m.Quantidade) == n.Quantidade && m.UsuarioID == usuarioID
}

type consultor interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func buscarMovimentacaoPorChave(ctx context.Context, q consultor, chave string) (movimentacao, bool, error) {
	var m movimentacao
	var motivo sql.NullString
	err := q.QueryRowContext(ctx, `
		SELECT Id, LoteId, UsuarioId, Tipo, Quantidade, Instante, Motivo, ChaveRequisicao
		FROM Movimentacao WHERE ChaveRequisicao = @chave`,
		sql.Named("chave", chave),
	).Scan(&m.ID, &m.LoteID, &m.UsuarioID, &m.Tipo, &m.Quantidade, &m.Instante, &motivo, &m.ChaveRequisicao)
	if errors.Is(err, sql.ErrNoRows) {
		return m, false, nil
	}
	if err != nil {
		return m, false, err
	}
	if motivo.Valid {
		m.Motivo = &motivo.String
	}
	return m, true, nil
}

// responderRepeticao devolve a movimentação já registrada para a mesma chave,
// sem executar nada de novo (proteção contra duplo clique).
func responderRepeticao(w http.ResponseWriter, req novaMovimentacao, existente movimentacao, usuarioID int) {
	if !req.mesmaOperacao(existente, usuarioID) {
		escreverErro(w, http.StatusConflict, "chaveRequisicao já utilizada em outra movimentação")
		return
	}
	w.Header().Set("Idempotent-Replayed", "true")
	escreverJSON(w, http.StatusOK, existente)
}

// POST /movimentacoes
//
// Tudo acontece em uma única transação: trava o lote, confere o saldo,
// atualiza o saldo e grava a movimentação. Qualquer erro (ou queda da conexão
// / cancelamento da requisição) desfaz tudo — o saldo nunca fica pela metade.
func (s *Server) registrarMovimentacao(w http.ResponseWriter, r *http.Request) {
	var req novaMovimentacao
	if !lerJSON(w, r, &req) {
		return
	}
	if msg := req.validar(); msg != "" {
		escreverErro(w, http.StatusUnprocessableEntity, msg)
		return
	}
	usuario := sessaoDoContexto(r.Context())
	ctx := r.Context()

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		erroInterno(w, "movimentação: iniciar transação", err)
		return
	}
	// Rollback é inofensivo depois de um Commit bem-sucedido; em qualquer
	// saída antecipada ele desfaz o que já foi feito.
	defer tx.Rollback()

	// 1) Duplo clique: a chave já foi processada?
	if existente, achou, err := buscarMovimentacaoPorChave(ctx, tx, req.ChaveRequisicao); err != nil {
		erroInterno(w, "movimentação: verificar chave", err)
		return
	} else if achou {
		responderRepeticao(w, req, existente, usuario.UsuarioID)
		return
	}

	// 2) Trava a linha do lote até o fim da transação. Duas retiradas
	//    simultâneas do mesmo lote passam a ser executadas uma após a outra.
	var saldoAtual float64
	err = tx.QueryRowContext(ctx,
		`SELECT Saldo FROM Lote WITH (UPDLOCK, ROWLOCK) WHERE Id = @id`,
		sql.Named("id", req.LoteID),
	).Scan(&saldoAtual)
	if errors.Is(err, sql.ErrNoRows) {
		escreverErro(w, http.StatusNotFound, "lote não encontrado")
		return
	}
	if err != nil {
		erroInterno(w, "movimentação: travar lote", err)
		return
	}

	// 3) Atualiza o saldo. Na saída, a condição Saldo >= @qtd é a garantia
	//    final (junto com o CHECK do banco) de que o saldo nunca fica negativo.
	var res sql.Result
	if req.Tipo == "S" {
		res, err = tx.ExecContext(ctx, `
			UPDATE Lote SET Saldo = Saldo - CAST(@qtd AS DECIMAL(14,3))
			WHERE Id = @id AND Saldo >= CAST(@qtd AS DECIMAL(14,3))`,
			sql.Named("qtd", req.Quantidade), sql.Named("id", req.LoteID))
	} else {
		res, err = tx.ExecContext(ctx, `
			UPDATE Lote SET Saldo = Saldo + CAST(@qtd AS DECIMAL(14,3))
			WHERE Id = @id`,
			sql.Named("qtd", req.Quantidade), sql.Named("id", req.LoteID))
	}
	if err != nil {
		if n, ok := codigoErroSQL(err); ok && n == sqlErrOverflowAritmetico {
			escreverErro(w, http.StatusUnprocessableEntity, "saldo resultante excede o limite permitido")
			return
		}
		erroInterno(w, "movimentação: atualizar saldo", err)
		return
	}
	if afetadas, err := res.RowsAffected(); err != nil {
		erroInterno(w, "movimentação: rows affected", err)
		return
	} else if afetadas == 0 {
		escreverJSON(w, http.StatusConflict, map[string]any{
			"erro":            "saldo insuficiente no lote",
			"saldoDisponivel": saldoAtual,
			"solicitado":      req.Quantidade,
		})
		return
	}

	// 4) Registra a movimentação.
	m := movimentacao{
		LoteID: req.LoteID, UsuarioID: usuario.UsuarioID, Tipo: req.Tipo,
		Quantidade: req.Quantidade, Motivo: req.Motivo, ChaveRequisicao: req.ChaveRequisicao,
	}
	var motivo any // NULL quando não informado
	if req.Motivo != nil {
		motivo = *req.Motivo
	}
	err = tx.QueryRowContext(ctx, `
		INSERT INTO Movimentacao (LoteId, UsuarioId, Tipo, Quantidade, Motivo, ChaveRequisicao)
		OUTPUT INSERTED.Id, INSERTED.Instante
		VALUES (@lote, @usuario, @tipo, CAST(@qtd AS DECIMAL(14,3)), @motivo, @chave)`,
		sql.Named("lote", m.LoteID),
		sql.Named("usuario", m.UsuarioID),
		sql.Named("tipo", m.Tipo),
		sql.Named("qtd", m.Quantidade),
		sql.Named("motivo", motivo),
		sql.Named("chave", m.ChaveRequisicao),
	).Scan(&m.ID, &m.Instante)
	if err != nil {
		if ehDuplicidade(err) {
			// Corrida: outro clique com a mesma chave terminou primeiro.
			// Desfazemos esta tentativa (inclusive o UPDATE do saldo) e
			// devolvemos a movimentação que venceu.
			_ = tx.Rollback()
			existente, achou, err2 := buscarMovimentacaoPorChave(ctx, s.db, req.ChaveRequisicao)
			if err2 != nil || !achou {
				erroInterno(w, "movimentação: reler chave duplicada", fmt.Errorf("%v / achou=%v", err2, achou))
				return
			}
			responderRepeticao(w, req, existente, usuario.UsuarioID)
			return
		}
		erroInterno(w, "movimentação: inserir", err)
		return
	}

	var saldoFinal float64
	if err := tx.QueryRowContext(ctx, `SELECT Saldo FROM Lote WHERE Id = @id`,
		sql.Named("id", req.LoteID)).Scan(&saldoFinal); err != nil {
		erroInterno(w, "movimentação: ler saldo final", err)
		return
	}

	// 5) Tudo certo: confirma de uma vez.
	if err := tx.Commit(); err != nil {
		erroInterno(w, "movimentação: commit", err)
		return
	}
	m.SaldoLote = &saldoFinal
	escreverJSON(w, http.StatusCreated, m)
}

// GET /movimentacoes?insumoId=&loteId=&tipo=E|S&de=AAAA-MM-DD&ate=AAAA-MM-DD&limite=100
func (s *Server) listarMovimentacoes(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var (
		filtros []string
		args    []any
	)

	inteiro := func(nome string) (int, bool) {
		v := q.Get(nome)
		if v == "" {
			return 0, true
		}
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			escreverErro(w, http.StatusBadRequest, nome+" inválido")
			return 0, false
		}
		return n, true
	}

	insumoID, ok := inteiro("insumoId")
	if !ok {
		return
	}
	if insumoID > 0 {
		filtros = append(filtros, "l.InsumoId = @insumoId")
		args = append(args, sql.Named("insumoId", insumoID))
	}
	loteID, ok := inteiro("loteId")
	if !ok {
		return
	}
	if loteID > 0 {
		filtros = append(filtros, "m.LoteId = @loteId")
		args = append(args, sql.Named("loteId", loteID))
	}
	if v := q.Get("tipo"); v != "" {
		tipo, ok := normalizarTipo(v)
		if !ok {
			escreverErro(w, http.StatusBadRequest, `tipo deve ser "E" ou "S"`)
			return
		}
		filtros = append(filtros, "m.Tipo = @tipo")
		args = append(args, sql.Named("tipo", tipo))
	}
	if v := q.Get("de"); v != "" {
		d, err := time.Parse(formatoData, v)
		if err != nil {
			escreverErro(w, http.StatusBadRequest, "de deve estar no formato AAAA-MM-DD")
			return
		}
		// Parâmetro como DATETIME (não texto): com o idioma do SQL Server em
		// português, o texto "AAAA-MM-DD" seria lido como ano-dia-mês.
		filtros = append(filtros, "m.Instante >= @de")
		args = append(args, sql.Named("de", mssql.DateTime1(d)))
	}
	if v := q.Get("ate"); v != "" {
		d, err := time.Parse(formatoData, v)
		if err != nil {
			escreverErro(w, http.StatusBadRequest, "ate deve estar no formato AAAA-MM-DD")
			return
		}
		// "ate" é inclusivo: tudo antes do dia seguinte.
		filtros = append(filtros, "m.Instante < @ateExclusivo")
		args = append(args, sql.Named("ateExclusivo", mssql.DateTime1(d.AddDate(0, 0, 1))))
	}
	limite, ok := inteiro("limite")
	if !ok {
		return
	}
	if limite == 0 {
		limite = 100
	}
	if limite > 500 {
		limite = 500
	}
	args = append(args, sql.Named("limite", limite))

	// SaldoLote = saldo do lote logo após cada movimentação. Como o lote nasce
	// com saldo 0 e só muda via POST /movimentacoes (com o lote travado, então
	// a ordem dos Ids por lote é a ordem real das operações), a soma acumulada
	// das entradas menos as saídas reproduz o saldo histórico. A janela é
	// calculada na CTE, sobre todas as movimentações, antes dos filtros.
	query := `
		WITH m AS (
			SELECT mv.*,
				SUM(CASE WHEN mv.Tipo = 'E' THEN mv.Quantidade ELSE -mv.Quantidade END)
					OVER (PARTITION BY mv.LoteId ORDER BY mv.Id ROWS UNBOUNDED PRECEDING) AS SaldoLote
			FROM Movimentacao mv
		)
		SELECT TOP (@limite)
			m.Id, m.LoteId, l.Codigo, i.Id, i.Nome, i.Unidade,
			m.UsuarioId, u.Login, m.Tipo, m.Quantidade, m.Instante, m.Motivo, m.ChaveRequisicao,
			m.SaldoLote
		FROM m
		JOIN Lote l    ON l.Id = m.LoteId
		JOIN Insumo i  ON i.Id = l.InsumoId
		JOIN Usuario u ON u.Id = m.UsuarioId`
	if len(filtros) > 0 {
		query += "\n\t\tWHERE " + strings.Join(filtros, " AND ")
	}
	query += "\n\t\tORDER BY m.Instante DESC, m.Id DESC"

	rows, err := s.db.QueryContext(r.Context(), query, args...)
	if err != nil {
		erroInterno(w, "listar movimentações", err)
		return
	}
	defer rows.Close()

	lista := []movimentacao{}
	for rows.Next() {
		var m movimentacao
		var motivo sql.NullString
		var saldoLote float64
		if err := rows.Scan(&m.ID, &m.LoteID, &m.LoteCodigo, &m.InsumoID, &m.InsumoNome, &m.Unidade,
			&m.UsuarioID, &m.UsuarioLogin, &m.Tipo, &m.Quantidade, &m.Instante, &motivo, &m.ChaveRequisicao,
			&saldoLote); err != nil {
			erroInterno(w, "listar movimentações: scan", err)
			return
		}
		if motivo.Valid {
			m.Motivo = &motivo.String
		}
		saldoLote = arredondar3(saldoLote)
		m.SaldoLote = &saldoLote
		lista = append(lista, m)
	}
	if err := rows.Err(); err != nil {
		erroInterno(w, "listar movimentações: rows", err)
		return
	}
	escreverJSON(w, http.StatusOK, lista)
}

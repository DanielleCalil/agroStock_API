package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"mime"
	"net/http"
	"strings"

	mssql "github.com/microsoft/go-mssqldb"
)

const tamanhoMaxCorpo = 1 << 20 // 1 MB

func escreverJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("erro ao escrever resposta JSON: %v", err)
	}
}

func escreverErro(w http.ResponseWriter, status int, msg string) {
	escreverJSON(w, status, map[string]string{"erro": msg})
}

// erroInterno registra o detalhe no log e devolve uma mensagem genérica,
// para não vazar detalhes do banco ao cliente.
func erroInterno(w http.ResponseWriter, contexto string, err error) {
	log.Printf("%s: %v", contexto, err)
	escreverErro(w, http.StatusInternalServerError, "erro interno")
}

// lerJSON decodifica o corpo da requisição. Exigir Content-Type JSON também
// protege contra CSRF via formulário HTML (que não consegue enviar esse tipo).
func lerJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	ct, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if ct != "application/json" {
		escreverErro(w, http.StatusUnsupportedMediaType, "Content-Type deve ser application/json")
		return false
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, tamanhoMaxCorpo))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		var maxErr *http.MaxBytesError
		var tipoErr *json.UnmarshalTypeError
		switch {
		case errors.As(err, &maxErr):
			escreverErro(w, http.StatusRequestEntityTooLarge, "corpo da requisição muito grande")
		case errors.As(err, &tipoErr):
			escreverErro(w, http.StatusBadRequest, fmt.Sprintf("campo %q com tipo inválido", tipoErr.Field))
		case strings.HasPrefix(err.Error(), "json: unknown field"):
			escreverErro(w, http.StatusBadRequest, strings.Replace(err.Error(), "json: unknown field", "campo desconhecido", 1))
		case errors.Is(err, io.EOF):
			escreverErro(w, http.StatusBadRequest, "corpo da requisição vazio")
		default:
			escreverErro(w, http.StatusBadRequest, "JSON inválido")
		}
		return false
	}
	if dec.More() {
		escreverErro(w, http.StatusBadRequest, "o corpo deve conter um único objeto JSON")
		return false
	}
	return true
}

// Códigos de erro do SQL Server que tratamos explicitamente.
const (
	sqlErrChaveDuplicada     = 2627 // violação de UNIQUE/PRIMARY KEY constraint
	sqlErrIndiceDuplicado    = 2601 // violação de índice único
	sqlErrFK                 = 547  // violação de FOREIGN KEY / CHECK
	sqlErrOverflowAritmetico = 8115
)

func codigoErroSQL(err error) (int32, bool) {
	var e mssql.Error
	if errors.As(err, &e) {
		return e.Number, true
	}
	return 0, false
}

func ehDuplicidade(err error) bool {
	n, ok := codigoErroSQL(err)
	return ok && (n == sqlErrChaveDuplicada || n == sqlErrIndiceDuplicado)
}

// Limite de DECIMAL(14,3): 11 dígitos inteiros.
const valorDecimalMax = 99_999_999_999.999

// arredondar3 normaliza o número para as 3 casas decimais do banco.
func arredondar3(v float64) float64 {
	return math.Round(v*1000) / 1000
}

func valorDecimalValido(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0) && math.Abs(v) <= valorDecimalMax
}

// Comando criar-usuario cadastra um usuário com a senha protegida por bcrypt.
//
//	go run ./cmd/criar-usuario -login admin
//
// A senha é lida da entrada padrão (para não ficar no histórico do terminal).
package main

import (
	"bufio"
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
	mssql "github.com/microsoft/go-mssqldb"
	"golang.org/x/crypto/bcrypt"
)

const custoBcrypt = 12

func main() {
	login := flag.String("login", "", "login do novo usuário (obrigatório)")
	flag.Parse()
	_ = godotenv.Load()

	*login = strings.TrimSpace(*login)
	if *login == "" || len(*login) > 50 {
		log.Fatal("informe -login (até 50 caracteres)")
	}
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL não definida (veja .env.example)")
	}

	fmt.Fprint(os.Stderr, "Senha: ")
	senha, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && senha == "" {
		log.Fatalf("erro ao ler senha: %v", err)
	}
	senha = strings.TrimRight(senha, "\r\n")
	if len(senha) < 8 {
		log.Fatal("a senha deve ter pelo menos 8 caracteres")
	}
	if len(senha) > 72 {
		log.Fatal("a senha deve ter no máximo 72 bytes (limite do bcrypt)")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(senha), custoBcrypt)
	if err != nil {
		log.Fatalf("erro ao gerar hash: %v", err)
	}

	db, err := sql.Open("sqlserver", dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var id int
	err = db.QueryRowContext(ctx,
		`INSERT INTO Usuario (Login, HashSenha) OUTPUT INSERTED.Id VALUES (@login, @hash)`,
		sql.Named("login", *login), sql.Named("hash", string(hash)),
	).Scan(&id)
	if err != nil {
		if e, ok := err.(mssql.Error); ok && (e.Number == 2627 || e.Number == 2601) {
			log.Fatalf("já existe um usuário com o login %q", *login)
		}
		log.Fatalf("erro ao inserir usuário: %v", err)
	}
	fmt.Printf("usuário %q criado (id %d)\n", *login, id)
}

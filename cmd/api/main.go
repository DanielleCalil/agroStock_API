// Comando api sobe o servidor HTTP da agroStock.
//
//	go run ./cmd/api
package main

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	_ "github.com/microsoft/go-mssqldb"

	"github.com/DanielleCalil/agroStock_API/internal/api"
	"github.com/DanielleCalil/agroStock_API/internal/config"
	"github.com/DanielleCalil/agroStock_API/internal/sessao"
)

func main() {
	_ = godotenv.Load() // .env é opcional; variáveis do sistema têm prioridade

	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	db, err := sql.Open("sqlserver", cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("configuração do banco inválida: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(30 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	err = db.PingContext(ctx)
	cancel()
	if err != nil {
		log.Fatalf("não foi possível conectar ao SQL Server: %v", err)
	}
	log.Println("conectado ao SQL Server")

	// Ajuda a diagnosticar 401 em produção: confira estas linhas no log do Render.
	log.Printf("CORS: origens autorizadas = %q", cfg.AllowedOrigins)
	if cfg.CookieSecure {
		log.Println("cookie de sessão: Secure + SameSite=None (front e API em domínios diferentes)")
	} else {
		log.Println("cookie de sessão: SameSite=Lax, sem Secure (apenas desenvolvimento/mesmo site)")
	}

	roteador := api.NewServer(db, sessao.NewStore(cfg.SessionTTL), cfg).Handler()

	// CORS: libera o frontend (ALLOWED_ORIGIN) a chamar a API com o cookie de sessão.
	cors := api.CORS(api.CORSConfig{
		Origens:     cfg.AllowedOrigins,
		Metodos:     []string{http.MethodGet, http.MethodPost, http.MethodDelete, http.MethodOptions},
		Cabecalhos:  []string{"Content-Type"},
		Credenciais: true,
		MaxAge:      10 * time.Minute,
	})

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           cors(roteador),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		log.Printf("agroStock API ouvindo em %s", cfg.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("servidor HTTP: %v", err)
		}
	}()

	// Desligamento gracioso: termina as requisições em andamento (e suas
	// transações) antes de encerrar.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	log.Println("encerrando...")
	ctx, cancel = context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("erro no desligamento: %v", err)
	}
}

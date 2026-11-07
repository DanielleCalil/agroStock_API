package sessao

import (
	"testing"
	"time"
)

func TestCriarBuscarRemover(t *testing.T) {
	s := NewStore(time.Hour)
	token, _, err := s.Criar(1, "ana")
	if err != nil {
		t.Fatal(err)
	}
	if ses, ok := s.Buscar(token); !ok || ses.UsuarioID != 1 || ses.Login != "ana" {
		t.Fatalf("sessão não encontrada: %+v %v", ses, ok)
	}
	s.Remover(token)
	if _, ok := s.Buscar(token); ok {
		t.Fatal("sessão removida ainda é válida")
	}
}

func TestSessaoExpira(t *testing.T) {
	agora := time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)
	s := NewStore(time.Hour)
	s.agora = func() time.Time { return agora }
	token, _, _ := s.Criar(1, "ana")

	agora = agora.Add(59 * time.Minute)
	if _, ok := s.Buscar(token); !ok {
		t.Fatal("sessão expirou cedo demais")
	}
	agora = agora.Add(time.Minute)
	if _, ok := s.Buscar(token); ok {
		t.Fatal("sessão expirada ainda é válida")
	}
}

func TestTokensUnicos(t *testing.T) {
	s := NewStore(time.Hour)
	a, _, _ := s.Criar(1, "ana")
	b, _, _ := s.Criar(1, "ana")
	if a == b || len(a) < 40 {
		t.Fatalf("tokens fracos ou repetidos: %q %q", a, b)
	}
}

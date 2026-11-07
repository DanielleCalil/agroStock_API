// Package sessao guarda as sessões ("crachás") dos usuários autenticados.
//
// O token é aleatório (256 bits) e só viaja no cookie HttpOnly; o servidor
// mantém o mapa token -> usuário em memória. Reiniciar a API desloga todos.
package sessao

import (
	"crypto/rand"
	"encoding/base64"
	"sync"
	"time"
)

type Sessao struct {
	UsuarioID int
	Login     string
	Expira    time.Time
}

type Store struct {
	mu       sync.Mutex
	sessoes  map[string]Sessao
	ttl      time.Duration
	agora    func() time.Time
	ultLimpa time.Time
}

func NewStore(ttl time.Duration) *Store {
	return &Store{sessoes: make(map[string]Sessao), ttl: ttl, agora: time.Now}
}

// Criar gera um novo token de sessão para o usuário.
func (s *Store) Criar(usuarioID int, login string) (string, Sessao, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", Sessao{}, err
	}
	token := base64.RawURLEncoding.EncodeToString(b)

	s.mu.Lock()
	defer s.mu.Unlock()
	agora := s.agora()
	s.limparExpiradas(agora)
	ses := Sessao{UsuarioID: usuarioID, Login: login, Expira: agora.Add(s.ttl)}
	s.sessoes[token] = ses
	return token, ses, nil
}

// Buscar devolve a sessão se o token existir e não estiver expirado.
func (s *Store) Buscar(token string) (Sessao, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ses, ok := s.sessoes[token]
	if !ok {
		return Sessao{}, false
	}
	if !s.agora().Before(ses.Expira) {
		delete(s.sessoes, token)
		return Sessao{}, false
	}
	return ses, true
}

func (s *Store) Remover(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessoes, token)
}

// limparExpiradas varre o mapa no máximo uma vez por minuto. Exige s.mu travado.
func (s *Store) limparExpiradas(agora time.Time) {
	if agora.Sub(s.ultLimpa) < time.Minute {
		return
	}
	s.ultLimpa = agora
	for t, ses := range s.sessoes {
		if !agora.Before(ses.Expira) {
			delete(s.sessoes, t)
		}
	}
}

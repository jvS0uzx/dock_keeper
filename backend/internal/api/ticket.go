package api

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"sync"
	"time"

	"github.com/joaov/vd_stats/internal/auth"
)

// Validade de um ticket de stream. Curta de propósito: ele existe só para
// cobrir o intervalo entre o fetch autenticado e o EventSource abrir.
const ticketTTL = 30 * time.Second

// ticketStore guarda tickets de uso único para as rotas de SSE.
//
// O EventSource do browser não deixa definir cabeçalhos, então o segredo
// precisa ir na URL. Mandar o API_TOKEN ali significa gravá-lo no access log do
// Nginx, no histórico do navegador e em qualquer proxy no caminho — e ele é
// permanente. O ticket resolve isso: vale uma vez, expira em 30s e não serve
// para nenhuma outra rota.
type ticketStore struct {
	mu     sync.Mutex
	issued map[string]ticketEntry
}

// ticketEntry amarra o ticket a quem o pediu.
//
// Sem a sessão guardada aqui o stream abriria sem identidade, e o handler não
// teria com que recortar por unidade: um visualizador de uma filial pedia
// ticket e lia o auth.log de qualquer VPS do parque.
type ticketEntry struct {
	expires time.Time
	session auth.Session
}

func newTicketStore() *ticketStore {
	return &ticketStore{issued: make(map[string]ticketEntry)}
}

// issue cria um ticket novo para a sessão informada e descarta os vencidos.
func (s *ticketStore) issue(session auth.Session) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	ticket := base64.RawURLEncoding.EncodeToString(raw)

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for t, entry := range s.issued {
		if now.After(entry.expires) {
			delete(s.issued, t)
		}
	}
	s.issued[ticket] = ticketEntry{expires: now.Add(ticketTTL), session: session}
	return ticket, nil
}

// consume valida o ticket, o invalida na mesma operação (uso único) e devolve
// a sessão de quem o pediu.
func (s *ticketStore) consume(ticket string) (auth.Session, bool) {
	if ticket == "" {
		return auth.Session{}, false
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.issued[ticket]
	if !ok {
		return auth.Session{}, false
	}
	delete(s.issued, ticket)
	if !time.Now().Before(entry.expires) {
		return auth.Session{}, false
	}
	return entry.session, true
}

// streamTicketHandler entrega um ticket para abrir um SSE. A rota em si é
// autenticada normalmente, por cabeçalho, e o ticket sai amarrado à sessão que
// a autenticou: o stream vale o que a pessoa vale, nada além.
func (c Config) streamTicketHandler(w http.ResponseWriter, r *http.Request) {
	ticket, err := c.tickets.issue(sessionFrom(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "falha ao gerar ticket")
		return
	}
	// no-store: o ticket não pode sobrar em cache de browser nem de proxy.
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{
		"ticket":     ticket,
		"expires_in": int(ticketTTL.Seconds()),
	})
}

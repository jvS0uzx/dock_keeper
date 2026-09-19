package api

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"sync"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/auth"
)

const ticketTTL = 30 * time.Second

type ticketStore struct {
	mu     sync.Mutex
	issued map[string]ticketEntry
}

type ticketEntry struct {
	expires time.Time
	session auth.Session
}

func newTicketStore() *ticketStore {
	return &ticketStore{issued: make(map[string]ticketEntry)}
}

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

func (c Config) streamTicketHandler(w http.ResponseWriter, r *http.Request) {
	ticket, err := c.tickets.issue(sessionFrom(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "falha ao gerar ticket")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{
		"ticket":     ticket,
		"expires_in": int(ticketTTL.Seconds()),
	})
}

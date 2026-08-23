package ssh

import (
	"fmt"
	"io"
	"net/http"
	"sync"
)

// sseWriter serializa a escrita no http.ResponseWriter. O stream de logs lê
// stdout e stderr em goroutines separadas: sem o mutex as duas escrevem no
// mesmo ResponseWriter ao mesmo tempo, o que é corrida de dados.
type sseWriter struct {
	mu      sync.Mutex
	w       io.Writer
	flusher http.Flusher
}

func newSSEWriter(w io.Writer, flusher http.Flusher) *sseWriter {
	return &sseWriter{w: w, flusher: flusher}
}

func (s *sseWriter) send(line string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fmt.Fprintf(s.w, "data: %s\n\n", line)
	s.flusher.Flush()
}

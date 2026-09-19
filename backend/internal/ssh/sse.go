package ssh

import (
	"fmt"
	"io"
	"net/http"
	"sync"
)

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

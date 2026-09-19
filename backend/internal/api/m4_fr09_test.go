package api

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestShutdownCortaStreamAberto(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	aberto := make(chan struct{})
	mux := http.NewServeMux()
	mux.HandleFunc("/stream", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		close(aberto)
		<-r.Context().Done()
	})

	ouvinte, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("abrir porta: %v", err)
	}
	srv := &http.Server{
		Handler:     mux,
		BaseContext: func(net.Listener) context.Context { return ctx },
	}
	go func() { _ = srv.Serve(ouvinte) }()

	resp, err := http.Get(fmt.Sprintf("http://%s/stream", ouvinte.Addr()))
	if err != nil {
		t.Fatalf("abrir stream: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	<-aberto

	cancel()
	inicio := time.Now()
	desligar, pare := context.WithTimeout(context.Background(), 5*time.Second)
	defer pare()
	if err := srv.Shutdown(desligar); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	if gasto := time.Since(inicio); gasto > 2*time.Second {
		t.Errorf("Shutdown levou %s com um SSE aberto; esperado encerrar assim que o sinal cancela o contexto", gasto)
	}
	if _, err := io.ReadAll(resp.Body); err == nil {
		t.Log("o corpo do stream terminou sem erro, o que também indica encerramento")
	}
}

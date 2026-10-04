package ssh

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"
)

func TestStreamsDeVidaLongaFechamConexaoMeioAberta(t *testing.T) {
	casos := map[string]func(ctx context.Context, alvo Target) error{
		"logs do container": func(ctx context.Context, alvo Target) error {
			rec := httptest.NewRecorder()
			return StreamDockerLogs(ctx, alvo, "api", rec, rec)
		},
		"auth.log no SSE": func(ctx context.Context, alvo Target) error {
			rec := httptest.NewRecorder()
			return StreamAuthLogs(ctx, alvo, rec, rec)
		},
		"vigia do auth.log": StartAuthWatch,
		"stream do Nginx":   StartNginxStream,
	}

	for nome, abrir := range casos {
		t.Run(nome, func(t *testing.T) {
			keepaliveRapido(t)
			t.Setenv("RTT_PROBE_INTERVAL", "20ms")
			t.Setenv("SSH_KEEPALIVE_MAX_MISSES", "3")
			srv := novoServidorKeepalive(t)
			srv.responder.Store(false)
			alvo := srv.alvo(t, "srv-ka-sob-demanda")
			alvo.ID = ""

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			fim := make(chan struct{})
			go func() {
				_ = abrir(ctx, alvo)
				close(fim)
			}()

			select {
			case <-fim:
			case <-time.After(5 * time.Second):
				cancel()
				<-fim
				t.Fatal("com o servidor mudo o stream continuou preso; esperado que o keepalive fechasse a conexão")
			}
		})
	}
}

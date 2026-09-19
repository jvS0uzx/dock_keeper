package alert

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func resetSaude() {
	saudeMu.Lock()
	defer saudeMu.Unlock()

	estado, detalhe, verificadoEm = EstadoDesligado, "", time.Time{}
}

func fakeTelegramDinamico(t *testing.T, responder func(metodo string) string) {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		metodo := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(responder(metodo)))
	}))
	t.Cleanup(srv.Close)

	base := apiBase
	apiBase = srv.URL
	t.Cleanup(func() { apiBase = base })
}

func vigiaRapida(t *testing.T) {
	t.Helper()

	base, teto := saudeBase, saudeMax
	saudeBase, saudeMax = 10*time.Millisecond, 40*time.Millisecond
	t.Cleanup(func() { saudeBase, saudeMax = base, teto })
}

func esperarEstado(t *testing.T, quer string, prazo time.Duration) {
	t.Helper()

	limite := time.Now().Add(prazo)
	for time.Now().Before(limite) {
		if Status().Estado == quer {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("estado = %q, esperado %q em até %s", Status().Estado, quer, prazo)
}

func TestFalhaNaVerificacaoDoBootDeixaLigadoEDegradado(t *testing.T) {
	resetState()
	fakeTelegram(t, map[string]string{
		"getMe": `{"ok":false,"description":"Bad Gateway"}`,
	})
	t.Setenv("TELEGRAM_BOT_TOKEN", "123:abc")
	t.Setenv("TELEGRAM_CHAT_ID", "123456789")

	Init()

	if !enabled {
		t.Error("uma falha de verificação no boot desligou o Telegram até o restart")
	}
	saude := Status()
	if saude.Estado != EstadoDegradado {
		t.Errorf("estado = %q, esperado %q", saude.Estado, EstadoDegradado)
	}
	if saude.Detalhe == "" {
		t.Error("estado degradado sem detalhe do motivo")
	}
}

func TestSemCredenciaisOEstadoEDesligado(t *testing.T) {
	resetState()
	t.Setenv("TELEGRAM_BOT_TOKEN", "")
	t.Setenv("TELEGRAM_CHAT_ID", "")

	Init()

	if enabled {
		t.Error("Telegram ligado sem credencial")
	}
	if e := Status().Estado; e != EstadoDesligado {
		t.Errorf("estado = %q, esperado %q", e, EstadoDesligado)
	}
}

func TestVigiaRevalidaEVoltaParaOK(t *testing.T) {
	resetState()
	vigiaRapida(t)

	var falhar atomic.Bool
	falhar.Store(true)
	fakeTelegramDinamico(t, func(metodo string) string {
		if falhar.Load() {
			return `{"ok":false,"description":"Bad Gateway"}`
		}
		switch metodo {
		case "getMe":
			return `{"ok":true,"result":{"username":"dockkeeper_bot"}}`
		default:
			return `{"ok":true,"result":{"id":123456789}}`
		}
	})
	t.Setenv("TELEGRAM_BOT_TOKEN", "123:abc")
	t.Setenv("TELEGRAM_CHAT_ID", "123456789")

	Init()
	if e := Status().Estado; e != EstadoDegradado {
		t.Fatalf("estado depois do boot com falha = %q, esperado %q", e, EstadoDegradado)
	}

	ctx, cancel := context.WithCancel(context.Background())
	fim := StartHealthWatch(ctx)
	t.Cleanup(func() {
		cancel()
		<-fim
	})

	falhar.Store(false)
	esperarEstado(t, EstadoOK, 3*time.Second)
}

func TestVigiaParaNoCancelamentoSemVazarGoroutine(t *testing.T) {
	resetState()
	vigiaRapida(t)
	fakeTelegram(t, map[string]string{
		"getMe":   `{"ok":true,"result":{"username":"dockkeeper_bot"}}`,
		"getChat": `{"ok":true,"result":{"id":123456789}}`,
	})
	t.Setenv("TELEGRAM_BOT_TOKEN", "123:abc")
	t.Setenv("TELEGRAM_CHAT_ID", "123456789")
	Init()

	base := runtime.NumGoroutine()
	ctx, cancel := context.WithCancel(context.Background())
	fim := StartHealthWatch(ctx)
	time.Sleep(30 * time.Millisecond)
	cancel()

	select {
	case <-fim:
	case <-time.After(2 * time.Second):
		t.Fatal("a vigia do canal não parou depois do cancelamento")
	}

	prazo := time.Now().Add(2 * time.Second)
	for runtime.NumGoroutine() > base && time.Now().Before(prazo) {
		time.Sleep(10 * time.Millisecond)
	}
	if n := runtime.NumGoroutine(); n > base {
		t.Errorf("goroutines = %d, esperado no máximo %d depois do cancelamento", n, base)
	}
}

func TestEnvioTentaMesmoDegradadoEVoltaParaOK(t *testing.T) {
	resetState()
	calls := fakeTelegram(t, map[string]string{
		"getMe":       `{"ok":true,"result":{"username":"dockkeeper_bot"}}`,
		"getChat":     `{"ok":true,"result":{"id":123456789}}`,
		"sendMessage": `{"ok":true,"result":{}}`,
	})
	t.Setenv("TELEGRAM_BOT_TOKEN", "123:abc")
	t.Setenv("TELEGRAM_CHAT_ID", "123456789")
	Init()

	marcarDegradado(errors.New("queda temporária"))
	if e := Status().Estado; e != EstadoDegradado {
		t.Fatalf("estado = %q, esperado %q", e, EstadoDegradado)
	}

	antes := len(*calls)
	Send("[ALERTA] teste de envio degradado")

	if len(*calls) != antes+1 {
		t.Fatalf("chamadas à API = %d, esperado %d: o envio não foi tentado no estado degradado", len(*calls)-antes, 1)
	}
	if e := Status().Estado; e != EstadoOK {
		t.Errorf("estado depois de um envio bem-sucedido = %q, esperado %q", e, EstadoOK)
	}
}

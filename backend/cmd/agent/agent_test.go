package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func capturarLog(t *testing.T) *syncBuffer {
	t.Helper()
	buf := &syncBuffer{}
	anterior := log.Writer()
	log.SetOutput(buf)
	t.Cleanup(func() { log.SetOutput(anterior) })
	return buf
}

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func envDe(valores map[string]string) func(string) string {
	return func(k string) string { return valores[k] }
}

func TestLoadConfigExigeServidor(t *testing.T) {
	if _, err := loadConfig(envDe(nil)); err == nil {
		t.Fatal("sem AGENT_SERVER_URL a configuração deveria ser recusada")
	}
}

func TestLoadConfigLeVariaveis(t *testing.T) {
	cfg, err := loadConfig(envDe(map[string]string{
		"AGENT_SERVER_URL": "https://painel.exemplo.com",
		"AGENT_HOSTNAME":   "estacao-01",
		"AGENT_SITE":       "  Matriz ",
		"AGENT_INTERVAL":   "30",
	}))
	if err != nil {
		t.Fatalf("configuração válida recusada: %v", err)
	}
	if cfg.serverURL != "https://painel.exemplo.com" || cfg.hostname != "estacao-01" || cfg.siteCode != "matriz" {
		t.Errorf("configuração lida errada: %+v", cfg)
	}
	if cfg.interval != 30*time.Second {
		t.Errorf("intervalo = %s, esperado 30s", cfg.interval)
	}
}

func TestLoadConfigIntervaloInvalidoCaiNoPadrao(t *testing.T) {
	for _, bruto := range []string{"", "0", "-3", "abc"} {
		cfg, err := loadConfig(envDe(map[string]string{
			"AGENT_SERVER_URL": "http://127.0.0.1:8080",
			"AGENT_INTERVAL":   bruto,
		}))
		if err != nil {
			t.Fatalf("AGENT_INTERVAL=%q: %v", bruto, err)
		}
		if cfg.interval != defaultIntervalSec*time.Second {
			t.Errorf("AGENT_INTERVAL=%q deu %s, esperado %ds", bruto, cfg.interval, defaultIntervalSec)
		}
		if cfg.hostname == "" {
			t.Errorf("sem AGENT_HOSTNAME o hostname do sistema deveria ser usado")
		}
	}
}

func TestPushCredencialPropriaEnviaCabecalhosDoDispositivo(t *testing.T) {
	var id, token, legado string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, token, legado = r.Header.Get("X-Device-Id"), r.Header.Get("X-Device-Token"), r.Header.Get("X-Agent-Token")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	err := push(context.Background(), srv.Client(), srv.URL, credential{DeviceID: "d1", Token: "t1"}, "", metricsPayload{})
	if err != nil {
		t.Fatalf("envio com 200 devolveu erro: %v", err)
	}
	if id != "d1" || token != "t1" || legado != "" {
		t.Errorf("cabeçalhos = (%q, %q, legado %q)", id, token, legado)
	}
}

func TestPushLegadoEnviaTokenCompartilhado(t *testing.T) {
	var legado, id string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		legado, id = r.Header.Get("X-Agent-Token"), r.Header.Get("X-Device-Id")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if err := push(context.Background(), srv.Client(), srv.URL, credential{}, "compartilhado", metricsPayload{}); err != nil {
		t.Fatalf("envio legado devolveu erro: %v", err)
	}
	if legado != "compartilhado" || id != "" {
		t.Errorf("cabeçalhos legado = (%q, device %q)", legado, id)
	}
}

func TestPushClassificaResposta(t *testing.T) {
	casos := []struct {
		status     int
		credencial bool
		ok         bool
	}{
		{http.StatusOK, false, true},
		{http.StatusUnauthorized, true, false},
		{http.StatusForbidden, true, false},
		{http.StatusInternalServerError, false, false},
		{http.StatusConflict, false, false},
	}
	for _, c := range casos {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(c.status)
		}))
		err := push(context.Background(), srv.Client(), srv.URL, credential{DeviceID: "d", Token: "t"}, "", metricsPayload{})
		srv.Close()

		if c.ok != (err == nil) {
			t.Errorf("HTTP %d: erro = %v, esperado sucesso=%v", c.status, err, c.ok)
			continue
		}
		if got := errors.Is(err, errCredencialRecusada); got != c.credencial {
			t.Errorf("HTTP %d: credencial recusada = %v, esperado %v (erro %v)", c.status, got, c.credencial, err)
		}
	}
}

func TestDescreverFalhaDistingueCredencialDeRede(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close()

	errRede := push(context.Background(), &http.Client{Timeout: time.Second}, url, credential{}, "x", metricsPayload{})
	if errRede == nil {
		t.Fatal("servidor fechado deveria dar erro de rede")
	}
	if errors.Is(errRede, errCredencialRecusada) {
		t.Fatal("falha de rede classificada como credencial recusada")
	}

	rede := descreverFalha(errRede)
	credencial := descreverFalha(errCredencialRecusadaHTTP(http.StatusUnauthorized))
	http500 := descreverFalha(&httpError{status: http.StatusInternalServerError})

	if !strings.HasPrefix(rede, "falha de rede") {
		t.Errorf("mensagem de rede = %q", rede)
	}
	if !strings.HasPrefix(credencial, "credencial recusada pelo painel (HTTP 401)") {
		t.Errorf("mensagem de credencial = %q", credencial)
	}
	if !strings.Contains(http500, "HTTP 500") || strings.HasPrefix(http500, "credencial") || strings.HasPrefix(http500, "falha de rede") {
		t.Errorf("mensagem de HTTP 500 = %q", http500)
	}
}

func agenteDeTeste(url string, intervalo time.Duration) *agente {
	return &agente{
		client:   &http.Client{Timeout: 5 * time.Second},
		endpoint: url,
		cred:     credential{DeviceID: "d", Token: "t"},
		interval: intervalo,
		coletar:  func() metricsPayload { return metricsPayload{Hostname: "h"} },
	}
}

func esperarRun(t *testing.T, a *agente, ctx context.Context, limite time.Duration) {
	t.Helper()
	fim := make(chan struct{})
	go func() {
		a.run(ctx)
		close(fim)
	}()
	<-ctx.Done()
	select {
	case <-fim:
	case <-time.After(limite):
		t.Fatalf("o laço não encerrou em %s depois do cancelamento", limite)
	}
}

func TestRunEncerraEmAteUmSegundoAoCancelar(t *testing.T) {
	capturarLog(t)
	ctx, cancel := context.WithCancel(context.Background())
	var envios atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if envios.Add(1) == 1 {
			cancel()
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	inicio := time.Now()
	esperarRun(t, agenteDeTeste(srv.URL, time.Hour), ctx, time.Second)
	if d := time.Since(inicio); d > time.Second {
		t.Errorf("encerramento levou %s, limite 1s", d)
	}
	if n := envios.Load(); n != 1 {
		t.Errorf("envios = %d, esperado 1", n)
	}
}

func TestRunCancelaEnvioEmAndamento(t *testing.T) {
	capturarLog(t)
	ctx, cancel := context.WithCancel(context.Background())
	chegou := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		close(chegou)
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	}))
	defer srv.Close()

	go func() {
		<-chegou
		cancel()
	}()
	esperarRun(t, agenteDeTeste(srv.URL, time.Hour), ctx, time.Second)
}

func TestRunNaoEnviaQuandoCanceladoDuranteColeta(t *testing.T) {
	capturarLog(t)
	ctx, cancel := context.WithCancel(context.Background())
	var envios atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		envios.Add(1)
	}))
	defer srv.Close()

	a := agenteDeTeste(srv.URL, time.Hour)
	a.coletar = func() metricsPayload {
		cancel()
		return metricsPayload{}
	}
	esperarRun(t, a, ctx, time.Second)
	if n := envios.Load(); n != 0 {
		t.Errorf("envio feito depois do cancelamento: %d", n)
	}
}

func TestRunSegueVivoDepoisDeCredencialRecusada(t *testing.T) {
	saida := capturarLog(t)
	ctx, cancel := context.WithCancel(context.Background())
	var envios atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		switch envios.Add(1) {
		case 1:
			w.WriteHeader(http.StatusUnauthorized)
		case 2:
			w.WriteHeader(http.StatusForbidden)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	a := agenteDeTeste(srv.URL, 10*time.Millisecond)
	var ciclos atomic.Int32
	a.coletar = func() metricsPayload {
		if ciclos.Add(1) == 4 {
			cancel()
		}
		return metricsPayload{}
	}
	esperarRun(t, a, ctx, 2*time.Second)

	if n := envios.Load(); n < 3 {
		t.Fatalf("o laço parou depois da recusa: %d envios", n)
	}
	log := saida.String()
	for _, trecho := range []string{"credencial recusada pelo painel (HTTP 401)", "credencial recusada pelo painel (HTTP 403)", "enviado"} {
		if !strings.Contains(log, trecho) {
			t.Errorf("log sem %q:\n%s", trecho, log)
		}
	}
}

func TestRunRegistraFalhaDeRedeSemConfundirComCredencial(t *testing.T) {
	saida := capturarLog(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	var ciclos atomic.Int32
	a := agenteDeTeste(url, 10*time.Millisecond)
	a.coletar = func() metricsPayload {
		if ciclos.Add(1) == 3 {
			cancel()
		}
		return metricsPayload{}
	}
	esperarRun(t, a, ctx, 2*time.Second)

	log := saida.String()
	if !strings.Contains(log, "falha de rede") {
		t.Errorf("log sem falha de rede:\n%s", log)
	}
	if strings.Contains(log, "credencial recusada") {
		t.Errorf("falha de rede registrada como credencial recusada:\n%s", log)
	}
}

func TestCollectPreencheIdentificacao(t *testing.T) {
	capturarLog(t)
	p := collect("estacao-01", "matriz", 7)
	if p.Hostname != "estacao-01" || p.SiteCode != "matriz" || p.AgentVersion != Version || p.ReportIntervalSec != 7 {
		t.Errorf("identificação da amostra errada: %+v", p)
	}
	if p.MemTotal <= 0 {
		t.Errorf("memória total = %d, esperado > 0", p.MemTotal)
	}
}

func TestFormatTemp(t *testing.T) {
	if got := formatTemp(nil); got != "sem sensor" {
		t.Errorf("sem sensor = %q", got)
	}
	v := 41.25
	if got := formatTemp(&v); got != "41.2°C" && got != "41.3°C" {
		t.Errorf("41.25 = %q", got)
	}
}

func TestRecusaNoModoLegadoExplicaAFlagDoPainel(t *testing.T) {
	saida := capturarLog(t)
	ctx, cancel := context.WithCancel(context.Background())
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	a := agenteDeTeste(srv.URL, 10*time.Millisecond)
	a.cred = credential{}
	a.legado = "compartilhado"
	var ciclos atomic.Int32
	a.coletar = func() metricsPayload {
		if ciclos.Add(1) == 2 {
			cancel()
		}
		return metricsPayload{}
	}
	esperarRun(t, a, ctx, time.Second)

	log := saida.String()
	if !strings.Contains(log, "ALLOW_LEGACY_INGEST_TOKEN=true") || !strings.Contains(log, "AGENT_ENROLL_TOKEN") {
		t.Errorf("recusa do token legado sem orientação:\n%s", log)
	}
}

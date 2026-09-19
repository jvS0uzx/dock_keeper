package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCarregarArquivoDeEnv(t *testing.T) {
	caminho := filepath.Join(t.TempDir(), "agent.env")
	conteudo := "# comentario do modelo\r\n" +
		"\r\n" +
		"AGENT_SERVER_URL=https://painel.exemplo.com\r\n" +
		"  AGENT_SITE =  matriz  \r\n" +
		"AGENT_ENROLL_TOKEN=\"convite com espaco\"\r\n" +
		"AGENT_HOSTNAME='pc-01'\r\n" +
		"AGENT_EXTRA=a=b=c\r\n" +
		"#AGENT_INTERVAL=99\r\n" +
		"linha sem igual\r\n" +
		"AGENT_TOKEN=\n"
	if err := os.WriteFile(caminho, []byte(conteudo), 0o600); err != nil {
		t.Fatal(err)
	}

	lidas := map[string]string{}
	err := carregarArquivoDeEnv(caminho, func(k, v string) error {
		lidas[k] = v
		return nil
	})
	if err != nil {
		t.Fatalf("carregar: %v", err)
	}

	esperado := map[string]string{
		"AGENT_SERVER_URL":   "https://painel.exemplo.com",
		"AGENT_SITE":         "matriz",
		"AGENT_ENROLL_TOKEN": "convite com espaco",
		"AGENT_HOSTNAME":     "pc-01",
		"AGENT_EXTRA":        "a=b=c",
		"AGENT_TOKEN":        "",
	}
	if len(lidas) != len(esperado) {
		t.Errorf("variáveis lidas = %v, esperado %v", lidas, esperado)
	}
	for k, v := range esperado {
		if got, ok := lidas[k]; !ok || got != v {
			t.Errorf("%s = %q (presente=%v), esperado %q", k, got, ok, v)
		}
	}
}

func TestArquivoDeEnvAusenteDizOCaminho(t *testing.T) {
	caminho := filepath.Join(t.TempDir(), "nao-existe.env")
	err := carregarArquivoDeEnv(caminho, func(string, string) error { return nil })
	if err == nil || !strings.Contains(err.Error(), caminho) {
		t.Errorf("erro = %v, esperado citar %s", err, caminho)
	}
}

func TestCaminhoDoArquivoDeEnv(t *testing.T) {
	padrao := caminhoDoArquivoDeEnv(envDe(map[string]string{"ProgramData": `C:\ProgramData`}))
	if padrao != filepath.Join(`C:\ProgramData`, "dockkeeper-agent", "agent.env") {
		t.Errorf("caminho padrão = %q", padrao)
	}
	if got := caminhoDoArquivoDeEnv(envDe(map[string]string{"AGENT_ENV_FILE": "/tmp/outro.env"})); got != "/tmp/outro.env" {
		t.Errorf("AGENT_ENV_FILE ignorado: %q", got)
	}
}

func TestIniciarLeConfigECredencialEEnviaAoPainel(t *testing.T) {
	capturarLog(t)
	caminhoCredencialTemporario(t)
	if err := saveCredential(credential{DeviceID: "dev-i", Token: "tok-i", SiteID: 1}); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	var rota, dispositivo string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rota, dispositivo = r.URL.Path, r.Header.Get("X-Device-Id")
		w.WriteHeader(http.StatusOK)
		cancel()
	}))
	defer srv.Close()

	t.Setenv("AGENT_SERVER_URL", srv.URL)
	t.Setenv("AGENT_INTERVAL", "3600")
	t.Setenv("AGENT_MACHINE_ID", "m-i")

	fim := make(chan struct{})
	go func() {
		iniciar(ctx)
		close(fim)
	}()
	select {
	case <-fim:
	case <-time.After(5 * time.Second):
		t.Fatal("iniciar não encerrou depois do cancelamento")
	}

	if rota != "/api/ingest/metrics" || dispositivo != "dev-i" {
		t.Errorf("envio = rota %q dispositivo %q", rota, dispositivo)
	}
}

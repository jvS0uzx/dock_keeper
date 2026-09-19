package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func caminhoCredencialTemporario(t *testing.T) string {
	t.Helper()
	caminho := filepath.Join(t.TempDir(), "vd-agent", "credential.json")
	t.Setenv("AGENT_CREDENTIAL_PATH", caminho)
	return caminho
}

func TestSalvarECarregarCredencial(t *testing.T) {
	caminho := caminhoCredencialTemporario(t)
	c := credential{DeviceID: "dev-1", Token: "tok-1", SiteID: 4, Kind: "agent"}

	if err := saveCredential(c); err != nil {
		t.Fatalf("gravar credencial: %v", err)
	}
	info, err := os.Stat(caminho)
	if err != nil {
		t.Fatalf("credencial não gravada: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("permissão da credencial = %o, esperado 600", perm)
	}

	lida, ok := loadCredential()
	if !ok || lida != c {
		t.Errorf("credencial lida = %+v (ok=%v), esperado %+v", lida, ok, c)
	}
}

func TestCarregarCredencialIlegivelOuIncompleta(t *testing.T) {
	caminho := caminhoCredencialTemporario(t)
	capturarLog(t)

	if _, ok := loadCredential(); ok {
		t.Fatal("sem arquivo não deveria haver credencial")
	}
	if err := os.MkdirAll(filepath.Dir(caminho), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, conteudo := range []string{"{nao e json", `{"device_id":"d"}`} {
		if err := os.WriteFile(caminho, []byte(conteudo), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, ok := loadCredential(); ok {
			t.Errorf("conteúdo %q aceito como credencial", conteudo)
		}
	}
}

func TestMachineIDPrefereVariavel(t *testing.T) {
	t.Setenv("AGENT_MACHINE_ID", "  maquina-fixa  ")
	if got := machineID(); got != "maquina-fixa" {
		t.Errorf("machineID = %q", got)
	}
}

func TestIdentificadorDeReservaPersiste(t *testing.T) {
	caminhoCredencialTemporario(t)
	primeiro := persistedFallbackID()
	if len(primeiro) != 32 {
		t.Fatalf("identificador de reserva = %q, esperado 32 hex", primeiro)
	}
	if segundo := persistedFallbackID(); segundo != primeiro {
		t.Errorf("identificador mudou entre chamadas: %q -> %q", primeiro, segundo)
	}
}

func servidorDeEnroll(t *testing.T, status int, resposta any) (*httptest.Server, *map[string]string) {
	t.Helper()
	recebido := map[string]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/enroll" {
			http.NotFound(w, r)
			return
		}
		json.NewDecoder(r.Body).Decode(&recebido)
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(resposta)
	}))
	t.Cleanup(srv.Close)
	return srv, &recebido
}

func TestEnrollDevolveCredencial(t *testing.T) {
	t.Setenv("AGENT_MACHINE_ID", "m-1")
	srv, recebido := servidorDeEnroll(t, http.StatusCreated, credential{DeviceID: "d9", Token: "t9", SiteID: 2, Kind: "agent"})

	c, err := enroll(srv.Client(), srv.URL, "convite", "estacao")
	if err != nil {
		t.Fatalf("enroll: %v", err)
	}
	if c.DeviceID != "d9" || c.Token != "t9" || c.SiteID != 2 {
		t.Errorf("credencial = %+v", c)
	}
	r := *recebido
	if r["enrollment_token"] != "convite" || r["kind"] != "agent" || r["machine_id"] != "m-1" || r["hostname"] != "estacao" {
		t.Errorf("corpo do enroll = %v", r)
	}
}

func TestEnrollRecusadoOuIncompleto(t *testing.T) {
	t.Setenv("AGENT_MACHINE_ID", "m-1")
	recusado, _ := servidorDeEnroll(t, http.StatusConflict, map[string]string{"error": "convite já usado"})
	if _, err := enroll(recusado.Client(), recusado.URL, "c", "h"); err == nil || !strings.Contains(err.Error(), "409") {
		t.Errorf("enroll recusado devolveu %v", err)
	}

	incompleto, _ := servidorDeEnroll(t, http.StatusCreated, map[string]string{"device_id": "d"})
	if _, err := enroll(incompleto.Client(), incompleto.URL, "c", "h"); err == nil {
		t.Error("credencial sem token aceita")
	}
}

func TestResolverIdentidadeUsaCredencialGravada(t *testing.T) {
	caminhoCredencialTemporario(t)
	capturarLog(t)
	gravada := credential{DeviceID: "dg", Token: "tg", SiteID: 1}
	if err := saveCredential(gravada); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGENT_ENROLL_TOKEN", "nao-deve-ser-usado")

	c, legado := resolverIdentidade(http.DefaultClient, "http://127.0.0.1:1", "h")
	if c.DeviceID != "dg" || legado != "" {
		t.Errorf("identidade = %+v, legado %q", c, legado)
	}
}

func TestResolverIdentidadeFazEnrollEGrava(t *testing.T) {
	caminhoCredencialTemporario(t)
	capturarLog(t)
	t.Setenv("AGENT_MACHINE_ID", "m-2")
	t.Setenv("AGENT_ENROLL_TOKEN", "convite-novo")
	srv, _ := servidorDeEnroll(t, http.StatusCreated, credential{DeviceID: "dn", Token: "tn", SiteID: 3})

	c, legado := resolverIdentidade(srv.Client(), srv.URL, "h")
	if c.DeviceID != "dn" || legado != "" {
		t.Fatalf("identidade = %+v, legado %q", c, legado)
	}
	if gravada, ok := loadCredential(); !ok || gravada.Token != "tn" {
		t.Errorf("credencial do enroll não gravada: %+v (ok=%v)", gravada, ok)
	}
}

func TestResolverIdentidadeModoLegado(t *testing.T) {
	caminhoCredencialTemporario(t)
	capturarLog(t)
	t.Setenv("AGENT_ENROLL_TOKEN", "")
	t.Setenv("AGENT_TOKEN", " compartilhado ")

	c, legado := resolverIdentidade(http.DefaultClient, "http://127.0.0.1:1", "h")
	if c.DeviceID != "" || legado != "compartilhado" {
		t.Errorf("modo legado = %+v, legado %q", c, legado)
	}
}

func TestCaminhoPadraoDaCredencialUsaNomeDockKeeper(t *testing.T) {
	t.Setenv("AGENT_CREDENTIAL_PATH", "")
	t.Setenv("ProgramData", `C:\ProgramData`)
	got := credentialPath()
	esperado := "/var/lib/dockkeeper-agent/credential.json"
	if runtime.GOOS == "windows" {
		esperado = filepath.Join(`C:\ProgramData`, "dockkeeper-agent", "credential.json")
	}
	if got != esperado {
		t.Errorf("credentialPath() = %q, esperado %q", got, esperado)
	}
}

func TestAvisoDoModoLegadoApontaAFlagDoPainel(t *testing.T) {
	caminhoCredencialTemporario(t)
	saida := capturarLog(t)
	t.Setenv("AGENT_ENROLL_TOKEN", "")
	t.Setenv("AGENT_TOKEN", "compartilhado")

	resolverIdentidade(http.DefaultClient, "http://127.0.0.1:1", "h")

	if !strings.Contains(saida.String(), "ALLOW_LEGACY_INGEST_TOKEN=true") {
		t.Errorf("aviso do modo legado não cita a flag do painel: %q", saida.String())
	}
}

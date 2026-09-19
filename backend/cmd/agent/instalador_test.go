package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type instalacao struct {
	raiz    string
	stubs   string
	binario string
	scripts string
}

func novaInstalacao(t *testing.T) instalacao {
	t.Helper()
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash indisponível")
	}
	base := t.TempDir()
	in := instalacao{
		raiz:    filepath.Join(base, "raiz"),
		stubs:   filepath.Join(base, "stubs"),
		binario: filepath.Join(base, "agent-linux-amd64"),
	}
	scripts, err := filepath.Abs(filepath.Join("..", "..", "deploy", "agent"))
	if err != nil {
		t.Fatal(err)
	}
	in.scripts = scripts

	gravar(t, in.binario, "binario-novo", 0o755)
	gravar(t, filepath.Join(in.stubs, "systemctl"),
		"#!/usr/bin/env bash\necho \"$*\" >> \"$DOCKKEEPER_ROOT/systemctl.log\"\nexit 0\n", 0o755)
	gravar(t, filepath.Join(in.stubs, "sleep"), "#!/usr/bin/env bash\nexit 0\n", 0o755)
	if err := os.MkdirAll(in.raiz, 0o755); err != nil {
		t.Fatal(err)
	}
	return in
}

func gravar(t *testing.T, caminho, conteudo string, modo os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(caminho), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(caminho, []byte(conteudo), modo); err != nil {
		t.Fatal(err)
	}
}

func (in instalacao) rodar(t *testing.T, script, entrada string, args ...string) string {
	t.Helper()
	cmd := exec.Command("bash", append([]string{filepath.Join(in.scripts, script)}, args...)...)
	cmd.Env = append(os.Environ(),
		"DOCKKEEPER_ROOT="+in.raiz,
		"PATH="+in.stubs+string(os.PathListSeparator)+os.Getenv("PATH"),
	)
	cmd.Stdin = strings.NewReader(entrada)
	saida, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s falhou: %v\n%s", script, err, saida)
	}
	return string(saida)
}

func (in instalacao) caminho(p string) string { return filepath.Join(in.raiz, p) }

func (in instalacao) ler(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(in.caminho(p))
	if err != nil {
		t.Fatalf("esperava %s: %v", p, err)
	}
	return string(b)
}

func (in instalacao) ausente(t *testing.T, p string) {
	t.Helper()
	if _, err := os.Lstat(in.caminho(p)); err == nil {
		t.Errorf("%s deveria ter sido removido", p)
	}
}

func (in instalacao) modo(t *testing.T, p string, esperado os.FileMode) {
	t.Helper()
	info, err := os.Stat(in.caminho(p))
	if err != nil {
		t.Fatalf("esperava %s: %v", p, err)
	}
	if info.Mode().Perm() != esperado {
		t.Errorf("%s com modo %o, esperado %o", p, info.Mode().Perm(), esperado)
	}
}

func (in instalacao) instalacaoAntiga(t *testing.T) {
	t.Helper()
	gravar(t, in.caminho("usr/local/bin/vd-agent"), "binario-antigo", 0o755)
	gravar(t, in.caminho("etc/systemd/system/vd-agent.service"), "[Unit]\n", 0o644)
	gravar(t, in.caminho("etc/vd-agent.env"),
		"AGENT_SERVER_URL=\"https://painel.exemplo.com\"\nAGENT_CREDENTIAL_PATH=/var/lib/vd-agent/credential.json\n", 0o600)
	gravar(t, in.caminho("var/lib/private/vd-agent/credential.json"), `{"device_id":"d-antigo","device_token":"t-antigo"}`, 0o600)
	gravar(t, in.caminho("var/lib/private/vd-agent/machine-id"), "maquina-antiga\n", 0o600)
	if err := os.Symlink("private/vd-agent", in.caminho("var/lib/vd-agent")); err != nil {
		t.Fatal(err)
	}
}

func TestInstaladorMigraInstalacaoAntiga(t *testing.T) {
	in := novaInstalacao(t)
	in.instalacaoAntiga(t)

	saida := in.rodar(t, "install.sh", "", in.binario)

	if !strings.Contains(saida, "migrando") {
		t.Errorf("saída sem aviso de migração:\n%s", saida)
	}
	if got := in.ler(t, "usr/local/bin/dockkeeper-agent"); got != "binario-novo" {
		t.Errorf("binário novo = %q", got)
	}
	in.modo(t, "usr/local/bin/dockkeeper-agent", 0o755)
	if !strings.Contains(in.ler(t, "etc/systemd/system/dockkeeper-agent.service"), "ExecStart=/usr/local/bin/dockkeeper-agent") {
		t.Error("unit nova não aponta para o binário novo")
	}

	env := in.ler(t, "etc/dockkeeper-agent.env")
	if !strings.Contains(env, "https://painel.exemplo.com") {
		t.Errorf("config antiga não foi levada:\n%s", env)
	}
	if !strings.Contains(env, "AGENT_CREDENTIAL_PATH=/var/lib/dockkeeper-agent/credential.json") || strings.Contains(env, "vd-agent") {
		t.Errorf("caminho da credencial não foi reescrito:\n%s", env)
	}
	in.modo(t, "etc/dockkeeper-agent.env", 0o600)

	if got := in.ler(t, "var/lib/private/dockkeeper-agent/credential.json"); !strings.Contains(got, "d-antigo") {
		t.Errorf("credencial não preservada: %q", got)
	}
	in.modo(t, "var/lib/private/dockkeeper-agent/credential.json", 0o600)
	in.modo(t, "var/lib/private/dockkeeper-agent", 0o700)
	if got := in.ler(t, "var/lib/private/dockkeeper-agent/machine-id"); got != "maquina-antiga\n" {
		t.Errorf("machine-id não preservado: %q", got)
	}

	for _, p := range []string{
		"usr/local/bin/vd-agent",
		"etc/systemd/system/vd-agent.service",
		"etc/vd-agent.env",
		"var/lib/vd-agent",
		"var/lib/private/vd-agent",
	} {
		in.ausente(t, p)
	}

	log := in.ler(t, "systemctl.log")
	for _, trecho := range []string{"disable --now vd-agent.service", "enable dockkeeper-agent", "restart dockkeeper-agent"} {
		if !strings.Contains(log, trecho) {
			t.Errorf("systemctl sem %q:\n%s", trecho, log)
		}
	}
}

func TestInstaladorRodarDuasVezesNaoQuebra(t *testing.T) {
	in := novaInstalacao(t)
	in.instalacaoAntiga(t)

	in.rodar(t, "install.sh", "", in.binario)
	saida := in.rodar(t, "install.sh", "", in.binario)

	if strings.Contains(saida, "migrando") {
		t.Errorf("segunda execução migrou de novo:\n%s", saida)
	}
	if !strings.Contains(saida, "config existente preservada") {
		t.Errorf("segunda execução não preservou a config:\n%s", saida)
	}
	if got := in.ler(t, "var/lib/private/dockkeeper-agent/credential.json"); !strings.Contains(got, "d-antigo") {
		t.Errorf("credencial perdida na segunda execução: %q", got)
	}
	if !strings.Contains(in.ler(t, "etc/dockkeeper-agent.env"), "https://painel.exemplo.com") {
		t.Error("config perdida na segunda execução")
	}
}

func TestInstaladorNovoCriaConfigDoModelo(t *testing.T) {
	in := novaInstalacao(t)

	saida := in.rodar(t, "install.sh", "", in.binario)

	if strings.Contains(saida, "migrando") {
		t.Errorf("instalação nova não deveria migrar:\n%s", saida)
	}
	env := in.ler(t, "etc/dockkeeper-agent.env")
	if !strings.Contains(env, "AGENT_ENROLL_TOKEN=") {
		t.Errorf("config modelo sem AGENT_ENROLL_TOKEN:\n%s", env)
	}
	in.modo(t, "etc/dockkeeper-agent.env", 0o600)
}

func TestInstaladorNaoSobrescreveConfigNovaNemCredencialNova(t *testing.T) {
	in := novaInstalacao(t)
	in.instalacaoAntiga(t)
	gravar(t, in.caminho("etc/dockkeeper-agent.env"), "AGENT_SERVER_URL=\"https://novo.exemplo.com\"\n", 0o600)
	gravar(t, in.caminho("var/lib/private/dockkeeper-agent/credential.json"), `{"device_id":"d-novo","device_token":"t-novo"}`, 0o600)

	in.rodar(t, "install.sh", "", in.binario)

	if env := in.ler(t, "etc/dockkeeper-agent.env"); !strings.Contains(env, "novo.exemplo.com") {
		t.Errorf("config nova sobrescrita:\n%s", env)
	}
	if got := in.ler(t, "var/lib/private/dockkeeper-agent/credential.json"); !strings.Contains(got, "d-novo") {
		t.Errorf("credencial nova sobrescrita: %q", got)
	}
	if got := in.ler(t, "etc/dockkeeper-agent.env.anterior"); !strings.Contains(got, "painel.exemplo.com") {
		t.Errorf("config antiga não guardada como .anterior: %q", got)
	}
	if got := in.ler(t, "var/lib/private/dockkeeper-agent/credential.json.anterior"); !strings.Contains(got, "d-antigo") {
		t.Errorf("credencial antiga não guardada como .anterior: %q", got)
	}
	in.ausente(t, "etc/vd-agent.env")
	in.ausente(t, "var/lib/private/vd-agent")
}

func TestDesinstaladorLimpaNomeNovoEAntigo(t *testing.T) {
	in := novaInstalacao(t)
	in.rodar(t, "install.sh", "", in.binario)
	gravar(t, in.caminho("usr/local/bin/vd-agent"), "sobra", 0o755)
	gravar(t, in.caminho("etc/systemd/system/vd-agent.service"), "[Unit]\n", 0o644)
	gravar(t, in.caminho("etc/vd-agent.env"), "AGENT_TOKEN=x\n", 0o600)

	in.rodar(t, "uninstall.sh", "s\n")

	for _, p := range []string{
		"usr/local/bin/dockkeeper-agent",
		"etc/systemd/system/dockkeeper-agent.service",
		"etc/dockkeeper-agent.env",
		"usr/local/bin/vd-agent",
		"etc/systemd/system/vd-agent.service",
		"etc/vd-agent.env",
	} {
		in.ausente(t, p)
	}
}

func TestDesinstaladorPreservaConfigQuandoRecusado(t *testing.T) {
	in := novaInstalacao(t)
	in.rodar(t, "install.sh", "", in.binario)

	in.rodar(t, "uninstall.sh", "n\n")

	in.ausente(t, "usr/local/bin/dockkeeper-agent")
	in.ler(t, "etc/dockkeeper-agent.env")
}

func TestInstaladorMigraEstadoForaDoDiretorioPrivado(t *testing.T) {
	in := novaInstalacao(t)
	gravar(t, in.caminho("etc/vd-agent.env"), "AGENT_SERVER_URL=\"https://painel.exemplo.com\"\n", 0o600)
	gravar(t, in.caminho("var/lib/vd-agent/credential.json"), `{"device_id":"d-real","device_token":"t"}`, 0o600)

	in.rodar(t, "install.sh", "", in.binario)

	if got := in.ler(t, "var/lib/private/dockkeeper-agent/credential.json"); !strings.Contains(got, "d-real") {
		t.Errorf("credencial fora do diretório privado não migrou: %q", got)
	}
	in.ausente(t, "var/lib/vd-agent")
}

func TestDesinstaladorApagaCredencialAntigaQuandoConfirmado(t *testing.T) {
	in := novaInstalacao(t)
	gravar(t, in.caminho("var/lib/private/vd-agent/credential.json"), `{"device_id":"d"}`, 0o600)
	if err := os.Symlink("private/vd-agent", in.caminho("var/lib/vd-agent")); err != nil {
		t.Fatal(err)
	}

	in.rodar(t, "uninstall.sh", "s\n")

	in.ausente(t, "var/lib/private/vd-agent")
	in.ausente(t, "var/lib/vd-agent")
}

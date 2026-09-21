package scripts

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type sondaNginx struct {
	Instalado    bool   `json:"instalado"`
	Ativo        bool   `json:"ativo"`
	ConfigLida   bool   `json:"config_lida"`
	ConfigMotivo string `json:"config_motivo"`
	LogLegivel   bool   `json:"log_legivel"`
	LogMotivo    string `json:"log_motivo"`
	LogCaminho   string `json:"log_caminho"`
	Usuario      string `json:"usuario"`
	Upstreams    []struct {
		Bloco    string   `json:"bloco"`
		Destinos []string `json:"destinos"`
	} `json:"upstreams"`
}

const configComUpstream = `
http {
    upstream api {
        server 203.0.113.10:8080;
        server 203.0.113.11:8080 backup;
    }

    upstream painel{
        least_conn;
        server 198.51.100.5:3000 weight=2;
    }

    server {
        listen 80;
        server_name exemplo.com.br;
    }
}
`

const configSemUpstream = `
http {
    server {
        listen 80;
        server_name exemplo.com.br;
    }
}
`

type cenarioDaSonda struct {
	instalado  bool
	ativo      bool
	configFala bool
	config     string
	logPath    string
}

func rodarSonda(t *testing.T, c cenarioDaSonda) sondaNginx {
	t.Helper()

	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash indisponível")
	}

	dir := t.TempDir()
	escrever := func(nome, corpo string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, nome), []byte(corpo), 0o755); err != nil {
			t.Fatalf("criar %s falso: %v", nome, err)
		}
	}

	if c.instalado {
		corpo := "#!/bin/sh\nexit 0\n"
		switch {
		case !c.configFala:
			corpo = "#!/bin/sh\n>&2 echo 'nginx: [emerg] open() /etc/nginx/nginx.conf failed (13: Permission denied)'\nexit 1\n"
		case c.config != "":
			conf := filepath.Join(dir, "nginx.conf.teste")
			if err := os.WriteFile(conf, []byte(c.config), 0o644); err != nil {
				t.Fatalf("criar configuração de teste: %v", err)
			}
			corpo = "#!/bin/sh\ncat " + conf + "\n"
		}
		escrever("nginx", corpo)
	}

	estado := "1"
	if c.ativo {
		estado = "0"
	}
	escrever("systemctl", "#!/bin/sh\nexit "+estado+"\n")
	escrever("service", "#!/bin/sh\nexit "+estado+"\n")
	escrever("pgrep", "#!/bin/sh\nexit "+estado+"\n")

	log := c.logPath
	if log == "" {
		log = filepath.Join(dir, "ausente.log")
	}

	cmd := exec.Command(bash, "-s")
	cmd.Env = append(os.Environ(),
		"PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"DOCKKEEPER_NGINX_LOG="+log,
	)
	cmd.Stdin = strings.NewReader(ProbeNginx)
	saida, err := cmd.Output()
	if err != nil {
		t.Fatalf("sonda falhou: %v", err)
	}

	var payload sondaNginx
	if err := json.Unmarshal(saida, &payload); err != nil {
		t.Fatalf("JSON inválido da sonda (%q): %v", string(saida), err)
	}
	return payload
}

func TestSondaSemNginxInstalado(t *testing.T) {
	p := rodarSonda(t, cenarioDaSonda{})

	if p.Instalado || p.Ativo {
		t.Errorf("instalado=%v ativo=%v, esperado falso nos dois", p.Instalado, p.Ativo)
	}
	if len(p.Upstreams) != 0 {
		t.Errorf("upstreams = %+v, esperado nenhum", p.Upstreams)
	}
}

func TestSondaComNginxParado(t *testing.T) {
	p := rodarSonda(t, cenarioDaSonda{instalado: true, configFala: true, config: configComUpstream})

	if !p.Instalado {
		t.Error("instalado = false, esperado true com o binário no PATH")
	}
	if p.Ativo {
		t.Error("ativo = true, esperado false quando systemctl e pgrep recusam")
	}
	if p.ConfigLida {
		t.Error("config_lida = true, esperado false: não se lê configuração de serviço parado")
	}
}

func TestSondaComProxyReversoListaUpstreams(t *testing.T) {
	logLegivel := filepath.Join(t.TempDir(), "access.log")
	if err := os.WriteFile(logLegivel, []byte("linha\n"), 0o644); err != nil {
		t.Fatalf("criar log de teste: %v", err)
	}

	p := rodarSonda(t, cenarioDaSonda{
		instalado: true, ativo: true, configFala: true,
		config: configComUpstream, logPath: logLegivel,
	})

	if !p.Ativo || !p.ConfigLida || !p.LogLegivel {
		t.Fatalf("ativo=%v config_lida=%v log_legivel=%v, esperado verdadeiro nos três",
			p.Ativo, p.ConfigLida, p.LogLegivel)
	}
	if len(p.Upstreams) != 2 {
		t.Fatalf("upstreams = %+v, esperado 2 blocos", p.Upstreams)
	}
	if p.Upstreams[0].Bloco != "api" || len(p.Upstreams[0].Destinos) != 2 {
		t.Errorf("primeiro bloco = %+v, esperado api com 2 destinos", p.Upstreams[0])
	}
	if p.Upstreams[0].Destinos[0] != "203.0.113.10:8080" {
		t.Errorf("destino = %q, esperado 203.0.113.10:8080", p.Upstreams[0].Destinos[0])
	}
	if p.Upstreams[1].Bloco != "painel" || p.Upstreams[1].Destinos[0] != "198.51.100.5:3000" {
		t.Errorf("segundo bloco = %+v, esperado painel com 198.51.100.5:3000", p.Upstreams[1])
	}
}

func TestSondaComSiteSemUpstream(t *testing.T) {
	p := rodarSonda(t, cenarioDaSonda{
		instalado: true, ativo: true, configFala: true, config: configSemUpstream,
	})

	if !p.ConfigLida {
		t.Fatal("config_lida = false, esperado true")
	}
	if len(p.Upstreams) != 0 {
		t.Errorf("upstreams = %+v, esperado nenhum: é site, não proxy", p.Upstreams)
	}
}

func TestSondaSemPermissaoDeLerConfiguracao(t *testing.T) {
	p := rodarSonda(t, cenarioDaSonda{instalado: true, ativo: true})

	if p.ConfigLida {
		t.Error("config_lida = true, esperado false quando nginx -T falha")
	}
	if !strings.Contains(p.ConfigMotivo, "Permission denied") {
		t.Errorf("config_motivo = %q, esperado carregar o erro do nginx", p.ConfigMotivo)
	}
}

func TestSondaComLogIlegivelExplicaOMotivo(t *testing.T) {
	semPermissao := filepath.Join(t.TempDir(), "access.log")
	if err := os.WriteFile(semPermissao, []byte("linha\n"), 0o000); err != nil {
		t.Fatalf("criar log sem permissão: %v", err)
	}
	if os.Geteuid() == 0 {
		t.Skip("rodando como root: permissão de arquivo não restringe leitura")
	}

	p := rodarSonda(t, cenarioDaSonda{
		instalado: true, ativo: true, configFala: true,
		config: configComUpstream, logPath: semPermissao,
	})

	if p.LogLegivel {
		t.Fatal("log_legivel = true, esperado false")
	}
	if p.LogMotivo != "arquivo existe e nao e legivel" {
		t.Errorf("log_motivo = %q, esperado distinguir de arquivo inexistente", p.LogMotivo)
	}
	if p.LogCaminho != semPermissao {
		t.Errorf("log_caminho = %q, esperado %q", p.LogCaminho, semPermissao)
	}
}

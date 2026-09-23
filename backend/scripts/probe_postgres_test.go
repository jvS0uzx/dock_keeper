package scripts

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type basePostgres struct {
	Nome         string `json:"nome"`
	Dono         string `json:"dono"`
	Encoding     string `json:"encoding"`
	TamanhoBytes *int64 `json:"tamanho_bytes"`
	Conexoes     *int   `json:"conexoes"`
}

type instanciaPostgres struct {
	Porta         int            `json:"porta"`
	EmContainer   bool           `json:"em_container"`
	ContainerNome string         `json:"container_nome"`
	Versao        string         `json:"versao"`
	Papel         string         `json:"papel"`
	WalLevel      string         `json:"wal_level"`
	MaxWalSenders *int           `json:"max_wal_senders"`
	ArchiveMode   string         `json:"archive_mode"`
	Estado        string         `json:"estado"`
	Motivo        string         `json:"motivo"`
	Bases         []basePostgres `json:"bases"`
}

type sondaPostgres struct {
	DescobertaHostOk      bool                `json:"descoberta_host_ok"`
	DescobertaContainerOk bool                `json:"descoberta_container_ok"`
	Instancias            []instanciaPostgres `json:"instancias"`
}

type cenarioPostgres struct {
	ss          string
	ssFalha     bool
	ssCmd       string
	netstat     string
	dockerPS    string
	dockerFalha bool
	psql        string
	psqlCmd     string
	uid         string
	sudoFalha   bool
}

const ssComPostgres = `Netid State  Recv-Q Send-Q  Local Address:Port  Peer Address:Port Process
tcp   LISTEN 0      244         127.0.0.1:5432       0.0.0.0:*    users:(("postgres",pid=900,fd=7))`

const ssSemDonoDoProcesso = `Netid State  Recv-Q Send-Q  Local Address:Port  Peer Address:Port Process
tcp   LISTEN 0      244         127.0.0.1:5432       0.0.0.0:*
tcp   LISTEN 0      128           0.0.0.0:22          0.0.0.0:*`

const ssSemPostgres = `Netid State  Recv-Q Send-Q  Local Address:Port  Peer Address:Port Process
tcp   LISTEN 0      128           0.0.0.0:22          0.0.0.0:*    users:(("sshd",pid=700,fd=3))`

const ssComDonoAlheioEscondido = `Netid State  Recv-Q Send-Q  Local Address:Port  Peer Address:Port Process
tcp   LISTEN 0      4096        127.0.0.1:33245      0.0.0.0:*    users:(("code",pid=4210,fd=25))
tcp   LISTEN 0      4096        127.0.0.1:44317      0.0.0.0:*    users:(("code",pid=4210,fd=31))
tcp   LISTEN 0      200         127.0.0.1:5432       0.0.0.0:*`

const ssComUdpDoPostgres = `Netid State  Recv-Q Send-Q  Local Address:Port  Peer Address:Port Process
udp   UNCONN 0      0             0.0.0.0:5433        0.0.0.0:*    users:(("postgres",pid=901,fd=9))
tcp   LISTEN 0      200         127.0.0.1:5432       0.0.0.0:*    users:(("postgres",pid=900,fd=7))
tcp   LISTEN 0      200             [::1]:5432           [::]:*    users:(("postgres",pid=900,fd=8))`

const psqlSaudavel = `case "$sql" in
  *server_version*) echo "16.2" ;;
  *pg_is_in_recovery*) echo "f" ;;
  *max_wal_senders*) echo "10" ;;
  *wal_level*) echo "replica" ;;
  *archive_mode*) echo "off" ;;
  *"pg_database_size(d.datname)"*) printf 'app|postgres|UTF8|1024|3\ndockkeeper|dono_app|LATIN1|2048|0\n' ;;
  *) exit 1 ;;
esac
`

func rodarSondaPostgres(t *testing.T, c cenarioPostgres) sondaPostgres {
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

	logica := c.psql
	if logica == "" {
		logica = "exit 1\n"
	}
	escrever("pgfake", "#!/bin/sh\nsql=\"\"\nanterior=\"\"\nfor a in \"$@\"; do\n"+
		"  if [ \"$anterior\" = \"-c\" ]; then sql=\"$a\"; fi\n  anterior=\"$a\"\ndone\n"+logica)

	pgfake := filepath.Join(dir, "pgfake")
	escrever("psql", "#!/bin/sh\nexec "+pgfake+" \"$@\"\n")

	uid := c.uid
	if uid == "" {
		uid = "1000"
	}
	escrever("id", "#!/bin/sh\nif [ \"$1\" = \"-u\" ]; then echo "+uid+"; else echo teste; fi\n")

	if c.sudoFalha {
		escrever("sudo", "#!/bin/sh\n>&2 echo 'sudo: a password is required'\nexit 1\n")
	} else {
		escrever("sudo", "#!/bin/sh\nwhile [ \"$1\" = \"-n\" ]; do shift; done\nexec \"$@\"\n")
	}

	if c.ssFalha {
		escrever("ss", "#!/bin/sh\n>&2 echo 'ss: comando indisponível'\nexit 1\n")
	} else {
		escrever("ss", "#!/bin/sh\ncat <<'FIM'\n"+c.ss+"\nFIM\n")
	}
	if c.netstat == "" {
		escrever("netstat", "#!/bin/sh\nexit 1\n")
	} else {
		escrever("netstat", "#!/bin/sh\ncat <<'FIM'\n"+c.netstat+"\nFIM\n")
	}

	respostaDoPS := "cat <<'FIM'\n" + c.dockerPS + "\nFIM\nexit 0\n"
	if c.dockerFalha {
		respostaDoPS = ">&2 echo 'permission denied while trying to connect to the Docker daemon socket'\nexit 1\n"
	}
	escrever("docker", "#!/bin/sh\nif [ \"$1\" = \"ps\" ]; then\n"+respostaDoPS+"fi\n"+
		"if [ \"$1\" = \"exec\" ]; then\nshift\nshift\nexec "+pgfake+" \"$@\"\nfi\nexit 0\n")

	ssCmd := c.ssCmd
	if ssCmd == "sudo" {
		ssCmd = "sudo -n " + filepath.Join(dir, "ss")
	}

	cmd := exec.Command(bash, "-s")
	cmd.Env = append(os.Environ(),
		"PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"DOCKKEEPER_PSQL_CMD="+c.psqlCmd,
		"DOCKKEEPER_SS_CMD="+ssCmd,
	)
	cmd.Stdin = strings.NewReader(ProbePostgres)
	saida, err := cmd.Output()
	if err != nil {
		t.Fatalf("sonda falhou: %v", err)
	}

	var payload sondaPostgres
	if err := json.Unmarshal(saida, &payload); err != nil {
		t.Fatalf("JSON inválido da sonda (%q): %v", string(saida), err)
	}
	return payload
}

func baseChamada(t *testing.T, inst instanciaPostgres, nome string) basePostgres {
	t.Helper()
	for _, b := range inst.Bases {
		if b.Nome == nome {
			return b
		}
	}
	t.Fatalf("base %q ausente em %+v", nome, inst.Bases)
	return basePostgres{}
}

func TestSondaPostgresNoHostListaBasesEConfiguracao(t *testing.T) {
	p := rodarSondaPostgres(t, cenarioPostgres{ss: ssComPostgres, psql: psqlSaudavel})

	if len(p.Instancias) != 1 {
		t.Fatalf("instâncias = %+v, esperado 1", p.Instancias)
	}
	inst := p.Instancias[0]
	if inst.Porta != 5432 || inst.EmContainer || inst.ContainerNome != "" {
		t.Errorf("instância = %+v, esperada no host na porta 5432", inst)
	}
	if inst.Estado != "ativo" || inst.Motivo != "" {
		t.Errorf("estado = %q motivo = %q, esperado ativo sem motivo", inst.Estado, inst.Motivo)
	}
	if inst.Papel != "primario" {
		t.Errorf("papel = %q, esperado primario: pg_is_in_recovery devolveu f", inst.Papel)
	}
	if inst.Versao != "16.2" || inst.WalLevel != "replica" || inst.ArchiveMode != "off" {
		t.Errorf("versao=%q wal_level=%q archive_mode=%q", inst.Versao, inst.WalLevel, inst.ArchiveMode)
	}
	if inst.MaxWalSenders == nil || *inst.MaxWalSenders != 10 {
		t.Errorf("max_wal_senders = %v, esperado 10", inst.MaxWalSenders)
	}
	if len(inst.Bases) != 2 {
		t.Fatalf("bases = %+v, esperado 2", inst.Bases)
	}

	app := baseChamada(t, inst, "app")
	if app.TamanhoBytes == nil || *app.TamanhoBytes != 1024 {
		t.Errorf("tamanho de app = %v, esperado 1024", app.TamanhoBytes)
	}
	if app.Conexoes == nil || *app.Conexoes != 3 {
		t.Errorf("conexões de app = %v, esperado 3", app.Conexoes)
	}

	dk := baseChamada(t, inst, "dockkeeper")
	if dk.Dono != "dono_app" || dk.Encoding != "LATIN1" {
		t.Errorf("dono/encoding de dockkeeper = %q/%q", dk.Dono, dk.Encoding)
	}
	if dk.Conexoes == nil || *dk.Conexoes != 0 {
		t.Errorf("conexões de dockkeeper = %v, esperado 0 medido, não chave ausente", dk.Conexoes)
	}
}

func TestSondaPostgresEmContainerUsaAPortaPublicada(t *testing.T) {
	p := rodarSondaPostgres(t, cenarioPostgres{
		dockerPS: "pg-app|postgres:16-alpine|0.0.0.0:5433->5432/tcp, :::5433->5432/tcp\n" +
			"redis-app|redis:7|0.0.0.0:6379->6379/tcp",
		psql: psqlSaudavel,
	})

	if len(p.Instancias) != 1 {
		t.Fatalf("instâncias = %+v, esperado só o container de postgres", p.Instancias)
	}
	inst := p.Instancias[0]
	if !inst.EmContainer || inst.ContainerNome != "pg-app" {
		t.Errorf("instância = %+v, esperada em container pg-app", inst)
	}
	if inst.Porta != 5433 {
		t.Errorf("porta = %d, esperada a publicada no host (5433)", inst.Porta)
	}
	if inst.Estado != "ativo" || len(inst.Bases) != 2 {
		t.Errorf("estado = %q com %d bases, esperado ativo com 2", inst.Estado, len(inst.Bases))
	}
}

func TestSondaPostgresSemContainerComPortaPublicadaCaiNaInterna(t *testing.T) {
	p := rodarSondaPostgres(t, cenarioPostgres{
		dockerPS: "pg-interno|postgres:16|5432/tcp",
		psql:     psqlSaudavel,
	})

	if len(p.Instancias) != 1 || p.Instancias[0].Porta != 5432 {
		t.Fatalf("instâncias = %+v, esperada uma na porta interna 5432", p.Instancias)
	}
}

func TestSondaPostgresSemPermissaoNaoDizInativo(t *testing.T) {
	recusa := `case "$sql" in
  *) >&2 echo 'psql: error: connection to server failed: FATAL:  password authentication failed for user "monitor"'; exit 2 ;;
esac
`
	p := rodarSondaPostgres(t, cenarioPostgres{ss: ssComPostgres, psql: recusa})

	if len(p.Instancias) != 1 {
		t.Fatalf("instâncias = %+v, esperado 1: falta de credencial não apaga a instância", p.Instancias)
	}
	inst := p.Instancias[0]
	if inst.Estado != "sem_acesso" {
		t.Fatalf("estado = %q, esperado sem_acesso: o que faltou foi permissão, não processo", inst.Estado)
	}
	if inst.Motivo == "" || !strings.Contains(inst.Motivo, "authentication") {
		t.Errorf("motivo = %q, esperado carregar o erro do psql", inst.Motivo)
	}
	if inst.Papel != "desconhecido" {
		t.Errorf("papel = %q, esperado desconhecido: sonda cega não inventa papel", inst.Papel)
	}
	if len(inst.Bases) != 0 {
		t.Errorf("bases = %+v, esperado nenhuma", inst.Bases)
	}
}

func TestSondaPostgresComPortaMortaReportaInativo(t *testing.T) {
	recusa := `case "$sql" in
  *) >&2 echo 'psql: error: connection to server at 127.0.0.1 port 5432 failed: Connection refused'; exit 2 ;;
esac
`
	p := rodarSondaPostgres(t, cenarioPostgres{ss: ssComPostgres, psql: recusa})

	if len(p.Instancias) != 1 || p.Instancias[0].Estado != "inativo" {
		t.Fatalf("instâncias = %+v, esperado estado inativo", p.Instancias)
	}
}

func TestSondaPostgresOmiteTamanhoDaBaseSemPermissao(t *testing.T) {
	parcial := `case "$sql" in
  *server_version*) echo "16.2" ;;
  *pg_is_in_recovery*) echo "t" ;;
  *max_wal_senders*) echo "" ;;
  *wal_level*) echo "replica" ;;
  *archive_mode*) echo "on" ;;
  *"pg_database_size(d.datname)"*) >&2 echo 'ERROR:  permission denied for database restrito'; exit 1 ;;
  *"pg_database_size('restrito')"*) >&2 echo 'ERROR:  permission denied for database restrito'; exit 1 ;;
  *"pg_database_size("*) echo "4096" ;;
  *"FROM pg_database d"*) printf 'app|postgres|UTF8|2\nrestrito|outro|UTF8|0\nzz|postgres|UTF8|1\n' ;;
  *) exit 1 ;;
esac
`
	p := rodarSondaPostgres(t, cenarioPostgres{ss: ssComPostgres, psql: parcial})

	if len(p.Instancias) != 1 {
		t.Fatalf("instâncias = %+v, esperado 1", p.Instancias)
	}
	inst := p.Instancias[0]
	if inst.Estado != "ativo" {
		t.Fatalf("estado = %q, esperado ativo: a instância respondeu", inst.Estado)
	}
	if inst.Papel != "replica" {
		t.Errorf("papel = %q, esperado replica", inst.Papel)
	}
	if inst.MaxWalSenders != nil {
		t.Errorf("max_wal_senders = %v, esperada a chave ausente quando não medido", *inst.MaxWalSenders)
	}
	if len(inst.Bases) != 3 {
		t.Fatalf("bases = %+v, esperado 3: uma base sem tamanho não derruba as outras", inst.Bases)
	}
	if b := baseChamada(t, inst, "restrito"); b.TamanhoBytes != nil {
		t.Errorf("tamanho de restrito = %v, esperado ausente, nunca 0", *b.TamanhoBytes)
	}
	for _, nome := range []string{"app", "zz"} {
		b := baseChamada(t, inst, nome)
		if b.TamanhoBytes == nil || *b.TamanhoBytes != 4096 {
			t.Errorf("tamanho de %s = %v, esperado 4096", nome, b.TamanhoBytes)
		}
	}
	if b := baseChamada(t, inst, "restrito"); b.Conexoes == nil || *b.Conexoes != 0 {
		t.Errorf("conexões de restrito = %v, esperado 0", b.Conexoes)
	}
}

func TestSondaPostgresUsaOComandoInjetadoPeloPreludio(t *testing.T) {
	p := rodarSondaPostgres(t, cenarioPostgres{
		ss:      ssComPostgres,
		psql:    psqlSaudavel,
		psqlCmd: "psql -U postgres",
	})

	if len(p.Instancias) != 1 || p.Instancias[0].Estado != "ativo" {
		t.Fatalf("instâncias = %+v, esperado ativo com DOCKKEEPER_PSQL_CMD com argumentos", p.Instancias)
	}
}

func TestSondaPostgresSemClientePsqlNaoDizInativo(t *testing.T) {
	p := rodarSondaPostgres(t, cenarioPostgres{
		ss:      ssComPostgres,
		psql:    psqlSaudavel,
		psqlCmd: "/opt/inexistente/psql -U postgres",
	})

	if len(p.Instancias) != 1 {
		t.Fatalf("instâncias = %+v, esperado 1: a porta foi vista", p.Instancias)
	}
	inst := p.Instancias[0]
	if inst.Estado != "desconhecido" {
		t.Errorf("estado = %q, esperado desconhecido: sem cliente não se sabe nada da instância", inst.Estado)
	}
	if !strings.Contains(inst.Motivo, "psql") {
		t.Errorf("motivo = %q, esperado citar o cliente ausente", inst.Motivo)
	}
}

func TestSondaPostgresSemInstanciaDeclaraDescobertaFeita(t *testing.T) {
	p := rodarSondaPostgres(t, cenarioPostgres{ss: ssSemPostgres, uid: "0"})

	if len(p.Instancias) != 0 {
		t.Errorf("instâncias = %+v, esperado nenhuma", p.Instancias)
	}
	if !p.DescobertaHostOk || !p.DescobertaContainerOk {
		t.Errorf("host_ok=%v container_ok=%v, esperado true nos dois: as duas procuras rodaram com privilégio e não acharam PostgreSQL",
			p.DescobertaHostOk, p.DescobertaContainerOk)
	}
}

func TestSondaPostgresSemPrivilegioNaoDeclaraDescobertaDeHost(t *testing.T) {
	p := rodarSondaPostgres(t, cenarioPostgres{
		ss:          ssComDonoAlheioEscondido,
		dockerFalha: true,
		psql:        psqlSaudavel,
		uid:         "1000",
	})

	if p.DescobertaHostOk {
		t.Error("descoberta_host_ok = true, esperado false: ver o dono dos próprios sockets não é ver o dono de qualquer socket; " +
			"o PostgreSQL do host roda sob outro usuário e some da listagem")
	}
	if len(p.Instancias) != 0 {
		t.Errorf("instâncias = %+v, esperado nenhuma: a linha da 5432 não tem dono para casar", p.Instancias)
	}

	comPrivilegio := rodarSondaPostgres(t, cenarioPostgres{
		ss:          ssComDonoAlheioEscondido,
		dockerFalha: true,
		psql:        psqlSaudavel,
		uid:         "0",
	})
	if !comPrivilegio.DescobertaHostOk {
		t.Error("mesma listagem com uid 0 deu descoberta_host_ok = false; " +
			"o que decide é o privilégio, não a presença de alguma coluna de dono")
	}
}

func TestSondaPostgresComoRootDeclaraDescobertaDeHost(t *testing.T) {
	p := rodarSondaPostgres(t, cenarioPostgres{
		ss:          ssComPostgres,
		dockerFalha: true,
		psql:        psqlSaudavel,
		uid:         "0",
	})

	if !p.DescobertaHostOk {
		t.Error("descoberta_host_ok = false, esperado true: uid 0 enxerga o dono de qualquer socket")
	}
	if len(p.Instancias) != 1 || p.Instancias[0].EmContainer {
		t.Fatalf("instâncias = %+v, esperada a do host", p.Instancias)
	}
}

func TestSondaPostgresComSudoQueFuncionaDeclaraDescobertaDeHost(t *testing.T) {
	p := rodarSondaPostgres(t, cenarioPostgres{
		ss:          ssComPostgres,
		ssCmd:       "sudo",
		dockerFalha: true,
		psql:        psqlSaudavel,
		uid:         "1000",
	})

	if !p.DescobertaHostOk {
		t.Error("descoberta_host_ok = false, esperado true: a listagem rodou sob sudo que passou")
	}
	if len(p.Instancias) != 1 || p.Instancias[0].Porta != 5432 {
		t.Fatalf("instâncias = %+v, esperada a do host na 5432", p.Instancias)
	}
}

func TestSondaPostgresComSudoQueExigeSenhaNaoDeclaraDescobertaDeHost(t *testing.T) {
	p := rodarSondaPostgres(t, cenarioPostgres{
		ss:          ssComPostgres,
		ssCmd:       "sudo",
		sudoFalha:   true,
		dockerFalha: true,
		psql:        psqlSaudavel,
		uid:         "1000",
	})

	if p.DescobertaHostOk {
		t.Error("descoberta_host_ok = true, esperado false: sudo -n que pede senha não é privilégio")
	}
	if len(p.Instancias) != 0 {
		t.Errorf("instâncias = %+v, esperado nenhuma: a listagem nem chegou a sair", p.Instancias)
	}
}

func TestSondaPostgresIgnoraSocketUdpDoPostgres(t *testing.T) {
	p := rodarSondaPostgres(t, cenarioPostgres{
		ss:          ssComUdpDoPostgres,
		dockerFalha: true,
		psql:        psqlSaudavel,
		uid:         "0",
	})

	if len(p.Instancias) != 1 {
		t.Fatalf("instâncias = %+v, esperada só uma: socket UDP não é PostgreSQL escutando, "+
			"e IPv4 e IPv6 da mesma porta são a mesma instância", p.Instancias)
	}
	if p.Instancias[0].Porta != 5432 {
		t.Errorf("porta = %d, esperada 5432; a 5433 é UNCONN e viraria instância fantasma no painel",
			p.Instancias[0].Porta)
	}
}

func TestSondaPostgresSemMecanismoDeDescobertaNaoDeclaraOk(t *testing.T) {
	p := rodarSondaPostgres(t, cenarioPostgres{ss: ssSemDonoDoProcesso, dockerFalha: true})

	if p.DescobertaHostOk {
		t.Error("descoberta_host_ok = true, esperado false: sem privilégio não se afirma ausência de PostgreSQL no host")
	}
	if p.DescobertaContainerOk {
		t.Error("descoberta_container_ok = true, esperado false: o docker ps saiu com erro")
	}
	if len(p.Instancias) != 0 {
		t.Errorf("instâncias = %+v, esperado nenhuma", p.Instancias)
	}
}

func TestSondaPostgresDeclaraCegueiraSoDoLadoQueFalhou(t *testing.T) {
	p := rodarSondaPostgres(t, cenarioPostgres{
		ss:       ssSemDonoDoProcesso,
		dockerPS: "pg-app|postgres:16|0.0.0.0:5433->5432/tcp",
		psql:     psqlSaudavel,
	})

	if p.DescobertaHostOk {
		t.Error("descoberta_host_ok = true, esperado false: a listagem rodou sem privilégio")
	}
	if !p.DescobertaContainerOk {
		t.Error("descoberta_container_ok = false, esperado true: o docker respondeu")
	}
	if len(p.Instancias) != 1 || !p.Instancias[0].EmContainer {
		t.Fatalf("instâncias = %+v, esperada a do container", p.Instancias)
	}
}

func TestSondaPostgresCaiNoNetstatQuandoOSsFalha(t *testing.T) {
	p := rodarSondaPostgres(t, cenarioPostgres{
		ssFalha: true,
		netstat: "tcp        0      0 127.0.0.1:5434          0.0.0.0:*               LISTEN      900/postgres\n" +
			"udp        0      0 0.0.0.0:5435            0.0.0.0:*                           901/postgres",
		dockerFalha: true,
		psql:        psqlSaudavel,
		uid:         "0",
	})

	if !p.DescobertaHostOk {
		t.Error("descoberta_host_ok = false, esperado true: o netstat rodou com privilégio")
	}
	if len(p.Instancias) != 1 || p.Instancias[0].Porta != 5434 {
		t.Fatalf("instâncias = %+v, esperada uma na 5434: a 5435 é UDP e o netstat também a lista com -tulnp",
			p.Instancias)
	}
}

func TestSondaPostgresNaoDuplicaContainerComRedeDoHost(t *testing.T) {
	p := rodarSondaPostgres(t, cenarioPostgres{
		ss:       ssComPostgres,
		dockerPS: "pg-host|postgres:16|",
		psql:     psqlSaudavel,
	})

	if len(p.Instancias) != 1 {
		t.Fatalf("instâncias = %+v, esperada uma só: mesma porta vista duas vezes", p.Instancias)
	}
	if !p.Instancias[0].EmContainer {
		t.Errorf("instância = %+v, esperada a leitura do container, mais informativa", p.Instancias[0])
	}
}

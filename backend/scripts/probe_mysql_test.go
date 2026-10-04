package scripts

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type baseMySQL struct {
	Nome         string `json:"nome"`
	Dono         string `json:"dono"`
	Encoding     string `json:"encoding"`
	TamanhoBytes *int64 `json:"tamanho_bytes"`
	Conexoes     *int   `json:"conexoes"`
}

type instanciaMySQL struct {
	Porta         int         `json:"porta"`
	EmContainer   bool        `json:"em_container"`
	ContainerNome string      `json:"container_nome"`
	Motor         string      `json:"motor"`
	Versao        string      `json:"versao"`
	Papel         string      `json:"papel"`
	WalLevel      string      `json:"wal_level"`
	MaxWalSenders *int        `json:"max_wal_senders"`
	ArchiveMode   string      `json:"archive_mode"`
	Estado        string      `json:"estado"`
	Motivo        string      `json:"motivo"`
	Bases         []baseMySQL `json:"bases"`
}

type sondaMySQL struct {
	DescobertaHostOk      bool             `json:"descoberta_host_ok"`
	DescobertaContainerOk bool             `json:"descoberta_container_ok"`
	Instancias            []instanciaMySQL `json:"instancias"`
}

type cenarioMySQL struct {
	ss          string
	ssFalha     bool
	ssCmd       string
	dockerPS    string
	dockerFalha bool
	mysql       string
	semCliente  bool
	soMariaDB   bool
	dockerExec  string
	mysqlCmd    string
	uid         string
	env         []string
}

const ssComMySQL = `Netid State  Recv-Q Send-Q  Local Address:Port  Peer Address:Port Process
tcp   LISTEN 0      151         127.0.0.1:3306       0.0.0.0:*    users:(("mysqld",pid=800,fd=21))
tcp   LISTEN 0      70          127.0.0.1:33060      0.0.0.0:*    users:(("mysqld",pid=800,fd=19))
tcp   LISTEN 0      128           0.0.0.0:22          0.0.0.0:*    users:(("sshd",pid=700,fd=3))`

const ssComMariaDBNaPorta3307 = `Netid State  Recv-Q Send-Q  Local Address:Port  Peer Address:Port Process
tcp   LISTEN 0      80          127.0.0.1:3307       0.0.0.0:*    users:(("mariadbd",pid=810,fd=20))`

const ssSemDonoDoMySQL = `Netid State  Recv-Q Send-Q  Local Address:Port  Peer Address:Port Process
tcp   LISTEN 0      151         127.0.0.1:3306       0.0.0.0:*`

const mysqlSaudavel = `case "$sql" in
  *"VERSION()"*) printf '8.0.36-0ubuntu0.22.04.1\tMySQL Community Server - GPL\n' ;;
  *"SHOW REPLICA STATUS"*) exit 0 ;;
  *"information_schema.processlist"*) printf 'app\tutf8mb4\t1048576\t3\nloja\tlatin1\t0\t0\n' ;;
  *) exit 1 ;;
esac
`

const mariadbSaudavel = `case "$sql" in
  *"VERSION()"*) printf '10.11.6-MariaDB-0+deb12u1\tDebian 12\n' ;;
  *"SHOW REPLICA STATUS"*) exit 0 ;;
  *"information_schema.processlist"*) printf 'loja\tutf8mb4\t2048\t1\n' ;;
  *) exit 1 ;;
esac
`

var ferramentasDoSistema = []string{"sh", "awk", "tr", "cut", "head", "grep", "sort", "cat"}

func pathHermetico(t *testing.T, stubs string) string {
	t.Helper()

	sistema := t.TempDir()
	for _, nome := range ferramentasDoSistema {
		real, err := exec.LookPath(nome)
		if err != nil {
			t.Skipf("%s indisponível no sistema", nome)
		}
		if err := os.Symlink(real, filepath.Join(sistema, nome)); err != nil {
			t.Fatalf("ligar %s: %v", nome, err)
		}
	}
	return stubs + string(os.PathListSeparator) + sistema
}

func rodarSondaMySQL(t *testing.T, c cenarioMySQL) (sondaMySQL, string) {
	t.Helper()

	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash indisponível")
	}

	dir := t.TempDir()
	registro := filepath.Join(t.TempDir(), "chamadas.log")
	escrever := func(nome, corpo string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, nome), []byte(corpo), 0o755); err != nil {
			t.Fatalf("criar %s falso: %v", nome, err)
		}
	}

	logica := c.mysql
	if logica == "" {
		logica = "exit 1\n"
	}
	escrever("myfake", "#!/bin/sh\nsql=\"\"\nanterior=\"\"\n"+
		"printf 'MYSQL %s\\n' \"$*\" >> \""+registro+"\"\n"+
		"printf 'PWD %s\\n' \"${MYSQL_PWD-<ausente>}\" >> \""+registro+"\"\n"+
		"for a in \"$@\"; do\n"+
		"  if [ \"$anterior\" = \"-e\" ]; then sql=\"$a\"; fi\n  anterior=\"$a\"\ndone\n"+logica)
	myfake := filepath.Join(dir, "myfake")
	switch {
	case c.soMariaDB:
		escrever("mariadb", "#!/bin/sh\nexec "+myfake+" \"$@\"\n")
	case !c.semCliente:
		escrever("mysql", "#!/bin/sh\nexec "+myfake+" \"$@\"\n")
	}

	uid := c.uid
	if uid == "" {
		uid = "1000"
	}
	escrever("id", "#!/bin/sh\nif [ \"$1\" = \"-u\" ]; then echo "+uid+"; else echo teste; fi\n")
	escrever("sudo", "#!/bin/sh\nwhile [ \"$1\" = \"-n\" ]; do shift; done\nexec \"$@\"\n")

	if c.ssFalha {
		escrever("ss", "#!/bin/sh\n>&2 echo 'ss: comando indisponível'\nexit 1\n")
	} else {
		escrever("ss", "#!/bin/sh\ncat <<'FIM'\n"+c.ss+"\nFIM\n")
	}
	escrever("netstat", "#!/bin/sh\nexit 1\n")

	respostaDoPS := "cat <<'FIM'\n" + c.dockerPS + "\nFIM\nexit 0\n"
	if c.dockerFalha {
		respostaDoPS = ">&2 echo 'permission denied while trying to connect to the Docker daemon socket'\nexit 1\n"
	}
	respostaDoExec := "shift\nshift\nexec \"$@\"\n"
	if c.dockerExec != "" {
		respostaDoExec = c.dockerExec
	}
	escrever("docker", "#!/bin/sh\nprintf 'DOCKER %s\\n' \"$*\" >> \""+registro+"\"\n"+
		"if [ \"$1\" = \"ps\" ]; then\n"+respostaDoPS+"fi\n"+
		"if [ \"$1\" = \"exec\" ]; then\n"+respostaDoExec+"fi\nexit 0\n")

	ssCmd := c.ssCmd
	if ssCmd == "sudo" {
		ssCmd = "sudo -n " + filepath.Join(dir, "ss")
	}

	cmd := exec.Command(bash, "-s")
	cmd.Env = append([]string{
		"PATH=" + pathHermetico(t, dir),
		"HOME=" + t.TempDir(),
		"DOCKKEEPER_MYSQL_CMD=" + c.mysqlCmd,
		"DOCKKEEPER_SS_CMD=" + ssCmd,
	}, c.env...)
	cmd.Stdin = strings.NewReader(ProbeMySQL)
	saida, err := cmd.Output()
	if err != nil {
		t.Fatalf("sonda falhou: %v", err)
	}

	var payload sondaMySQL
	if err := json.Unmarshal(saida, &payload); err != nil {
		t.Fatalf("JSON inválido da sonda (%q): %v", string(saida), err)
	}
	if strings.Count(strings.TrimSpace(string(saida)), "\n") != 0 {
		t.Errorf("a sonda emitiu mais de uma linha: %q", string(saida))
	}

	chamadas, _ := os.ReadFile(registro)
	return payload, string(chamadas)
}

func baseMySQLChamada(t *testing.T, inst instanciaMySQL, nome string) baseMySQL {
	t.Helper()
	for _, b := range inst.Bases {
		if b.Nome == nome {
			return b
		}
	}
	t.Fatalf("base %q ausente em %+v", nome, inst.Bases)
	return baseMySQL{}
}

func TestSondaMySQLNoHostListaBasesSemWAL(t *testing.T) {
	p, chamadas := rodarSondaMySQL(t, cenarioMySQL{ss: ssComMySQL, ssCmd: "sudo", mysql: mysqlSaudavel})

	if !p.DescobertaHostOk {
		t.Error("descoberta_host_ok = false com ss via sudo")
	}
	if len(p.Instancias) != 1 {
		t.Fatalf("instâncias = %+v, esperado só a 3306 (a 33060 é o X Protocol)", p.Instancias)
	}
	inst := p.Instancias[0]
	if inst.Porta != 3306 || inst.EmContainer || inst.Motor != "mysql" {
		t.Errorf("instância = %+v, esperada mysql no host na 3306", inst)
	}
	if inst.Versao != "8.0.36" || inst.Estado != "ativo" || inst.Motivo != "" || inst.Papel != "primario" {
		t.Errorf("versao=%q estado=%q motivo=%q papel=%q", inst.Versao, inst.Estado, inst.Motivo, inst.Papel)
	}
	if inst.WalLevel != "" || inst.MaxWalSenders != nil || inst.ArchiveMode != "" {
		t.Errorf("WAL preenchido em MySQL: %q %v %q", inst.WalLevel, inst.MaxWalSenders, inst.ArchiveMode)
	}

	app := baseMySQLChamada(t, inst, "app")
	if app.TamanhoBytes == nil || *app.TamanhoBytes != 1048576 || app.Conexoes == nil || *app.Conexoes != 3 {
		t.Errorf("app = %+v, esperado 1048576 bytes e 3 conexões", app)
	}
	if app.Encoding != "utf8mb4" || app.Dono != "" {
		t.Errorf("app encoding=%q dono=%q, esperado utf8mb4 e dono vazio", app.Encoding, app.Dono)
	}
	loja := baseMySQLChamada(t, inst, "loja")
	if loja.TamanhoBytes == nil || *loja.TamanhoBytes != 0 || loja.Conexoes == nil || *loja.Conexoes != 0 {
		t.Errorf("loja = %+v, esperado zero medido, não chave ausente", loja)
	}

	if !strings.Contains(chamadas, "--connect-timeout=5") {
		t.Errorf("o cliente rodou sem timeout de conexão: %s", chamadas)
	}
	if strings.Contains(chamadas, "-P 3306") || strings.Contains(chamadas, "--protocol=TCP") {
		t.Errorf("a 3306 no host deveria usar o socket local (root via unix_socket): %s", chamadas)
	}
}

func TestSondaMySQLReconheceMariaDBPelaVersao(t *testing.T) {
	p, _ := rodarSondaMySQL(t, cenarioMySQL{ss: ssComMySQL, ssCmd: "sudo", mysql: mariadbSaudavel})

	if len(p.Instancias) != 1 {
		t.Fatalf("instâncias = %+v, esperado 1", p.Instancias)
	}
	if inst := p.Instancias[0]; inst.Motor != "mariadb" || inst.Versao != "10.11.6" {
		t.Errorf("motor=%q versao=%q, esperado mariadb 10.11.6 mesmo com o processo chamado mysqld", inst.Motor, inst.Versao)
	}
}

func TestSondaMySQLPortaForaDoPadraoVaiPorTCP(t *testing.T) {
	p, chamadas := rodarSondaMySQL(t, cenarioMySQL{ss: ssComMariaDBNaPorta3307, ssCmd: "sudo", mysql: mariadbSaudavel})

	if len(p.Instancias) != 1 || p.Instancias[0].Porta != 3307 || p.Instancias[0].Motor != "mariadb" {
		t.Fatalf("instâncias = %+v, esperada mariadb na 3307", p.Instancias)
	}
	if !strings.Contains(chamadas, "--protocol=TCP -h 127.0.0.1 -P 3307") {
		t.Errorf("porta 3307 consultada sem TCP explícito: %s", chamadas)
	}
}

func TestSondaMySQLPapelDeReplica(t *testing.T) {
	casos := []struct {
		nome, replica, slave, papel string
	}{
		{"replica responde linha", "printf 'Waiting for source\\t10.0.0.1\\n'", "exit 1", "replica"},
		{"cai no slave antigo", "exit 1", "printf 'Waiting for master\\t10.0.0.1\\n'", "replica"},
		{"slave vazio é primario", "exit 1", "exit 0", "primario"},
		{"sem privilégio é desconhecido", "exit 1", "exit 1", "desconhecido"},
	}
	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			logica := `case "$sql" in
  *"VERSION()"*) printf '8.4.0\tMySQL Community Server - GPL\n' ;;
  *"SHOW REPLICA STATUS"*) ` + caso.replica + ` ;;
  *"SHOW SLAVE STATUS"*) ` + caso.slave + ` ;;
  *) exit 1 ;;
esac
`
			p, _ := rodarSondaMySQL(t, cenarioMySQL{ss: ssComMySQL, ssCmd: "sudo", mysql: logica})
			if len(p.Instancias) != 1 || p.Instancias[0].Papel != caso.papel {
				t.Fatalf("instâncias = %+v, esperado papel %s", p.Instancias, caso.papel)
			}
		})
	}
}

func TestSondaMySQLEmContainerPassaSenhaSoPeloAmbiente(t *testing.T) {
	p, chamadas := rodarSondaMySQL(t, cenarioMySQL{
		dockerPS: "loja-db|mariadb:11|0.0.0.0:13306->3306/tcp, :::13306->3306/tcp\n" +
			"cache|redis:7|0.0.0.0:6379->6379/tcp",
		mysql: mariadbSaudavel,
		env:   []string{"MARIADB_ROOT_PASSWORD=segredo-do-root"},
	})

	if !p.DescobertaContainerOk {
		t.Error("descoberta_container_ok = false com docker ps respondendo")
	}
	if len(p.Instancias) != 1 {
		t.Fatalf("instâncias = %+v, esperado só o container mariadb", p.Instancias)
	}
	inst := p.Instancias[0]
	if !inst.EmContainer || inst.ContainerNome != "loja-db" || inst.Porta != 13306 || inst.Motor != "mariadb" {
		t.Errorf("instância = %+v, esperada mariadb no container loja-db na 13306", inst)
	}
	if inst.Estado != "ativo" || len(inst.Bases) != 1 {
		t.Errorf("estado = %q com %d bases, esperado ativo com 1", inst.Estado, len(inst.Bases))
	}

	if !strings.Contains(chamadas, "PWD segredo-do-root") {
		t.Errorf("a senha do container não chegou por MYSQL_PWD: %s", chamadas)
	}
	for _, linha := range strings.Split(chamadas, "\n") {
		if (strings.HasPrefix(linha, "DOCKER ") || strings.HasPrefix(linha, "MYSQL ")) && strings.Contains(linha, "segredo-do-root") {
			t.Errorf("a senha apareceu numa linha de comando: %q", linha)
		}
	}
}

func TestSondaMySQLEmContainerSemSenhaTentaSemSenha(t *testing.T) {
	recusa := `case "$sql" in
  *) >&2 echo "ERROR 1045 (28000): Access denied for user 'root'@'localhost' (using password: NO)"; exit 1 ;;
esac
`
	p, chamadas := rodarSondaMySQL(t, cenarioMySQL{dockerPS: "app-mysql|mysql:8.4|3306/tcp, 33060/tcp", mysql: recusa})

	if len(p.Instancias) != 1 {
		t.Fatalf("instâncias = %+v, esperado 1", p.Instancias)
	}
	inst := p.Instancias[0]
	if inst.Porta != 3306 {
		t.Errorf("porta = %d, esperada a interna 3306 (não a 33060)", inst.Porta)
	}
	if inst.Estado != "sem_acesso" || !strings.Contains(inst.Motivo, "Access denied") {
		t.Errorf("estado=%q motivo=%q, esperado sem_acesso explicando a recusa", inst.Estado, inst.Motivo)
	}
	if strings.Contains(inst.Motivo, "'") {
		t.Errorf("motivo = %q, texto do banco precisa passar pela limpeza", inst.Motivo)
	}
	if inst.Motor != "mysql" || inst.Bases == nil {
		t.Errorf("motor = %q bases = %v, esperado o motor da imagem e lista vazia", inst.Motor, inst.Bases)
	}
	if !strings.Contains(chamadas, "PWD <ausente>") {
		t.Errorf("sem senha no container, MYSQL_PWD deveria ficar ausente: %s", chamadas)
	}
}

func TestSondaMySQLClassificaInstanciaParada(t *testing.T) {
	parada := `>&2 echo "ERROR 2002 (HY000): Can't connect to local MySQL server through socket '/run/mysqld/mysqld.sock' (2)"
exit 1
`
	p, _ := rodarSondaMySQL(t, cenarioMySQL{ss: ssComMySQL, ssCmd: "sudo", mysql: parada})

	if len(p.Instancias) != 1 || p.Instancias[0].Estado != "inativo" {
		t.Fatalf("instâncias = %+v, esperado inativo", p.Instancias)
	}
	if !strings.Contains(p.Instancias[0].Motivo, "não respondeu") {
		t.Errorf("motivo = %q, esperado em português", p.Instancias[0].Motivo)
	}
}

func TestSondaMySQLSemClienteNaoInventaEstado(t *testing.T) {
	p, _ := rodarSondaMySQL(t, cenarioMySQL{ss: ssComMySQL, ssCmd: "sudo", semCliente: true})

	if len(p.Instancias) != 1 {
		t.Fatalf("instâncias = %+v, esperado 1", p.Instancias)
	}
	inst := p.Instancias[0]
	if inst.Estado != "desconhecido" || !strings.Contains(inst.Motivo, "sem cliente mysql") {
		t.Errorf("estado=%q motivo=%q, esperado desconhecido por falta de cliente", inst.Estado, inst.Motivo)
	}
}

func TestSondaMySQLCegaDeclaraAsDuasDescobertas(t *testing.T) {
	p, _ := rodarSondaMySQL(t, cenarioMySQL{ssFalha: true, dockerFalha: true, mysql: mysqlSaudavel})

	if p.DescobertaHostOk || p.DescobertaContainerOk || len(p.Instancias) != 0 {
		t.Errorf("payload = %+v, esperado cego nas duas descobertas e sem instância", p)
	}
}

func TestSondaMySQLSemPrivilegioNaListagemNaoAutorizaPoda(t *testing.T) {
	p, _ := rodarSondaMySQL(t, cenarioMySQL{ss: ssSemDonoDoMySQL, mysql: mysqlSaudavel})

	if p.DescobertaHostOk {
		t.Error("descoberta_host_ok = true com ss sem privilégio, que esconde o dono do socket")
	}
	if len(p.Instancias) != 0 {
		t.Errorf("instâncias = %+v, esperado nenhuma sem o nome do processo", p.Instancias)
	}
}

func TestSondaMySQLLimpaTextoHostilDoBanco(t *testing.T) {
	hostil := `case "$sql" in
  *"VERSION()"*) printf '8.0.36"},{"x":"\tMySQL\n' ;;
  *"SHOW REPLICA STATUS"*) exit 0 ;;
  *"information_schema.processlist"*) printf 'ev"il\\\\base\tutf8"mb4\t10\t1\n' ;;
  *) exit 1 ;;
esac
`
	p, _ := rodarSondaMySQL(t, cenarioMySQL{ss: ssComMySQL, ssCmd: "sudo", mysql: hostil})

	if len(p.Instancias) != 1 || len(p.Instancias[0].Bases) != 1 {
		t.Fatalf("instâncias = %+v, esperado 1 com 1 base", p.Instancias)
	}
	inst := p.Instancias[0]
	if strings.ContainsAny(inst.Versao, `"{}`) || strings.ContainsAny(inst.Bases[0].Nome, `"\`) ||
		strings.ContainsAny(inst.Bases[0].Encoding, `"`) {
		t.Errorf("texto do banco passou sem limpeza: versao=%q base=%+v", inst.Versao, inst.Bases[0])
	}
}

func TestSondaMySQLSemProcesslistGuardaConexoesNulas(t *testing.T) {
	logica := `case "$sql" in
  *"VERSION()"*) printf '8.0.36\tMySQL\n' ;;
  *"SHOW REPLICA STATUS"*) exit 0 ;;
  *"information_schema.processlist"*) exit 1 ;;
  *"information_schema.schemata"*) printf 'app\tutf8mb4\t4096\n' ;;
  *) exit 1 ;;
esac
`
	p, _ := rodarSondaMySQL(t, cenarioMySQL{ss: ssComMySQL, ssCmd: "sudo", mysql: logica})

	if len(p.Instancias) != 1 || len(p.Instancias[0].Bases) != 1 {
		t.Fatalf("instâncias = %+v, esperado 1 com 1 base", p.Instancias)
	}
	b := p.Instancias[0].Bases[0]
	if b.TamanhoBytes == nil || *b.TamanhoBytes != 4096 || b.Conexoes != nil {
		t.Errorf("base = %+v, esperado tamanho medido e conexões nulas", b)
	}
}

func TestSondaMySQLUsaOComandoConfigurado(t *testing.T) {
	_, chamadas := rodarSondaMySQL(t, cenarioMySQL{
		ss: ssComMySQL, ssCmd: "sudo", mysql: mysqlSaudavel, mysqlCmd: "sudo -n mysql -u dk_monitor",
	})

	if !strings.Contains(chamadas, "MYSQL -u dk_monitor --connect-timeout=5") {
		t.Errorf("DOCKKEEPER_MYSQL_CMD não foi usado: %s", chamadas)
	}
}

func TestSondaMySQLExcluiSchemasDeSistema(t *testing.T) {
	for _, schema := range []string{"'mysql'", "'information_schema'", "'performance_schema'", "'sys'"} {
		if strings.Count(ProbeMySQL, schema) < 2 {
			t.Errorf("o SQL das bases não exclui %s nas duas variantes", schema)
		}
	}
}

func TestSondaMySQLNoHostSoComClienteMariaDBTrocaOComando(t *testing.T) {
	for _, comando := range []string{"", "sudo -n mysql"} {
		p, chamadas := rodarSondaMySQL(t, cenarioMySQL{
			ss: ssComMariaDBNaPorta3307, ssCmd: "sudo", mysql: mariadbSaudavel, soMariaDB: true, mysqlCmd: comando,
		})

		if len(p.Instancias) != 1 || p.Instancias[0].Estado != "ativo" || p.Instancias[0].Motor != "mariadb" {
			t.Fatalf("comando %q: instâncias = %+v, esperado mariadb ativo pelo cliente mariadb", comando, p.Instancias)
		}
		if !strings.Contains(chamadas, "MYSQL --connect-timeout=5 --protocol=TCP -h 127.0.0.1 -P 3307") {
			t.Errorf("comando %q: o cliente mariadb não foi chamado: %s", comando, chamadas)
		}
	}
}

func TestSondaMySQLConexoesNulasQuandoOBancoDevolveNULL(t *testing.T) {
	logica := `case "$sql" in
  *"VERSION()"*) printf '8.4.3\tMySQL Community Server - GPL\n' ;;
  *"SHOW REPLICA STATUS"*) exit 0 ;;
  *"information_schema.processlist"*) printf 'app\tutf8mb4\t4096\tNULL\n' ;;
  *) exit 1 ;;
esac
`
	p, _ := rodarSondaMySQL(t, cenarioMySQL{ss: ssComMySQL, ssCmd: "sudo", mysql: logica})

	if len(p.Instancias) != 1 || len(p.Instancias[0].Bases) != 1 {
		t.Fatalf("instâncias = %+v, esperado 1 com 1 base", p.Instancias)
	}
	if b := p.Instancias[0].Bases[0]; b.Conexoes != nil || b.TamanhoBytes == nil {
		t.Errorf("base = %+v, sem PROCESS as conexões ficam nulas e o tamanho continua medido", b)
	}
	if !strings.Contains(ProbeMySQL, "privilege_type = 'PROCESS'") {
		t.Error("a contagem de conexões precisa exigir o privilégio PROCESS do usuário corrente")
	}
}

func TestSondaMySQLEmContainerSemShellExplicaFaltaDeCliente(t *testing.T) {
	semShell := `printf 'OCI runtime exec failed: exec failed: unable to start container process: exec: "sh": executable file not found in $PATH\n'
exit 127
`
	p, _ := rodarSondaMySQL(t, cenarioMySQL{
		dockerPS: "banco|mysql:8.4|127.0.0.1:13306->3306/tcp", dockerExec: semShell,
	})

	if len(p.Instancias) != 1 {
		t.Fatalf("instâncias = %+v, esperado 1", p.Instancias)
	}
	inst := p.Instancias[0]
	if inst.Estado != "desconhecido" || !strings.Contains(inst.Motivo, "sem cliente mysql") ||
		!strings.Contains(inst.Motivo, "executable file not found") {
		t.Errorf("estado=%q motivo=%q, esperado desconhecido explicando a falta de shell", inst.Estado, inst.Motivo)
	}
}

func TestSondaMySQLEmContainerLeSenhaDoArquivoDeSegredo(t *testing.T) {
	segredo := filepath.Join(t.TempDir(), "raiz")
	if err := os.WriteFile(segredo, []byte("senha-do-segredo"), 0o600); err != nil {
		t.Fatal(err)
	}
	p, chamadas := rodarSondaMySQL(t, cenarioMySQL{
		dockerPS: "loja-db|mariadb:11|127.0.0.1:13306->3306/tcp",
		mysql:    mariadbSaudavel,
		env:      []string{"MARIADB_ROOT_PASSWORD_FILE=" + segredo},
	})

	if len(p.Instancias) != 1 || p.Instancias[0].Estado != "ativo" {
		t.Fatalf("instâncias = %+v, esperado ativo", p.Instancias)
	}
	if !strings.Contains(chamadas, "PWD senha-do-segredo") {
		t.Errorf("a senha do arquivo de segredo não chegou por MYSQL_PWD: %s", chamadas)
	}
}

package ssh

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const jsonDaSondaMySQL = `{"descoberta_host_ok":true,"descoberta_container_ok":true,"instancias":[{"porta":3306,"em_container":false,"container_nome":"","motor":"mariadb","versao":"10.11.6","papel":"primario","wal_level":"","archive_mode":"","estado":"ativo","motivo":"","bases":[{"nome":"loja","dono":"","encoding":"utf8mb4","tamanho_bytes":2048,"conexoes":1}]}]}`

func instanciaMySQLDeTeste(porta int, emContainer bool, motor string) PostgresInstanciaPayload {
	inst := instanciaDeTeste(porta, emContainer)
	inst.Motor = motor
	inst.Versao = "8.4.0"
	inst.Bases = []PostgresBasePayload{{Nome: "loja", Encoding: "utf8mb4"}}
	return inst
}

func motoresGravados(t *testing.T, serverID string) map[int]string {
	t.Helper()
	porPorta := map[int]string{}
	for _, linha := range instanciasGravadas(t, serverID) {
		porPorta[linha.Porta] = linha.Motor
	}
	return porPorta
}

func TestSondarMySQLLeOMotorDoJSON(t *testing.T) {
	alvo := alvoComServidor(t, func(string) respostaExec {
		return respostaExec{stdout: "ruído\n" + jsonDaSondaMySQL + "\n", consomeStdin: true}
	})

	p, err := SondarMySQL(context.Background(), alvo)
	if err != nil {
		t.Fatalf("sonda falhou: %v", err)
	}
	if len(p.Instancias) != 1 || p.Instancias[0].Motor != "mariadb" || p.Instancias[0].Porta != 3306 {
		t.Fatalf("instâncias = %+v, esperada uma mariadb na 3306", p.Instancias)
	}
}

func TestSondarMySQLSemJSONExplicaEmPortugues(t *testing.T) {
	alvo := alvoComServidor(t, func(string) respostaExec {
		return respostaExec{stdout: "bash: mysql: comando não encontrado\n", consomeStdin: true}
	})

	_, err := SondarMySQL(context.Background(), alvo)
	if err == nil || !strings.Contains(err.Error(), "sonda do mysql não devolveu JSON") {
		t.Fatalf("erro = %v, esperado explicar em pt-BR que a sonda do mysql não devolveu JSON", err)
	}
}

func TestNormalizarSondaMySQLSoAceitaOsMotoresDela(t *testing.T) {
	limpo := normalizarSondaMySQL(sondaPostgresDeTeste(true, true,
		instanciaMySQLDeTeste(3306, false, "MariaDB"),
		instanciaMySQLDeTeste(3307, false, "postgres"),
		instanciaMySQLDeTeste(3308, false, ""),
		instanciaMySQLDeTeste(3309, false, "mysql"),
	))

	esperado := map[int]string{3306: "mariadb", 3307: "mysql", 3308: "mysql", 3309: "mysql"}
	for _, inst := range limpo.Instancias {
		if inst.Motor != esperado[inst.Porta] {
			t.Errorf("porta %d: motor = %q, esperado %q", inst.Porta, inst.Motor, esperado[inst.Porta])
		}
	}
}

func TestNormalizarSondaPostgresIgnoraMotorAlheioNoJSON(t *testing.T) {
	limpo := normalizarSondaPostgres(sondaPostgresDeTeste(true, true, instanciaMySQLDeTeste(5432, false, "mysql")))
	if limpo.Instancias[0].Motor != motorPostgres {
		t.Errorf("motor = %q, esperado postgres: a sonda do postgres só grava postgres", limpo.Instancias[0].Motor)
	}
}

func TestGravarSondaMySQLGravaOMotorDeCadaInstancia(t *testing.T) {
	srv := servidorDeTeste(t)

	sonda := sondaPostgresDeTeste(true, true,
		instanciaMySQLDeTeste(3306, false, "mysql"), instanciaMySQLDeTeste(3307, true, "mariadb"))
	if err := gravarSondaMySQL(srv.ID, sonda, time.Now().UTC()); err != nil {
		t.Fatalf("sonda: %v", err)
	}

	motores := motoresGravados(t, srv.ID)
	if motores[3306] != "mysql" || motores[3307] != "mariadb" {
		t.Errorf("motores = %v, esperado 3306 mysql e 3307 mariadb", motores)
	}

	linhas := instanciasGravadas(t, srv.ID)
	for _, l := range linhas {
		if l.WalLevel != "" || l.MaxWalSenders != nil || l.ArchiveMode != "" {
			t.Errorf("porta %d com WAL preenchido (%q, %v, %q), esperado vazio/nulo", l.Porta, l.WalLevel, l.MaxWalSenders, l.ArchiveMode)
		}
	}
}

func TestSondaPostgresNaoPodaInstanciaDoMySQL(t *testing.T) {
	srv := servidorDeTeste(t)
	agora := time.Now().UTC()

	if err := gravarSondaMySQL(srv.ID, sondaPostgresDeTeste(true, true,
		instanciaMySQLDeTeste(3306, false, "mysql"), instanciaMySQLDeTeste(3307, true, "mariadb")), agora); err != nil {
		t.Fatalf("sonda do mysql: %v", err)
	}
	if err := gravarSondaPostgres(srv.ID, sondaPostgresDeTeste(true, true, instanciaDeTeste(5432, false)), agora.Add(time.Second)); err != nil {
		t.Fatalf("sonda do postgres: %v", err)
	}

	motores := motoresGravados(t, srv.ID)
	if len(motores) != 3 || motores[3306] != "mysql" || motores[3307] != "mariadb" || motores[5432] != motorPostgres {
		t.Fatalf("motores = %v, esperado as duas do mysql preservadas ao lado da do postgres", motores)
	}
}

func TestSondaMySQLNaoPodaInstanciaDoPostgres(t *testing.T) {
	srv := servidorDeTeste(t)
	agora := time.Now().UTC()

	if err := gravarSondaPostgres(srv.ID, sondaPostgresDeTeste(true, true,
		instanciaDeTeste(5432, false), instanciaDeTeste(5433, true)), agora); err != nil {
		t.Fatalf("sonda do postgres: %v", err)
	}
	if err := gravarSondaMySQL(srv.ID, sondaPostgresDeTeste(true, true), agora.Add(time.Second)); err != nil {
		t.Fatalf("sonda do mysql vazia: %v", err)
	}

	motores := motoresGravados(t, srv.ID)
	if len(motores) != 2 || motores[5432] != motorPostgres || motores[5433] != motorPostgres {
		t.Fatalf("motores = %v, esperado as do postgres intactas depois de a sonda do mysql não ver nada", motores)
	}
}

func TestSondaMySQLPodaSoOQueEraDela(t *testing.T) {
	srv := servidorDeTeste(t)
	agora := time.Now().UTC()

	if err := gravarSondaMySQL(srv.ID, sondaPostgresDeTeste(true, true,
		instanciaMySQLDeTeste(3306, false, "mysql"), instanciaMySQLDeTeste(3307, true, "mariadb")), agora); err != nil {
		t.Fatalf("primeira sonda do mysql: %v", err)
	}
	if err := gravarSondaPostgres(srv.ID, sondaPostgresDeTeste(true, true, instanciaDeTeste(5432, false)), agora); err != nil {
		t.Fatalf("sonda do postgres: %v", err)
	}
	if err := gravarSondaMySQL(srv.ID, sondaPostgresDeTeste(true, true,
		instanciaMySQLDeTeste(3306, false, "mysql")), agora.Add(time.Second)); err != nil {
		t.Fatalf("segunda sonda do mysql: %v", err)
	}

	motores := motoresGravados(t, srv.ID)
	if len(motores) != 2 || motores[3306] != "mysql" || motores[5432] != motorPostgres {
		t.Fatalf("motores = %v, esperado a 3307 podada e as outras preservadas", motores)
	}
}

func TestPortaDeOutroMotorNaoESobrescrita(t *testing.T) {
	srv := servidorDeTeste(t)
	agora := time.Now().UTC()

	if err := gravarSondaPostgres(srv.ID, sondaPostgresDeTeste(true, true, instanciaDeTeste(3306, false)), agora); err != nil {
		t.Fatalf("sonda do postgres: %v", err)
	}
	antes := instanciasGravadas(t, srv.ID)

	if err := gravarSondaMySQL(srv.ID, sondaPostgresDeTeste(true, true,
		instanciaMySQLDeTeste(3306, false, "mysql")), agora.Add(time.Second)); err != nil {
		t.Fatalf("sonda do mysql na mesma porta: %v", err)
	}

	depois := instanciasGravadas(t, srv.ID)
	if len(depois) != 1 || depois[0].ID != antes[0].ID || depois[0].Motor != motorPostgres || depois[0].Versao != "16.2" {
		t.Fatalf("instâncias = %+v, esperado a linha do postgres intacta: a sonda do mysql não toma porta alheia", depois)
	}
}

func TestSondaMySQLDesligadaNaoRodaEApagaOInventario(t *testing.T) {
	srv := servidorDeTeste(t)

	if err := gravarSondaMySQL(srv.ID, sondaPostgresDeTeste(true, true,
		instanciaMySQLDeTeste(3306, false, "mysql")), time.Now().UTC()); err != nil {
		t.Fatalf("semear inventário: %v", err)
	}

	var chamadas atomic.Int32
	alvo := alvoDeBancosContado(t, srv.ID, false, &chamadas)

	if !esperarRetorno(t, func(ctx context.Context) { SondarMySQLPeriodicamente(ctx, alvo) }) {
		t.Error("a sonda do mysql desligada ficou rodando, esperado retorno imediato")
	}
	if n := chamadas.Load(); n != 0 {
		t.Errorf("a sonda do mysql desligada abriu %d sessão(ões) SSH", n)
	}
	if linhas := instanciasGravadas(t, srv.ID); len(linhas) != 0 {
		t.Errorf("instâncias = %+v, esperado inventário apagado", linhas)
	}
}

func TestIntervaloDaSondaMySQLRespeitaAVariavel(t *testing.T) {
	if got := intervaloDaSondaMySQL(); got != 15*time.Minute {
		t.Errorf("intervalo padrão = %s, esperado 15m", got)
	}

	t.Setenv("MYSQL_PROBE_INTERVAL", "3m")
	if got := intervaloDaSondaMySQL(); got != 3*time.Minute {
		t.Errorf("intervalo = %s, esperado 3m", got)
	}

	t.Setenv("MYSQL_PROBE_INTERVAL", "um quarto de hora")
	if got := intervaloDaSondaMySQL(); got != 15*time.Minute {
		t.Errorf("intervalo inválido = %s, esperado o padrão de 15m", got)
	}
}

func TestMysqlCmdPadraoESudo(t *testing.T) {
	t.Setenv("SSH_MYSQL_CMD", "")
	t.Setenv("SSH_USE_SUDO", "false")
	if got := MysqlCmd(Target{User: "deploy"}); got != "mysql" {
		t.Errorf("sem sudo = %q, esperado mysql", got)
	}

	t.Setenv("SSH_USE_SUDO", "true")
	if got := MysqlCmd(Target{User: "deploy"}); got != "sudo -n mysql" {
		t.Errorf("com sudo e usuário comum = %q, esperado sudo -n mysql", got)
	}
	if got := MysqlCmd(Target{User: "root"}); got != "mysql" {
		t.Errorf("com sudo e root = %q, esperado mysql", got)
	}
}

func TestMysqlCmdRecusaMetacaractere(t *testing.T) {
	t.Setenv("SSH_USE_SUDO", "false")

	t.Setenv("SSH_MYSQL_CMD", "/usr/bin/mysql -u dk_monitor")
	if got := MysqlCmd(Target{}); got != "/usr/bin/mysql -u dk_monitor" {
		t.Errorf("comando seguro = %q, esperado repassado", got)
	}

	for _, hostil := range []string{"mysql; rm -rf /", "mysql $(id)", "mysql `id`", "mysql | nc x 1", `mysql "x"`} {
		t.Setenv("SSH_MYSQL_CMD", hostil)
		if got := MysqlCmd(Target{}); got != "mysql" {
			t.Errorf("SSH_MYSQL_CMD=%q virou %q, esperado o padrão mysql", hostil, got)
		}
	}
}

func TestPreludioRepassaOComandoDoMySQL(t *testing.T) {
	t.Setenv("SSH_MYSQL_CMD", "mysql -u dk_monitor")
	if !strings.Contains(scriptPrelude(Target{}), `DOCKKEEPER_MYSQL_CMD="mysql -u dk_monitor"`) {
		t.Errorf("prelúdio = %q, sem DOCKKEEPER_MYSQL_CMD", scriptPrelude(Target{}))
	}
}

package scripts

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	senhaRealMySQL   = "S3nha!forte"
	senhaRealMariaDB = "Outra$enha"
	senhaRealMySQL80 = "raiz80"
)

type bancoReal struct {
	nome     string
	imagem   string
	cliente  string
	env      []string
	senha    string
	extras   []string
	comandos []string
}

func dockerReal(t *testing.T, args ...string) string {
	t.Helper()
	saida, err := exec.Command("docker", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("docker %s: %v\n%s", strings.Join(args, " "), err, saida)
	}
	return strings.TrimSpace(string(saida))
}

func sqlReal(t *testing.T, b bancoReal, sql string) string {
	t.Helper()
	return dockerReal(t, "exec", "-e", "MYSQL_PWD="+b.senha, b.nome, b.cliente, "-uroot", "-N", "-B", "-e", sql)
}

func subirBancoReal(t *testing.T, rede string, b bancoReal) {
	t.Helper()
	args := []string{"run", "-d", "--rm", "--name", b.nome, "--network", rede, "-p", "127.0.0.1::3306"}
	for _, e := range b.env {
		args = append(args, "-e", e)
	}
	args = append(args, b.extras...)
	args = append(args, b.imagem)
	args = append(args, b.comandos...)
	dockerReal(t, args...)
	t.Cleanup(func() { _ = exec.Command("docker", "stop", b.nome).Run() })
}

func esperarBancoReal(t *testing.T, b bancoReal) {
	t.Helper()
	limite := time.Now().Add(3 * time.Minute)
	for time.Now().Before(limite) {
		logs, _ := exec.Command("docker", "logs", b.nome).CombinedOutput()
		if strings.Contains(string(logs), "port: 3306") {
			if b.senha == "" {
				return
			}
			err := exec.Command("docker", "exec", "-e", "MYSQL_PWD="+b.senha, b.nome,
				b.cliente, "-uroot", "-e", "SELECT 1").Run()
			if err == nil {
				return
			}
		}
		time.Sleep(time.Second)
	}
	logs, _ := exec.Command("docker", "logs", b.nome).CombinedOutput()
	t.Fatalf("%s não ficou pronto a tempo:\n%s", b.nome, logs)
}

func semearBancoReal(t *testing.T, b bancoReal) {
	t.Helper()
	sqlReal(t, b, "CREATE DATABASE app CHARACTER SET utf8mb4; "+
		"CREATE TABLE app.pedidos (id INT AUTO_INCREMENT PRIMARY KEY, txt VARCHAR(200)); "+
		"INSERT INTO app.pedidos (txt) SELECT REPEAT('x', 150) FROM information_schema.columns LIMIT 500; "+
		"CREATE DATABASE loja CHARACTER SET latin1; "+
		"CREATE TABLE loja.itens (id INT PRIMARY KEY); "+
		"CREATE USER 'leitor'@'%' IDENTIFIED BY 'leitor'; "+
		"GRANT SELECT, SHOW DATABASES ON *.* TO 'leitor'@'%'; "+
		"CREATE USER 'repl'@'%' IDENTIFIED BY 'repl'; "+
		"GRANT REPLICATION SLAVE ON *.* TO 'repl'@'%'")
}

func abrirConexaoOciosa(t *testing.T, b bancoReal) {
	t.Helper()
	dockerReal(t, "exec", "-d", "-e", "MYSQL_PWD="+b.senha, b.nome, b.cliente, "-uroot", "app", "-e", "SELECT SLEEP(600)")
	limite := time.Now().Add(30 * time.Second)
	for time.Now().Before(limite) {
		if sqlReal(t, b, "SELECT COUNT(*) FROM information_schema.processlist WHERE db = 'app'") != "0" {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("a conexão ociosa em %s não apareceu", b.nome)
}

func dockerFiltrado(t *testing.T, prefixo string) string {
	t.Helper()
	real, err := exec.LookPath("docker")
	if err != nil {
		t.Skip("docker indisponível")
	}
	dir := t.TempDir()
	corpo := "#!/bin/sh\nif [ \"$1\" = \"ps\" ]; then\n  \"" + real + "\" \"$@\" | grep '^" + prefixo + "'\n  exit 0\nfi\n" +
		"exec \"" + real + "\" \"$@\"\n"
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(corpo), 0o755); err != nil {
		t.Fatalf("criar docker filtrado: %v", err)
	}
	return dir
}

func lerSondaReal(t *testing.T, saida []byte) sondaMySQL {
	t.Helper()
	var p sondaMySQL
	if err := json.Unmarshal(saida, &p); err != nil {
		t.Fatalf("JSON inválido da sonda (%q): %v", string(saida), err)
	}
	return p
}

func instanciaDoContainer(t *testing.T, p sondaMySQL, nome string) instanciaMySQL {
	t.Helper()
	for _, inst := range p.Instancias {
		if inst.ContainerNome == nome {
			return inst
		}
	}
	t.Fatalf("container %s ausente da sonda: %+v", nome, p.Instancias)
	return instanciaMySQL{}
}

func conferirAtiva(t *testing.T, inst instanciaMySQL, motor, prefixoVersao, papel string) {
	t.Helper()
	if inst.Estado != "ativo" || inst.Motor != motor || !strings.HasPrefix(inst.Versao, prefixoVersao) || inst.Papel != papel {
		t.Errorf("%s: estado=%q motor=%q versao=%q papel=%q motivo=%q, esperado ativo/%s/%s*/%s",
			inst.ContainerNome, inst.Estado, inst.Motor, inst.Versao, inst.Papel, inst.Motivo, motor, prefixoVersao, papel)
	}
	if !inst.EmContainer || inst.Porta == 3306 || inst.Porta <= 0 {
		t.Errorf("%s: em_container=%v porta=%d, esperada a porta publicada", inst.ContainerNome, inst.EmContainer, inst.Porta)
	}
	nomes := map[string]bool{}
	for _, b := range inst.Bases {
		nomes[b.Nome] = true
	}
	if !nomes["app"] || !nomes["loja"] || nomes["mysql"] || nomes["sys"] || nomes["information_schema"] {
		t.Errorf("%s: bases = %+v, esperado app e loja sem schemas de sistema", inst.ContainerNome, inst.Bases)
	}
}

func TestSondaMySQLContraBancosReais(t *testing.T) {
	if os.Getenv("TESTE_MYSQL_REAL") != "1" {
		t.Skip("defina TESTE_MYSQL_REAL=1 para subir MySQL e MariaDB reais em containers")
	}
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash indisponível")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker indisponível")
	}

	prefixo := fmt.Sprintf("dk-teste-mysql-%d-", os.Getpid())
	rede := prefixo + "rede"
	dockerReal(t, "network", "create", rede)
	t.Cleanup(func() {
		for range 20 {
			if exec.Command("docker", "network", "rm", rede).Run() == nil {
				return
			}
			time.Sleep(500 * time.Millisecond)
		}
	})

	segredos := t.TempDir()
	if err := os.Chmod(segredos, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(segredos, "raiz"), []byte(senhaRealMariaDB), 0o644); err != nil {
		t.Fatal(err)
	}

	mysql84 := bancoReal{
		nome: prefixo + "mysql84", imagem: "mysql:8.4", cliente: "mysql", senha: senhaRealMySQL,
		env:      []string{"MYSQL_ROOT_PASSWORD=" + senhaRealMySQL},
		comandos: []string{"--server-id=1", "--gtid-mode=ON", "--enforce-gtid-consistency=ON"},
	}
	replica := bancoReal{
		nome: prefixo + "replica", imagem: "mysql:8.4", cliente: "mysql", senha: senhaRealMySQL,
		env:      []string{"MYSQL_ROOT_PASSWORD=" + senhaRealMySQL},
		comandos: []string{"--server-id=2", "--gtid-mode=ON", "--enforce-gtid-consistency=ON"},
	}
	mysql80 := bancoReal{
		nome: prefixo + "mysql80", imagem: "mysql:8.0", cliente: "mysql", senha: senhaRealMySQL80,
		env: []string{"MYSQL_ROOT_PASSWORD=" + senhaRealMySQL80},
	}
	mariadb := bancoReal{
		nome: prefixo + "mariadb11", imagem: "mariadb:11", cliente: "mariadb", senha: senhaRealMariaDB,
		env:    []string{"MARIADB_ROOT_PASSWORD_FILE=/run/secrets/raiz"},
		extras: []string{"-v", filepath.Join(segredos, "raiz") + ":/run/secrets/raiz:ro"},
	}
	semAcesso := bancoReal{
		nome: prefixo + "semacesso", imagem: "mysql:8.4", cliente: "mysql",
		env: []string{"MYSQL_RANDOM_ROOT_PASSWORD=yes"},
	}

	todos := []bancoReal{mysql84, replica, mysql80, mariadb, semAcesso}
	for _, b := range todos {
		subirBancoReal(t, rede, b)
	}
	for _, b := range todos {
		esperarBancoReal(t, b)
	}
	for _, b := range []bancoReal{mysql84, mysql80, mariadb} {
		semearBancoReal(t, b)
	}
	sqlReal(t, replica, "CHANGE REPLICATION SOURCE TO SOURCE_HOST='"+mysql84.nome+"', SOURCE_USER='repl', "+
		"SOURCE_PASSWORD='repl', SOURCE_AUTO_POSITION=1, GET_SOURCE_PUBLIC_KEY=1; START REPLICA")
	abrirConexaoOciosa(t, mysql84)
	abrirConexaoOciosa(t, mariadb)

	limite := time.Now().Add(time.Minute)
	for sqlReal(t, replica, "SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name = 'loja'") != "1" {
		if time.Now().After(limite) {
			t.Fatalf("a réplica não recebeu as bases do primário: %s", sqlReal(t, replica, "SHOW REPLICA STATUS"))
		}
		time.Sleep(time.Second)
	}

	t.Run("containers", func(t *testing.T) {
		cmd := exec.Command(bash, "-s")
		cmd.Env = append(os.Environ(),
			"PATH="+dockerFiltrado(t, prefixo)+string(os.PathListSeparator)+os.Getenv("PATH"),
			"HOME="+t.TempDir(),
			"DOCKKEEPER_MYSQL_CMD=mysql",
		)
		cmd.Stdin = strings.NewReader(ProbeMySQL)
		saida, err := cmd.Output()
		if err != nil {
			t.Fatalf("sonda falhou: %v", err)
		}
		p := lerSondaReal(t, saida)

		if !p.DescobertaContainerOk || len(p.Instancias) != len(todos) {
			t.Fatalf("descoberta_container_ok=%v instâncias=%+v, esperadas %d", p.DescobertaContainerOk, p.Instancias, len(todos))
		}

		i84 := instanciaDoContainer(t, p, mysql84.nome)
		conferirAtiva(t, i84, "mysql", "8.4.", "primario")
		app := baseMySQLChamada(t, i84, "app")
		if app.Encoding != "utf8mb4" || app.TamanhoBytes == nil || *app.TamanhoBytes <= 0 {
			t.Errorf("base app do 8.4 = %+v, esperado utf8mb4 com tamanho", app)
		}
		if app.Conexoes == nil || *app.Conexoes < 1 {
			t.Errorf("base app do 8.4 = %+v, esperada ao menos a conexão ociosa", app)
		}
		if loja := baseMySQLChamada(t, i84, "loja"); loja.Encoding != "latin1" {
			t.Errorf("base loja do 8.4 = %+v, esperado latin1", loja)
		}

		conferirAtiva(t, instanciaDoContainer(t, p, replica.nome), "mysql", "8.4.", "replica")
		conferirAtiva(t, instanciaDoContainer(t, p, mysql80.nome), "mysql", "8.0.", "primario")

		imdb := instanciaDoContainer(t, p, mariadb.nome)
		conferirAtiva(t, imdb, "mariadb", "11.", "primario")
		if c := baseMySQLChamada(t, imdb, "app").Conexoes; c == nil || *c < 1 {
			t.Errorf("base app do MariaDB sem a conexão ociosa: %v", c)
		}

		isa := instanciaDoContainer(t, p, semAcesso.nome)
		if isa.Estado != "sem_acesso" || !strings.Contains(isa.Motivo, "Access denied") || len(isa.Bases) != 0 {
			t.Errorf("sem acesso: estado=%q motivo=%q bases=%+v", isa.Estado, isa.Motivo, isa.Bases)
		}
	})

	t.Run("host com mariadb e usuario de leitura", func(t *testing.T) {
		dockerReal(t, "exec", mariadb.nome, "mkdir", "-p", "/tmp/dk-home")
		cnf := exec.Command("docker", "exec", "-i", mariadb.nome, "tee", "/tmp/dk-home/.my.cnf")
		cnf.Stdin = strings.NewReader("[client]\nuser=leitor\npassword=leitor\n")
		if saida, err := cnf.CombinedOutput(); err != nil {
			t.Fatalf("gravar .my.cnf: %v\n%s", err, saida)
		}

		cmd := exec.Command("docker", "exec", "-i", "--privileged", "-e", "HOME=/tmp/dk-home",
			"-e", "DOCKKEEPER_MYSQL_CMD=mysql", mariadb.nome, "bash", "-s")
		cmd.Stdin = strings.NewReader(ProbeMySQL)
		saida, err := cmd.Output()
		if err != nil {
			t.Fatalf("sonda no host falhou: %v", err)
		}
		p := lerSondaReal(t, saida)

		if !p.DescobertaHostOk || len(p.Instancias) != 1 {
			t.Fatalf("descoberta_host_ok=%v instâncias=%+v, esperada a 3306 do mariadbd", p.DescobertaHostOk, p.Instancias)
		}
		inst := p.Instancias[0]
		if inst.EmContainer || inst.Porta != 3306 || inst.Motor != "mariadb" || inst.Estado != "ativo" {
			t.Fatalf("instância = %+v, esperado mariadb ativo no host pela 3306", inst)
		}
		if inst.Papel != "desconhecido" {
			t.Errorf("papel = %q, sem SLAVE MONITOR o papel não pode ser afirmado", inst.Papel)
		}
		app := baseMySQLChamada(t, inst, "app")
		if app.Conexoes != nil {
			t.Errorf("conexões = %d, sem PROCESS a contagem só vê a própria sessão e deve ficar nula", *app.Conexoes)
		}
		if app.TamanhoBytes == nil || *app.TamanhoBytes <= 0 {
			t.Errorf("base app = %+v, esperado tamanho", app)
		}
	})
}

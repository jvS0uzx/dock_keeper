package logstore

import (
	"os"
	"testing"
	"time"

	"github.com/joaov/vd_stats/internal/database"
)

// UUIDs sintéticos deste pacote. Prefixo próprio porque os binários de teste de
// vários pacotes rodam em paralelo contra o mesmo Postgres, e limpeza cega de um
// já atropelou a de outro nesta base.
const (
	srvLogstore      = "00000000-0000-0000-0000-00000000105e"
	srvLogstoreOutro = "00000000-0000-0000-0000-00000000105f"
)

func setupLogstoreDB(t *testing.T) {
	t.Helper()

	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("DATABASE_URL não definido; pulando teste de logstore")
	}
	if database.DB == nil {
		if err := database.Connect(); err != nil {
			t.Skipf("banco indisponível: %v", err)
		}
	}
	limparLogs(t)
	t.Cleanup(func() { limparLogs(t) })
}

func limparLogs(t *testing.T) {
	t.Helper()
	database.DB.Where("server_id IN ?", []string{srvLogstore, srvLogstoreOutro}).
		Delete(&database.LogEntry{})
}

func contarLinhas(t *testing.T, serverID string) int64 {
	t.Helper()

	var n int64
	if err := database.DB.Model(&database.LogEntry{}).
		Where("server_id = ?", serverID).Count(&n).Error; err != nil {
		t.Fatalf("contar linhas de %s: %v", serverID, err)
	}
	return n
}

// Os dois chamadores de Save são laços `for scanner.Scan()` sobre `tail -f` de
// auth.log e de `docker logs`. Os dois fluxos emitem linha em branco o tempo
// todo, e log_entries é a tabela de maior volume do sistema: sem esta guarda,
// cada linha vazia vira uma linha gravada, e a poda passa a correr atrás de
// registro que nunca deveria ter existido.
func TestLinhaEmBrancoNaoEGravada(t *testing.T) {
	setupLogstoreDB(t)

	for _, vazia := range []string{"", " ", "\t", "\n", "   \t\n  "} {
		Save(srvLogstore, "auth", "", vazia)
	}

	if n := contarLinhas(t, srvLogstore); n != 0 {
		t.Errorf("linhas gravadas = %d, esperada nenhuma: linha em branco entrou no histórico", n)
	}
}

// O contraponto que impede o teste acima de ser satisfeito por um Save que
// ignora tudo. Confere também os quatro campos: gravar a linha no lugar do
// container, ou perder o source, quebraria o filtro da tela de Segurança sem
// quebrar a contagem.
func TestLinhaRealEGravadaComOsCamposIntactos(t *testing.T) {
	setupLogstoreDB(t)

	Save(srvLogstore, "container", "nginx_proxy", "Accepted publickey for root")

	var entrada database.LogEntry
	if err := database.DB.Where("server_id = ?", srvLogstore).First(&entrada).Error; err != nil {
		t.Fatalf("a linha não foi gravada: %v", err)
	}

	if entrada.Source != "container" {
		t.Errorf("source = %q, esperado container", entrada.Source)
	}
	if entrada.Container != "nginx_proxy" {
		t.Errorf("container = %q, esperado nginx_proxy", entrada.Container)
	}
	if entrada.Line != "Accepted publickey for root" {
		t.Errorf("line = %q, o conteúdo da linha não sobreviveu", entrada.Line)
	}
}

// A linha é gravada como veio, sem trim. O TrimSpace existe só para DECIDIR se a
// linha entra; aplicá-lo ao conteúdo comeria a indentação de stack trace e de
// log multilinha de container, que é justamente o que se lê quando algo quebrou.
func TestIndentacaoDaLinhaEPreservada(t *testing.T) {
	setupLogstoreDB(t)

	const comIndentacao = "    at main.handler (server.go:42)"
	Save(srvLogstore, "container", "app", comIndentacao)

	var entrada database.LogEntry
	if err := database.DB.Where("server_id = ?", srvLogstore).First(&entrada).Error; err != nil {
		t.Fatalf("a linha não foi gravada: %v", err)
	}
	if entrada.Line != comIndentacao {
		t.Errorf("line = %q, esperado %q: a indentação foi comida", entrada.Line, comIndentacao)
	}
}

// A linha é carimbada no instante da chamada. Não é preciosismo: quem define o
// horário aqui é o Go, não o Postgres, e um Timestamp deixado no valor zero
// grava o ano 1 — a linha nasce velha e a primeira passada da retenção a apaga,
// então o histórico simplesmente não existiria e nada acusaria erro.
//
// A conferência é do INSTANTE, não do fuso: a coluna é `timestamp with time
// zone`, então o Postgres normaliza a gravação e time.Now() e time.Now().UTC()
// produzem exatamente o mesmo valor. O .UTC() no código é estilo defensivo, não
// guarda de correção — um teste de fuso aqui passaria nos dois estados.
func TestLinhaECarimbadaNoInstanteDaChamada(t *testing.T) {
	setupLogstoreDB(t)

	antes := time.Now().UTC()
	Save(srvLogstore, "auth", "", "linha para conferir o horário")
	depois := time.Now().UTC()

	var entrada database.LogEntry
	if err := database.DB.Where("server_id = ?", srvLogstore).First(&entrada).Error; err != nil {
		t.Fatalf("a linha não foi gravada: %v", err)
	}

	gravado := entrada.Timestamp.UTC()
	// Um segundo de folga nas pontas absorve o arredondamento do Postgres.
	if gravado.Before(antes.Add(-time.Second)) || gravado.After(depois.Add(time.Second)) {
		t.Errorf("timestamp = %s, fora da janela [%s, %s] da chamada",
			gravado, antes, depois)
	}
}

// Save é chamada de dentro de um `for scanner.Scan()`. Uma linha que o banco
// recuse não pode derrubar o laço: o stream inteiro morreria por causa de uma
// linha, e a tela pararia de receber sem nenhum erro visível.
//
// O ServerID é `type:uuid`, então um valor que não seja UUID produz um erro real
// do Postgres — o mesmo caminho de "o banco recusou a linha", sem precisar
// derrubar a conexão compartilhada com os outros pacotes.
func TestErroDoBancoNaoDerrubaOChamador(t *testing.T) {
	setupLogstoreDB(t)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Save entrou em pânico com linha recusada pelo banco: %v", r)
		}
	}()

	Save("isto-nao-e-um-uuid", "auth", "", "linha que o banco vai recusar")
}

// A poda apaga o que é velho e preserva o que é recente. Uma inversão do sinal
// no cálculo do corte apagaria exatamente o inverso — o histórico recente, que é
// o que se consulta — e não quebraria nada visível até alguém procurar um log de
// ontem e não achar.
//
// Cuidado ao mexer: o DELETE de StartRetention é global, não recortado por
// server_id. maxAge de 24h com os demais pacotes semeando log em time.Now()
// mantém o corte longe das linhas deles; semear linha antiga em outro pacote
// passaria a interferir aqui.
func TestPodaApagaOVelhoEPreservaORecente(t *testing.T) {
	setupLogstoreDB(t)

	agora := time.Now().UTC()
	linhas := []database.LogEntry{
		{ServerID: srvLogstore, Source: "auth", Line: "linha de dois dias atrás", Timestamp: agora.Add(-48 * time.Hour)},
		{ServerID: srvLogstoreOutro, Source: "auth", Line: "linha de agora", Timestamp: agora},
	}
	for _, l := range linhas {
		if err := database.DB.Create(&l).Error; err != nil {
			t.Fatalf("semear linha de %s: %v", l.ServerID, err)
		}
	}

	// Intervalo longo de propósito: o laço poda UMA vez e fica bloqueado no
	// ticker. Sem isso a goroutine — que não tem como ser parada — ficaria
	// apagando a tabela durante o resto da suíte.
	StartRetention(24*time.Hour, time.Hour)

	esperarPoda(t, srvLogstore)

	if n := contarLinhas(t, srvLogstore); n != 0 {
		t.Errorf("linhas antigas restantes = %d, esperada nenhuma", n)
	}
	if n := contarLinhas(t, srvLogstoreOutro); n != 1 {
		t.Errorf("linhas recentes = %d, esperada 1: a poda levou o histórico recente junto", n)
	}
}

// esperarPoda aguarda a goroutine da retenção rodar a primeira passada. Sondar é
// preferível a dormir um valor fixo: o teste termina assim que a poda acontece,
// e falha com mensagem própria se ela não acontecer.
func esperarPoda(t *testing.T, serverID string) {
	t.Helper()

	limite := time.Now().Add(5 * time.Second)
	for time.Now().Before(limite) {
		if contarLinhas(t, serverID) == 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("a poda não rodou em 5s; o teste abaixo mediria o estado errado")
}

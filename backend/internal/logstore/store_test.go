package logstore

import (
	"os"
	"testing"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/database"
)

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

func TestLinhaEmBrancoNaoEGravada(t *testing.T) {
	setupLogstoreDB(t)

	for _, vazia := range []string{"", " ", "\t", "\n", "   \t\n  "} {
		Save(srvLogstore, "auth", "", vazia)
	}
	Flush()

	if n := contarLinhas(t, srvLogstore); n != 0 {
		t.Errorf("linhas gravadas = %d, esperada nenhuma: linha em branco entrou no histórico", n)
	}
}

func TestLinhaRealEGravadaComOsCamposIntactos(t *testing.T) {
	setupLogstoreDB(t)

	Save(srvLogstore, "container", "nginx_proxy", "Accepted publickey for root")
	Flush()

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

func TestIndentacaoDaLinhaEPreservada(t *testing.T) {
	setupLogstoreDB(t)

	const comIndentacao = "    at main.handler (server.go:42)"
	Save(srvLogstore, "container", "app", comIndentacao)
	Flush()

	var entrada database.LogEntry
	if err := database.DB.Where("server_id = ?", srvLogstore).First(&entrada).Error; err != nil {
		t.Fatalf("a linha não foi gravada: %v", err)
	}
	if entrada.Line != comIndentacao {
		t.Errorf("line = %q, esperado %q: a indentação foi comida", entrada.Line, comIndentacao)
	}
}

func TestLinhaECarimbadaNoInstanteDaChamada(t *testing.T) {
	setupLogstoreDB(t)

	antes := time.Now().UTC()
	Save(srvLogstore, "auth", "", "linha para conferir o horário")
	depois := time.Now().UTC()
	Flush()

	var entrada database.LogEntry
	if err := database.DB.Where("server_id = ?", srvLogstore).First(&entrada).Error; err != nil {
		t.Fatalf("a linha não foi gravada: %v", err)
	}

	gravado := entrada.Timestamp.UTC()
	if gravado.Before(antes.Add(-time.Second)) || gravado.After(depois.Add(time.Second)) {
		t.Errorf("timestamp = %s, fora da janela [%s, %s] da chamada",
			gravado, antes, depois)
	}
}

func TestErroDoBancoNaoDerrubaOChamador(t *testing.T) {
	setupLogstoreDB(t)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Save entrou em pânico com linha recusada pelo banco: %v", r)
		}
	}()

	Save("isto-nao-e-um-uuid", "auth", "", "linha que o banco vai recusar")
	Flush()
}

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

	StartRetention(24*time.Hour, time.Hour)

	esperarPoda(t, srvLogstore)

	if n := contarLinhas(t, srvLogstore); n != 0 {
		t.Errorf("linhas antigas restantes = %d, esperada nenhuma", n)
	}
	if n := contarLinhas(t, srvLogstoreOutro); n != 1 {
		t.Errorf("linhas recentes = %d, esperada 1: a poda levou o histórico recente junto", n)
	}
}

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

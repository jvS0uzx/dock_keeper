package logstore

import (
	"strconv"
	"testing"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/database"
)

func contarTransacoes(t *testing.T, serverID string) int64 {
	t.Helper()

	var n int64
	if err := database.DB.Raw("SELECT COUNT(DISTINCT xmin::text) FROM log_entries WHERE server_id = ?", serverID).
		Row().Scan(&n); err != nil {
		t.Fatalf("contar transações: %v", err)
	}
	return n
}

func TestLoteDe450LinhasChegaEmAteTresInserts(t *testing.T) {
	setupLogstoreDB(t)

	for i := range 450 {
		Save(srvLogstore, "container", "app", "linha "+strconv.Itoa(i))
	}

	prazo := time.Now().Add(3 * time.Second)
	for contarLinhas(t, srvLogstore) < 450 && time.Now().Before(prazo) {
		time.Sleep(50 * time.Millisecond)
	}
	if n := contarLinhas(t, srvLogstore); n != 450 {
		t.Fatalf("depois de 3 s havia %d de 450 linhas no banco", n)
	}
	if n := contarTransacoes(t, srvLogstore); n > 3 {
		t.Errorf("450 linhas geraram %d INSERTs em log_entries, esperado no máximo 3", n)
	}
}

func TestFilaCheiaDescartaSemEsperarOBanco(t *testing.T) {
	bloqueio := make(chan struct{})
	w := newWriter(100, func([]database.LogEntry) error {
		<-bloqueio
		return nil
	})
	t.Cleanup(func() { close(bloqueio) })

	inicio := time.Now()
	for i := range 1000 {
		w.save(database.LogEntry{ServerID: srvLogstore, Source: "auth", Line: "linha " + strconv.Itoa(i)})
	}
	if gasto := time.Since(inicio); gasto > time.Second {
		t.Errorf("1000 chamadas com o banco travado levaram %s: o stream esperou o banco", gasto)
	}
	if w.dropped() == 0 {
		t.Error("com a fila cheia nenhuma linha foi contada como descartada")
	}
}

func TestLinhaRecusadaNoLoteNaoDerrubaAsVizinhas(t *testing.T) {
	setupLogstoreDB(t)

	Save(srvLogstore, "auth", "", "boa antes")
	Save("isto-nao-e-um-uuid", "auth", "", "linha que o banco vai recusar")
	Save(srvLogstore, "auth", "", "boa depois")
	Flush()

	if n := contarLinhas(t, srvLogstore); n != 2 {
		t.Errorf("linhas boas gravadas = %d, esperado 2: uma linha ruim derrubou o lote", n)
	}
}

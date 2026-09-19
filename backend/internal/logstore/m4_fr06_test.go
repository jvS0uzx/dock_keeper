package logstore

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/database"
)

func lote(n int) []database.LogEntry {
	entradas := make([]database.LogEntry, 0, n)
	for i := 0; i < n; i++ {
		entradas = append(entradas, database.LogEntry{ServerID: "srv", Line: "linha", Timestamp: time.Now().UTC()})
	}
	return entradas
}

func TestErroDeConexaoNaoViraGravacaoLinhaALinha(t *testing.T) {
	var mu sync.Mutex
	var tamanhos []int
	var esperas []time.Duration
	quedas := 2

	w := newWriter(10, func(entradas []database.LogEntry) error {
		mu.Lock()
		tamanhos = append(tamanhos, len(entradas))
		restam := quedas
		quedas--
		mu.Unlock()
		if restam > 0 {
			return errors.New("dial tcp 127.0.0.1:5432: connect: connection refused")
		}
		return nil
	})
	w.conexaoCaiu = func(error) bool { return true }
	w.espera = func(d time.Duration) {
		mu.Lock()
		esperas = append(esperas, d)
		mu.Unlock()
	}

	w.write(lote(200))

	mu.Lock()
	defer mu.Unlock()
	for _, n := range tamanhos {
		if n != 200 {
			t.Fatalf("o lote foi quebrado em %d linhas: com o banco fora não pode virar gravação linha a linha (%v)", n, tamanhos)
		}
	}
	if len(tamanhos) != 3 {
		t.Errorf("%d tentativas de lote, esperadas 3", len(tamanhos))
	}
	if len(esperas) != 2 || esperas[0] != time.Second || esperas[1] != 2*time.Second {
		t.Errorf("esperas = %v, esperado backoff de 1 s e 2 s", esperas)
	}
}

func TestErroDeDadoIsolaALinhaRuim(t *testing.T) {
	var mu sync.Mutex
	var tamanhos []int

	w := newWriter(10, func(entradas []database.LogEntry) error {
		mu.Lock()
		tamanhos = append(tamanhos, len(entradas))
		mu.Unlock()
		if len(entradas) > 1 {
			return errors.New("ERROR: value too long for type character varying(16)")
		}
		return nil
	})
	w.conexaoCaiu = func(error) bool { return false }
	w.espera = func(time.Duration) { t.Error("erro de dado não pode esperar backoff") }

	w.write(lote(3))

	mu.Lock()
	defer mu.Unlock()
	if len(tamanhos) != 4 {
		t.Fatalf("tentativas = %v, esperado 1 lote e 3 linhas", tamanhos)
	}
	for _, n := range tamanhos[1:] {
		if n != 1 {
			t.Errorf("tentativa com %d linhas, esperada 1 a 1", n)
		}
	}
}

func TestLoteDesistidoContaComoDescartado(t *testing.T) {
	w := newWriter(10, func([]database.LogEntry) error {
		return errors.New("connection reset by peer")
	})
	w.conexaoCaiu = func(error) bool { return true }
	w.espera = func(time.Duration) {}
	w.maxTentativas = 3

	antes := w.dropped()
	w.write(lote(25))

	if delta := w.dropped() - antes; delta != 25 {
		t.Errorf("descartadas = %d, esperadas 25 depois de desistir do lote", delta)
	}
}

func TestContadorDeDescarteEExposto(t *testing.T) {
	if Descartadas() < 0 {
		t.Error("o acessor de linhas descartadas precisa existir para o /readyz")
	}
}

func TestSemBancoContaDescarteSemEsperar(t *testing.T) {
	w := newWriter(10, func([]database.LogEntry) error { return errSemBanco })
	w.espera = func(time.Duration) { t.Error("sem banco conectado não faz sentido esperar backoff") }

	antes := w.dropped()
	w.write(lote(7))

	if delta := w.dropped() - antes; delta != 7 {
		t.Errorf("descartadas = %d, esperadas 7 com o banco não conectado", delta)
	}
}

package alert

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/database"
	"github.com/jvS0uzx/dock_keeper/internal/observabilidade"
)

type saidaSegura struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *saidaSegura) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *saidaSegura) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

func resetFila() {
	filaMu.Lock()
	defer filaMu.Unlock()
	filaAtual, ultimoAvisoDaFila = Fila{}, time.Time{}
}

func capturarLog(t *testing.T) *saidaSegura {
	t.Helper()

	saida := &saidaSegura{}
	log.SetOutput(saida)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	return saida
}

func tempestade(t *testing.T, n int, desde time.Time) {
	t.Helper()

	linhas := make([]database.Alert, 0, n)
	for i := range n {
		linhas = append(linhas, database.Alert{
			Key: fmt.Sprintf("%stempestade-%d", prefixoFila, i), Severity: "critical",
			Text:   fmt.Sprintf("[CRITICO] tempestade %d", i),
			Status: database.AlertStatusOpen, CreatedAt: desde, NextAttemptAt: &desde,
			Delivery: database.AlertDeliveryPendente,
		})
	}
	if err := database.DB.Create(&linhas).Error; err != nil {
		t.Fatalf("criar a tempestade: %v", err)
	}
}

func TestFilaAtrasadaAvisaNoLogEExpoeAContagem(t *testing.T) {
	setupFila(t)
	saida := capturarLog(t)
	t.Cleanup(resetFila)

	base := time.Now().UTC()
	agora = func() time.Time { return base }
	tempestade(t, defaultDespachoLote+5, base.Add(-10*time.Minute))

	if n := dispatchPending(base); n != defaultDespachoLote {
		t.Fatalf("despachou %d, esperado o lote de %d: o ritmo de entrega mudou", n, defaultDespachoLote)
	}

	f := EstadoDaFila()
	if f.Pendentes < 5 {
		t.Errorf("pendentes = %d, esperado ao menos os 5 que sobraram do lote", f.Pendentes)
	}
	if f.AtrasoSeg == nil || *f.AtrasoSeg < 590 {
		t.Errorf("atraso = %v, esperado cerca de 600 s", f.AtrasoSeg)
	}
	if !f.Atrasada {
		t.Error("fila com alerta esperando há 10 min não foi marcada como atrasada")
	}
	if v := observabilidade.AlertasNaFila.Value(); v < 5 {
		t.Errorf("dockkeeper_alertas_na_fila = %d, esperado ao menos 5", v)
	}
	if !strings.Contains(saida.String(), "fila de alertas atrasada") {
		t.Errorf("nenhum aviso de fila atrasada no log:\n%s", saida.String())
	}

	dispatchPending(base)
	if n := strings.Count(saida.String(), "fila de alertas atrasada"); n != 1 {
		t.Errorf("aviso repetido %d vez(es) dentro do intervalo entre avisos", n)
	}
	if f := EstadoDaFila(); f.Atrasada || f.AtrasoSeg != nil {
		t.Errorf("fila vazia segue como %+v, esperado em dia e atraso nulo", f)
	}
	if !strings.Contains(saida.String(), "fila de alertas em dia") {
		t.Errorf("a volta ao normal não foi registrada no log:\n%s", saida.String())
	}
}

func TestFilaEmDiaNaoAvisa(t *testing.T) {
	setupFila(t)
	saida := capturarLog(t)
	t.Cleanup(resetFila)

	base := time.Now().UTC()
	agora = func() time.Time { return base }
	tempestade(t, 3, base.Add(-time.Second))

	dispatchPending(base)

	if strings.Contains(saida.String(), "fila de alertas atrasada") {
		t.Errorf("fila que esvaziou num lote gerou aviso:\n%s", saida.String())
	}
	if f := EstadoDaFila(); f.Atrasada {
		t.Errorf("fila em dia marcada como atrasada: %+v", f)
	}
}

package database

import (
	"os"
	"strings"
	"testing"
	"time"
)

// O achado E4: sem limite inferior o rollup varria metric_servers inteira a
// cada 15 minutos e reescrevia todos os baldes de todos os hosts.
func TestRollupSQLIncrementalLimitaAJanela(t *testing.T) {
	since := time.Now().UTC().Add(-trendRollupWindow)

	sql, args := rollupSQL(&since)

	if !strings.Contains(sql, "timestamp >= ?") {
		t.Errorf("rollup incremental sem limite inferior varre a tabela inteira:\n%s", sql)
	}
	if len(args) != 1 || args[0] != since {
		t.Errorf("args = %v, esperado apenas o corte %v", args, since)
	}
	if !strings.Contains(sql, "timestamp < date_trunc('hour', NOW())") {
		t.Error("a hora corrente precisa continuar de fora: agregá-la grava média parcial")
	}
}

// A primeira passada depois do boot não pode ter janela: se o painel ficou fora
// do ar por mais tempo que trendRollupWindow, o bruto daquele período ainda não
// virou trend e a poda — que espera justamente este rollup — o apagaria.
func TestRollupSQLCompletoNaoTemLimiteInferior(t *testing.T) {
	sql, args := rollupSQL(nil)

	if strings.Contains(sql, "timestamp >= ?") {
		t.Errorf("a varredura completa não pode limitar a janela:\n%s", sql)
	}
	if len(args) != 0 {
		t.Errorf("args = %v, esperado nenhum", args)
	}
}

func TestRollupSinceUsaJanelaSoDepoisDaPrimeiraPassada(t *testing.T) {
	if rollupSince(true) != nil {
		t.Error("a passada do boot deveria varrer tudo")
	}

	since := rollupSince(false)
	if since == nil {
		t.Fatal("as passadas seguintes deveriam limitar a janela")
	}
	if decorrido := time.Since(*since); decorrido < trendRollupWindow {
		t.Errorf("corte a %s atrás, esperado pelo menos %s", decorrido, trendRollupWindow)
	}
}

// O achado E5: a agregação devolve NULL de verdade nesses quatro campos, e NULL
// não entra num float64 — o Scan erra a linha inteira. O teste trava o tipo,
// que é o que a correção mudou.
func TestTrendGuardaNuloOndeAAgregacaoPodeDevolverNulo(t *testing.T) {
	var trend MetricServerTrend

	// Compila somente enquanto os quatro forem ponteiro.
	nulos := map[string]**float64{
		"mem_percent_avg":  &trend.MemPercentAvg,
		"disk_percent_avg": &trend.DiskPercentAvg,
		"temperature_avg":  &trend.TemperatureAvg,
		"temperature_max":  &trend.TemperatureMax,
	}

	for coluna, campo := range nulos {
		if *campo != nil {
			t.Errorf("%s deveria nascer nulo, e não zero", coluna)
		}
	}
}

// A partir daqui a cobertura é de integração: o que se verifica é o que o
// Postgres devolve da agregação, não a string do SQL. Pula sem DATABASE_URL.
const trendServerID = "00000000-0000-0000-0000-0000000000e5"

func setupTrendDB(t *testing.T) {
	t.Helper()

	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("DATABASE_URL não definido; pulando teste de agregação")
	}
	if DB == nil {
		if err := Connect(); err != nil {
			t.Skipf("banco indisponível: %v", err)
		}
	}
	limpaTrendDeTeste(t)
	t.Cleanup(func() { limpaTrendDeTeste(t) })
}

func limpaTrendDeTeste(t *testing.T) {
	t.Helper()
	DB.Where("server_id = ?", trendServerID).Delete(&MetricServer{})
	DB.Where("server_id = ?", trendServerID).Delete(&MetricServerTrend{})
}

// horaFechada devolve um instante dentro de uma hora que já terminou, que é a
// única que o rollup agrega.
func horaFechada(atras time.Duration) time.Time {
	return time.Now().UTC().Truncate(time.Hour).Add(-atras).Add(30 * time.Minute)
}

// O achado E5 até o fim: a hora sem sensor e sem total de memória agrega para
// NULL, e é a leitura de volta pelo modelo que quebrava com float64.
func TestRollupGravaNuloOndeNaoHouveMedicao(t *testing.T) {
	setupTrendDB(t)

	ts := horaFechada(2 * time.Hour)
	amostra := MetricServer{
		ServerID:        trendServerID,
		CPUUsagePercent: 42,
		LoadAvg1:        1.5,
		MemUsedBytes:    100,
		MemTotalBytes:   0, // host que não conseguiu ler o total
		DiskUsedBytes:   100,
		DiskTotalBytes:  0,
		TemperatureC:    nil, // máquina sem sensor
		Timestamp:       ts,
	}
	if err := DB.Create(&amostra).Error; err != nil {
		t.Fatalf("criar amostra: %v", err)
	}

	rollupTrends(nil)

	var trend MetricServerTrend
	if err := DB.Where("server_id = ?", trendServerID).First(&trend).Error; err != nil {
		t.Fatalf("ler a trend de volta: %v", err)
	}

	if trend.TemperatureAvg != nil {
		t.Errorf("temperatura média = %v, esperado nulo: nenhuma amostra tinha sensor", *trend.TemperatureAvg)
	}
	if trend.TemperatureMax != nil {
		t.Errorf("temperatura máxima = %v, esperado nulo", *trend.TemperatureMax)
	}
	if trend.MemPercentAvg != nil {
		t.Errorf("memória = %v, esperado nulo: o host não reportou o total", *trend.MemPercentAvg)
	}
	if trend.DiskPercentAvg != nil {
		t.Errorf("disco = %v, esperado nulo", *trend.DiskPercentAvg)
	}
	if trend.CPUAvg != 42 {
		t.Errorf("cpu média = %v, esperado 42: essa foi medida", trend.CPUAvg)
	}
}

// O achado E4: a passada incremental só reconsolida a janela recente. Sem isso
// ela reescrevia todos os baldes de todos os hosts a cada 15 minutos.
func TestRollupIncrementalIgnoraOQueEstaForaDaJanela(t *testing.T) {
	setupTrendDB(t)

	antigo := horaFechada(trendRollupWindow + 5*time.Hour)
	recente := horaFechada(time.Hour)
	for _, ts := range []time.Time{antigo, recente} {
		err := DB.Create(&MetricServer{
			ServerID: trendServerID, CPUUsagePercent: 10, Timestamp: ts,
		}).Error
		if err != nil {
			t.Fatalf("criar amostra em %s: %v", ts, err)
		}
	}

	rollupTrends(rollupSince(false))

	var baldes int64
	DB.Model(&MetricServerTrend{}).Where("server_id = ?", trendServerID).Count(&baldes)
	if baldes != 1 {
		t.Errorf("baldes consolidados = %d, esperado 1 (só o da janela)", baldes)
	}

	// E a varredura do boot precisa alcançar o que ficou para trás, senão a poda
	// apaga o bruto que nunca virou trend.
	rollupTrends(nil)
	DB.Model(&MetricServerTrend{}).Where("server_id = ?", trendServerID).Count(&baldes)
	if baldes != 2 {
		t.Errorf("baldes após a varredura completa = %d, esperado 2", baldes)
	}
}

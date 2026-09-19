package database

import (
	"os"
	"strings"
	"testing"
	"time"
)

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

func TestTrendGuardaNuloOndeAAgregacaoPodeDevolverNulo(t *testing.T) {
	var trend MetricServerTrend

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

func horaFechada(atras time.Duration) time.Time {
	return time.Now().UTC().Truncate(time.Hour).Add(-atras).Add(30 * time.Minute)
}

func TestRollupGravaNuloOndeNaoHouveMedicao(t *testing.T) {
	setupTrendDB(t)

	ts := horaFechada(2 * time.Hour)
	cpu42, carga15 := 42.0, 1.5
	amostra := MetricServer{
		ServerID:        trendServerID,
		CPUUsagePercent: &cpu42,
		LoadAvg1:        &carga15,
		MemUsedBytes:    100,
		MemTotalBytes:   0,
		DiskUsedBytes:   100,
		DiskTotalBytes:  0,
		TemperatureC:    nil,
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
	if trend.CPUAvg == nil || *trend.CPUAvg != 42 {
		t.Errorf("cpu média = %v, esperado 42: essa foi medida", trend.CPUAvg)
	}
}

func TestRollupIncrementalIgnoraOQueEstaForaDaJanela(t *testing.T) {
	setupTrendDB(t)

	antigo := horaFechada(trendRollupWindow + 5*time.Hour)
	recente := horaFechada(time.Hour)
	cpu10 := 10.0
	for _, ts := range []time.Time{antigo, recente} {
		err := DB.Create(&MetricServer{
			ServerID: trendServerID, CPUUsagePercent: &cpu10, Timestamp: ts,
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

	rollupTrends(nil)
	DB.Model(&MetricServerTrend{}).Where("server_id = ?", trendServerID).Count(&baldes)
	if baldes != 2 {
		t.Errorf("baldes após a varredura completa = %d, esperado 2", baldes)
	}
}

package database

import (
	"testing"
	"time"
)

func TestRollupDeCPUIgnoraAmostraSemMedicao(t *testing.T) {
	setupTrendDB(t)

	ts := horaFechada(2 * time.Hour)
	v10, v30, c1, c3 := 10.0, 30.0, 1.0, 3.0
	amostras := []MetricServer{
		{ServerID: trendServerID, CPUUsagePercent: &v10, LoadAvg1: &c1, Timestamp: ts},
		{ServerID: trendServerID, CPUUsagePercent: &v30, LoadAvg1: &c3, Timestamp: ts.Add(time.Minute)},
		{ServerID: trendServerID, Timestamp: ts.Add(2 * time.Minute)},
	}
	if err := DB.Create(&amostras).Error; err != nil {
		t.Fatalf("criar amostras: %v", err)
	}

	rollupTrends(nil)

	var trend MetricServerTrend
	if err := DB.Where("server_id = ?", trendServerID).First(&trend).Error; err != nil {
		t.Fatalf("ler a trend: %v", err)
	}
	if trend.CPUAvg == nil || *trend.CPUAvg != 20 {
		t.Errorf("cpu_avg = %v, esperado 20 (média de 10 e 30, sem contar a amostra sem medição)", trend.CPUAvg)
	}
	if trend.LoadAvg1Avg == nil || *trend.LoadAvg1Avg != 2 {
		t.Errorf("load_avg1_avg = %v, esperado 2", trend.LoadAvg1Avg)
	}
}

func TestRollupDeBaldeSemCPUFicaNulo(t *testing.T) {
	setupTrendDB(t)

	mem := int64(100)
	if err := DB.Create(&MetricServer{
		ServerID: trendServerID, MemUsedBytes: 10, MemTotalBytes: mem,
		Timestamp: horaFechada(2 * time.Hour),
	}).Error; err != nil {
		t.Fatalf("criar amostra: %v", err)
	}

	rollupTrends(nil)

	var trend MetricServerTrend
	if err := DB.Where("server_id = ?", trendServerID).First(&trend).Error; err != nil {
		t.Fatalf("ler a trend: %v", err)
	}
	if trend.CPUAvg != nil || trend.CPUMax != nil {
		t.Errorf("balde sem medição de cpu virou (%v, %v), esperado nulo", trend.CPUAvg, trend.CPUMax)
	}
	if trend.LoadAvg1Avg != nil || trend.LoadAvg1Max != nil {
		t.Errorf("balde sem medição de load virou (%v, %v), esperado nulo", trend.LoadAvg1Avg, trend.LoadAvg1Max)
	}
}

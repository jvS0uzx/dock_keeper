package database

import (
	"testing"
	"time"
)

func TestRollupMediaDeRedeIgnoraNulo(t *testing.T) {
	setupTrendDB(t)

	ts := horaFechada(2 * time.Hour)
	v100, v300, v10, v30 := 100.0, 300.0, 10.0, 30.0
	amostras := []MetricServer{
		{ServerID: trendServerID, NetRxBps: &v100, NetTxBps: &v10, Timestamp: ts},
		{ServerID: trendServerID, NetRxBps: &v300, NetTxBps: &v30, Timestamp: ts.Add(time.Minute)},
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
	if trend.NetRxAvg == nil || *trend.NetRxAvg != 200 {
		t.Errorf("net_rx_avg = %v, esperado 200 (média de 100 e 300, sem contar a amostra nula)", trend.NetRxAvg)
	}
	if trend.NetTxAvg == nil || *trend.NetTxAvg != 20 {
		t.Errorf("net_tx_avg = %v, esperado 20", trend.NetTxAvg)
	}
}

func TestRollupSemMedicaoDeRedeFicaNulo(t *testing.T) {
	setupTrendDB(t)

	cpu := 5.0
	if err := DB.Create(&MetricServer{ServerID: trendServerID, CPUUsagePercent: &cpu, Timestamp: horaFechada(2 * time.Hour)}).Error; err != nil {
		t.Fatalf("criar amostra: %v", err)
	}
	rollupTrends(nil)

	var trend MetricServerTrend
	if err := DB.Where("server_id = ?", trendServerID).First(&trend).Error; err != nil {
		t.Fatalf("ler a trend: %v", err)
	}
	if trend.NetRxAvg != nil || trend.NetTxAvg != nil {
		t.Errorf("rede sem medição virou (%v, %v); esperado nulo", trend.NetRxAvg, trend.NetTxAvg)
	}
}

func TestRollupDeRTTGuardaMediaEMaximo(t *testing.T) {
	setupTrendDB(t)

	ts := horaFechada(2 * time.Hour)
	v10, v30 := 10.0, 30.0
	amostras := []MetricServer{
		{ServerID: trendServerID, RTTMs: &v10, Timestamp: ts},
		{ServerID: trendServerID, RTTMs: &v30, Timestamp: ts.Add(time.Minute)},
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
	if trend.RTTAvg == nil || *trend.RTTAvg != 20 || trend.RTTMax == nil || *trend.RTTMax != 30 {
		t.Errorf("rtt_avg/rtt_max = (%v, %v), esperado (20, 30) sem contar a amostra nula", trend.RTTAvg, trend.RTTMax)
	}
}

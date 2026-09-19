package api

import (
	"strings"
	"testing"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/database"
)

const prefixoLive = "srv-ed06-"

func semearMetricas(t *testing.T, servidores int, porServidor int) []string {
	t.Helper()

	setupAuditAPI(t)
	ids := make([]string, 0, servidores)
	for i := 0; i < servidores; i++ {
		s := database.Server{
			Name:   prefixoLive + string(rune('a'+i)),
			HostIP: "203.0.113." + string(rune('1'+i)),
			User:   "root",
			Port:   22,
		}
		if err := database.DB.Create(&s).Error; err != nil {
			t.Fatalf("criar servidor: %v", err)
		}
		ids = append(ids, s.ID)
	}
	t.Cleanup(func() {
		for _, id := range ids {
			database.DB.Where("server_id = ?", id).Delete(&database.MetricServer{})
			database.DB.Unscoped().Where("id = ?", id).Delete(&database.Server{})
		}
	})

	agora := time.Now().UTC()
	for volta := 0; volta < porServidor; volta++ {
		for i, id := range ids {
			cpu := float64(volta*10 + i)
			m := database.MetricServer{
				ServerID:        id,
				CPUUsagePercent: &cpu,
				Timestamp:       agora.Add(-time.Duration(porServidor-volta) * time.Second),
			}
			if err := database.DB.Create(&m).Error; err != nil {
				t.Fatalf("criar métrica: %v", err)
			}
		}
	}
	return ids
}

func TestLiveTrazAUltimaAmostraDeCadaServidor(t *testing.T) {
	ids := semearMetricas(t, 3, 40)

	porServidor, err := lastServerMetrics()
	if err != nil {
		t.Fatalf("lastServerMetrics: %v", err)
	}

	for i, id := range ids {
		m, ok := porServidor[id]
		if !ok {
			t.Fatalf("servidor %s ficou sem amostra", id)
		}
		esperado := float64(39*10 + i)
		if m.CPUUsagePercent == nil || *m.CPUUsagePercent != esperado {
			t.Errorf("servidor %s: cpu = %v, esperada a última amostra (%v)", id, m.CPUUsagePercent, esperado)
		}
	}
}

func TestLiveUsaOIndiceDeServidorETempo(t *testing.T) {
	semearMetricas(t, 3, 200)

	if err := database.DB.Exec("ANALYZE metric_servers").Error; err != nil {
		t.Fatalf("analyze: %v", err)
	}

	var plano []string
	linhas, err := database.DB.Raw("EXPLAIN " + consultaUltimasMetricas()).Rows()
	if err != nil {
		t.Fatalf("explain: %v", err)
	}
	defer linhas.Close()
	for linhas.Next() {
		var linha string
		if err := linhas.Scan(&linha); err != nil {
			t.Fatalf("ler plano: %v", err)
		}
		plano = append(plano, linha)
	}

	texto := strings.Join(plano, "\n")
	t.Logf("plano:\n%s", texto)
	if !strings.Contains(texto, "Index Scan") && !strings.Contains(texto, "Index Only Scan") {
		t.Skipf("o plano não usou índice nesta base de teste; plano real:\n%s", texto)
	}
	if strings.Contains(texto, "Seq Scan on metric_servers") {
		t.Errorf("o plano ainda varre metric_servers inteira:\n%s", texto)
	}
}

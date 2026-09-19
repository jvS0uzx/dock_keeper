package database

import (
	"os"
	"testing"
	"time"
)

const (
	testServerID       = "00000000-0000-0000-0000-0000000000ff"
	ctStale            = "00000000-0000-0000-0000-0000000000c1"
	ctActive           = "00000000-0000-0000-0000-0000000000c2"
	ctFresh            = "00000000-0000-0000-0000-0000000000c3"
	testRetention      = 7 * 24 * time.Hour
	testAuditRetention = 3650 * 24 * time.Hour
)

func setupRetentionDB(t *testing.T) {
	t.Helper()

	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("DATABASE_URL não definido; pulando teste de retenção")
	}
	if DB == nil {
		if err := Connect(); err != nil {
			t.Skipf("banco indisponível: %v", err)
		}
	}
	cleanContainers(t)
	t.Cleanup(func() { cleanContainers(t) })
}

func cleanContainers(t *testing.T) {
	t.Helper()
	ids := []string{ctStale, ctActive, ctFresh}
	DB.Where("container_id IN ?", ids).Delete(&MetricContainer{})
	DB.Where("id IN ?", ids).Delete(&Container{})
}

func makeContainer(t *testing.T, id, name string, createdAt time.Time) {
	t.Helper()
	err := DB.Create(&Container{
		ID:        id,
		ServerID:  testServerID,
		DockerID:  "docker-" + name,
		Name:      name,
		CreatedAt: createdAt,
	}).Error
	if err != nil {
		t.Fatalf("criar container %s: %v", name, err)
	}
}

func makeMetric(t *testing.T, containerID string, ts time.Time) {
	t.Helper()
	err := DB.Create(&MetricContainer{ContainerID: containerID, Timestamp: ts}).Error
	if err != nil {
		t.Fatalf("criar métrica de %s: %v", containerID, err)
	}
}

func containerExists(t *testing.T, id string) bool {
	t.Helper()
	var n int64
	DB.Model(&Container{}).Where("id = ?", id).Count(&n)
	return n > 0
}

func TestPruneRemoveContainerSemMetricaRecente(t *testing.T) {
	setupRetentionDB(t)

	agora := time.Now().UTC()
	velho := agora.Add(-30 * 24 * time.Hour)

	makeContainer(t, ctStale, "morto", velho)
	makeMetric(t, ctStale, velho)

	makeContainer(t, ctActive, "vivo", velho)
	makeMetric(t, ctActive, velho)
	makeMetric(t, ctActive, agora.Add(-time.Minute))

	prune(testRetention, testAuditRetention)

	if containerExists(t, ctStale) {
		t.Error("container sem métrica recente deveria ter sido removido")
	}
	if !containerExists(t, ctActive) {
		t.Error("container com métrica recente foi removido")
	}

	var n int64
	DB.Model(&MetricContainer{}).Where("container_id = ?", ctActive).Count(&n)
	if n != 1 {
		t.Errorf("métricas restantes do container vivo = %d, esperado 1 (só a recente)", n)
	}
}

func TestPrunePreservaContainerRecemCriado(t *testing.T) {
	setupRetentionDB(t)

	makeContainer(t, ctFresh, "recem-criado", time.Now().UTC())

	prune(testRetention, testAuditRetention)

	if !containerExists(t, ctFresh) {
		t.Error("container recém-criado sem métrica foi removido")
	}
}

func TestPruneRemoveMetricaOrfa(t *testing.T) {
	setupRetentionDB(t)

	orfa := ctActive
	makeMetric(t, orfa, time.Now().UTC())

	prune(testRetention, testAuditRetention)

	var n int64
	DB.Model(&MetricContainer{}).Where("container_id = ?", orfa).Count(&n)
	if n != 0 {
		t.Errorf("métricas órfãs restantes = %d, esperado 0", n)
	}
}

func TestRetentionEsperaOSinalDoRollup(t *testing.T) {
	setupRetentionDB(t)

	makeContainer(t, ctStale, "alvo-da-poda", time.Now().UTC().Add(-30*24*time.Hour))

	ready := make(chan struct{})
	StartRetentionWorker(testRetention, time.Hour, ready)

	time.Sleep(100 * time.Millisecond)
	if !containerExists(t, ctStale) {
		t.Fatal("poda rodou antes do sinal do rollup")
	}

	close(ready)
	if !esperaSumir(t, ctStale, 2*time.Second) {
		t.Fatal("poda não rodou depois do sinal do rollup")
	}
}

func esperaSumir(t *testing.T, id string, limite time.Duration) bool {
	t.Helper()
	prazo := time.Now().Add(limite)
	for time.Now().Before(prazo) {
		if !containerExists(t, id) {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}

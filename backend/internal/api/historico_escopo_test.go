package api

import (
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/auth"
	"github.com/jvS0uzx/dock_keeper/internal/database"
)

const (
	srvEscopoA = "00000000-0000-0000-0000-0000000e5c0a"
	srvEscopoB = "00000000-0000-0000-0000-0000000e5c0b"
	ctnEscopoA = "00000000-0000-0000-0000-0000000e5ca1"
	ctnEscopoB = "00000000-0000-0000-0000-0000000e5cb1"
)

func limparEscopoDoHistorico() {
	database.DB.Where("container_id IN ?", []string{ctnEscopoA, ctnEscopoB}).Delete(&database.MetricContainer{})
	database.DB.Where("id IN ?", []string{ctnEscopoA, ctnEscopoB}).Delete(&database.Container{})
	database.DB.Unscoped().Where("id IN ?", []string{srvEscopoA, srvEscopoB}).Delete(&database.Server{})
	database.DB.Where("code IN ?", []string{"hist-escopo-a", "hist-escopo-b"}).Delete(&database.Site{})
}

func setupEscopoDoHistorico(t *testing.T) (filialA uint) {
	t.Helper()
	setupAuditAPI(t)

	limparEscopoDoHistorico()
	t.Cleanup(limparEscopoDoHistorico)

	sedeA := database.Site{Name: "Filial A do histórico", Code: "hist-escopo-a"}
	sedeB := database.Site{Name: "Filial B do histórico", Code: "hist-escopo-b"}
	for _, s := range []*database.Site{&sedeA, &sedeB} {
		if err := database.DB.Create(s).Error; err != nil {
			t.Fatalf("criar unidade %s: %v", s.Code, err)
		}
	}
	servidores := []database.Server{
		{ID: srvEscopoA, Name: "host-hist-a", HostIP: "10.93.0.1", SiteID: &sedeA.ID},
		{ID: srvEscopoB, Name: "host-hist-b", HostIP: "10.93.0.2", SiteID: &sedeB.ID},
	}
	for _, s := range servidores {
		if err := database.DB.Create(&s).Error; err != nil {
			t.Fatalf("criar servidor %s: %v", s.Name, err)
		}
	}
	containers := []database.Container{
		{ID: ctnEscopoA, ServerID: srvEscopoA, DockerID: "hist-a", Name: "api-a"},
		{ID: ctnEscopoB, ServerID: srvEscopoB, DockerID: "hist-b", Name: "api-b"},
	}
	agora := time.Now().UTC()
	for _, c := range containers {
		if err := database.DB.Create(&c).Error; err != nil {
			t.Fatalf("criar container %s: %v", c.Name, err)
		}
		amostra := database.MetricContainer{ContainerID: c.ID, CPUUsagePercent: 42, MemUsedBytes: 1, MemLimitBytes: 2, Timestamp: agora.Add(-5 * time.Minute)}
		if err := database.DB.Create(&amostra).Error; err != nil {
			t.Fatalf("criar amostra de %s: %v", c.Name, err)
		}
	}
	return sedeA.ID
}

func historicoDeContainer(t *testing.T, sess auth.Session, servidor, container string) int {
	t.Helper()
	q := url.Values{"server_id": {servidor}, "container_id": {container}, "metric": {"cpu"}}
	return pedirComSessao(t, http.MethodGet, "/api/metrics/history?"+q.Encode(), "", sess).Code
}

func TestHistoricoDeContainerExigeQueOContainerSejaDoServidor(t *testing.T) {
	filialA := setupEscopoDoHistorico(t)
	restrito := sessaoDeTeste(t, "viewer-hist-escopo", []auth.Access{{SiteID: &filialA, Role: auth.RoleViewer}})
	global := sessaoReal(t, "admin-hist-escopo", auth.RoleAdmin)

	if code := historicoDeContainer(t, restrito, srvEscopoA, ctnEscopoA); code != http.StatusOK {
		t.Fatalf("container da própria unidade: status %d, esperado 200", code)
	}
	if code := historicoDeContainer(t, restrito, srvEscopoA, ctnEscopoB); code != http.StatusNotFound {
		t.Errorf("container de outra unidade pelo servidor permitido: status %d, esperado 404", code)
	}
	if code := historicoDeContainer(t, global, srvEscopoA, ctnEscopoB); code != http.StatusNotFound {
		t.Errorf("container que não é do servidor pedido, mesmo para admin: status %d, esperado 404", code)
	}
	if code := historicoDeContainer(t, restrito, srvEscopoA, "nao-e-uuid"); code != http.StatusNotFound {
		t.Errorf("container_id inválido: status %d, esperado 404", code)
	}
}

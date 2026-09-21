package malha

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/database"
)

func setupMalhaDB(t *testing.T) {
	t.Helper()

	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("DATABASE_URL não definido; pulando teste de eleição da malha")
	}
	if database.DB == nil {
		if err := database.Connect(); err != nil {
			t.Skipf("banco indisponível: %v", err)
		}
	}
	mu.Lock()
	desafios = map[string]int{}
	mu.Unlock()
}

func unidadeDeTeste(t *testing.T, code string) uint {
	t.Helper()

	database.DB.Where("code = ?", code).Delete(&database.Site{})
	site := database.Site{Name: code, Code: code}
	if err := database.DB.Create(&site).Error; err != nil {
		t.Fatalf("criar unidade %s: %v", code, err)
	}
	t.Cleanup(func() { database.DB.Where("code = ?", code).Delete(&database.Site{}) })
	return site.ID
}

func servidorDeTeste(t *testing.T, nome, ip string, siteID uint, candidato bool) database.Server {
	t.Helper()

	estado := database.NginxAusente
	if candidato {
		estado = database.NginxCandidato
	}
	database.DB.Unscoped().Where("host_ip = ?", ip).Delete(&database.Server{})
	s := database.Server{
		Name: nome, HostIP: ip, User: "root", Port: 22, SiteID: &siteID,
		NginxEstado: estado,
	}
	if err := database.DB.Create(&s).Error; err != nil {
		t.Fatalf("criar servidor %s: %v", nome, err)
	}
	t.Cleanup(func() {
		database.DB.Where("server_id = ?", s.ID).Delete(&database.MetricLoadBalancer{})
		database.DB.Where("server_id = ?", s.ID).Delete(&database.Alert{})
		database.DB.Unscoped().Where("host_ip = ?", ip).Delete(&database.Server{})
	})
	return s
}

func registrarTrafego(t *testing.T, s database.Server, requisicoes int) {
	t.Helper()

	linha := database.MetricLoadBalancer{
		UpstreamAddr:  "198.51.100.10:8080",
		ServerName:    "exemplo.com.br",
		Status:        "200",
		ServerID:      &s.ID,
		SiteID:        s.SiteID,
		RequestsCount: requisicoes,
		Timestamp:     time.Now().UTC(),
	}
	if err := database.DB.Create(&linha).Error; err != nil {
		t.Fatalf("gravar tráfego de %s: %v", s.Name, err)
	}
}

func papelDe(t *testing.T, id string) (string, *time.Time) {
	t.Helper()

	var s database.Server
	if err := database.DB.Where("id = ?", id).First(&s).Error; err != nil {
		t.Fatalf("reler servidor %s: %v", id, err)
	}
	return s.NginxPapel, s.NginxPapelDesde
}

func TestElegerGravaOsPapeisEPreservaOCarimbo(t *testing.T) {
	setupMalhaDB(t)
	t.Setenv("MALHA_CICLOS_TROCA", "1")

	siteID := unidadeDeTeste(t, "malha-papeis")
	a := servidorDeTeste(t, "malha-papeis-a", "203.0.113.71", siteID, true)
	b := servidorDeTeste(t, "malha-papeis-b", "203.0.113.72", siteID, true)
	registrarTrafego(t, a, 500)
	registrarTrafego(t, b, 5)

	Eleger()

	papelA, desdeA := papelDe(t, a.ID)
	papelB, _ := papelDe(t, b.ID)
	if papelA != database.NginxPapelPrincipal {
		t.Fatalf("papel de a = %q, esperado principal: é quem recebeu o tráfego", papelA)
	}
	if papelB != database.NginxPapelReserva {
		t.Errorf("papel de b = %q, esperado reserva", papelB)
	}
	if desdeA == nil {
		t.Fatalf("nginx_papel_desde de a ficou nulo depois da eleição")
	}

	Eleger()

	_, depois := papelDe(t, a.ID)
	if depois == nil || !depois.Equal(*desdeA) {
		t.Errorf("nginx_papel_desde mudou sem o papel mudar: %v -> %v", desdeA, depois)
	}
}

func TestElegerAvisaQuandoOPrincipalTroca(t *testing.T) {
	setupMalhaDB(t)
	t.Setenv("MALHA_CICLOS_TROCA", "1")

	siteID := unidadeDeTeste(t, "malha-troca")
	a := servidorDeTeste(t, "malha-troca-a", "203.0.113.73", siteID, true)
	b := servidorDeTeste(t, "malha-troca-b", "203.0.113.74", siteID, true)

	registrarTrafego(t, a, 400)
	Eleger()
	if papel, _ := papelDe(t, a.ID); papel != database.NginxPapelPrincipal {
		t.Fatalf("papel de a = %q, esperado principal antes da troca", papel)
	}

	database.DB.Where("server_id = ?", a.ID).Delete(&database.MetricLoadBalancer{})
	registrarTrafego(t, b, 900)
	Eleger()

	if papel, _ := papelDe(t, b.ID); papel != database.NginxPapelPrincipal {
		t.Fatalf("papel de b = %q, esperado principal depois de receber o tráfego", papel)
	}

	var alertas []database.Alert
	if err := database.DB.Where("key LIKE ?", "nginx_principal:%").Find(&alertas).Error; err != nil {
		t.Fatalf("ler alertas: %v", err)
	}
	t.Cleanup(func() { database.DB.Where("key LIKE ?", "nginx_principal:%").Delete(&database.Alert{}) })

	var achado *database.Alert
	for i := range alertas {
		if strings.Contains(alertas[i].Text, a.Name) && strings.Contains(alertas[i].Text, b.Name) {
			achado = &alertas[i]
		}
	}
	if achado == nil {
		t.Fatalf("nenhum alerta citou a troca de %s para %s: %d alertas com a chave", a.Name, b.Name, len(alertas))
	}
	if achado.Severity != "high" {
		t.Errorf("severidade = %q, esperado high numa troca de balanceador", achado.Severity)
	}
	if achado.AlvoTipo == nil || *achado.AlvoTipo != database.AlvoTipoServico {
		t.Errorf("alvo_tipo = %v, esperado servico", achado.AlvoTipo)
	}
	if achado.AlvoNome == nil || *achado.AlvoNome != b.Name {
		t.Errorf("alvo_nome = %v, esperado %s", achado.AlvoNome, b.Name)
	}
}

func TestElegerNaoMisturaUnidades(t *testing.T) {
	setupMalhaDB(t)
	t.Setenv("MALHA_CICLOS_TROCA", "1")

	umID := unidadeDeTeste(t, "malha-unidade-1")
	doisID := unidadeDeTeste(t, "malha-unidade-2")
	um := servidorDeTeste(t, "malha-unidade-1-lb", "203.0.113.75", umID, true)
	dois := servidorDeTeste(t, "malha-unidade-2-lb", "203.0.113.76", doisID, true)
	registrarTrafego(t, um, 800)
	registrarTrafego(t, dois, 3)

	Eleger()

	if papel, _ := papelDe(t, um.ID); papel != database.NginxPapelPrincipal {
		t.Errorf("papel na unidade 1 = %q, esperado principal", papel)
	}
	if papel, _ := papelDe(t, dois.ID); papel != database.NginxPapelPrincipal {
		t.Errorf("papel na unidade 2 = %q, esperado principal: cada unidade elege o seu", papel)
	}
	t.Cleanup(func() { database.DB.Where("key LIKE ?", "nginx_principal:%").Delete(&database.Alert{}) })
}

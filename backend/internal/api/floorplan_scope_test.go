package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/joaov/vd_stats/internal/auth"
	"github.com/joaov/vd_stats/internal/database"
)

// Regressão do item N1. FloorPlanPin identifica o host só pelo IP, e a
// unicidade do inventário deixou de ser global: duas filiais com a mesma faixa
// RFC1918 têm o mesmo endereço em dois equipamentos diferentes. Resolvendo pelo
// endereço sozinho, o marcador de uma filial exibia o estado da outra.

const (
	ipRepetido    = "192.168.77.10"
	srvPlantaA    = "00000000-0000-0000-0000-00000000f001"
	srvPlantaB    = "00000000-0000-0000-0000-00000000f002"
	codigoPlantaA = "teste-planta-a"
	codigoPlantaB = "teste-planta-b"
)

// operadorGlobal é a sessão usada nos testes de gravação: o gate de papel não
// pode ser o que reprova quando o que está sob teste é a validação do endereço.
var operadorGlobal = auth.Session{
	Username: "operador-de-teste",
	Role:     auth.RoleOperator,
	Accesses: []auth.Access{{SiteID: nil, Role: auth.RoleOperator}},
}

type cenarioPlanta struct {
	siteA, siteB uint
	planoA       database.FloorPlan
	planoB       database.FloorPlan
}

func setupPlantaDB(t *testing.T) cenarioPlanta {
	t.Helper()

	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("DATABASE_URL não definido; pulando teste de recorte da planta")
	}
	if database.DB == nil {
		if err := database.Connect(); err != nil {
			t.Skipf("banco indisponível: %v", err)
		}
	}

	limpaPlantas(t)
	t.Cleanup(func() { limpaPlantas(t) })

	sedeA := database.Site{Name: "Filial A da planta", Code: codigoPlantaA}
	sedeB := database.Site{Name: "Filial B da planta", Code: codigoPlantaB}
	for _, s := range []*database.Site{&sedeA, &sedeB} {
		if err := database.DB.Create(s).Error; err != nil {
			t.Fatalf("criar unidade %s: %v", s.Code, err)
		}
	}

	agora := time.Now().UTC()
	// O mesmo endereço nas duas unidades: é o caso que a chave global não
	// distinguia. Só o da filial A está online, e os tipos diferem, para o teste
	// conseguir dizer qual das duas linhas foi resolvida.
	hosts := []database.NetworkHost{
		{
			IP: ipRepetido, Hostname: "maquina-da-filial-a", DeviceType: "linux",
			SiteID: &sedeA.ID, FirstSeen: agora, LastSeen: agora,
		},
		{
			IP: ipRepetido, Hostname: "maquina-da-filial-b", DeviceType: "printer",
			SiteID: &sedeB.ID, FirstSeen: agora, LastSeen: agora.Add(-24 * time.Hour),
		},
	}
	for _, h := range hosts {
		if err := database.DB.Create(&h).Error; err != nil {
			t.Fatalf("criar host %s da unidade %v: %v", h.IP, h.SiteID, err)
		}
	}

	servidores := []database.Server{
		{ID: srvPlantaA, Name: "maquina-da-filial-a", HostIP: ipRepetido, Kind: "agent", SiteID: &sedeA.ID},
		{ID: srvPlantaB, Name: "maquina-da-filial-b", HostIP: ipRepetido, Kind: "agent", SiteID: &sedeB.ID},
	}
	for _, s := range servidores {
		if err := database.DB.Create(&s).Error; err != nil {
			t.Fatalf("criar servidor %s: %v", s.Name, err)
		}
	}

	planoA := database.FloorPlan{Name: "Planta da filial A", SiteID: &sedeA.ID, ContentType: "image/png"}
	planoB := database.FloorPlan{Name: "Planta da filial B", SiteID: &sedeB.ID, ContentType: "image/png"}
	for _, p := range []*database.FloorPlan{&planoA, &planoB} {
		if err := database.DB.Create(p).Error; err != nil {
			t.Fatalf("criar planta %s: %v", p.Name, err)
		}
	}

	pins := []database.FloorPlanPin{
		{PlanID: planoA.ID, HostIP: ipRepetido, Label: "pino A", X: 10, Y: 10},
		{PlanID: planoB.ID, HostIP: ipRepetido, Label: "pino B", X: 20, Y: 20},
	}
	for _, pin := range pins {
		if err := database.DB.Create(&pin).Error; err != nil {
			t.Fatalf("criar pino da planta %d: %v", pin.PlanID, err)
		}
	}

	return cenarioPlanta{siteA: sedeA.ID, siteB: sedeB.ID, planoA: planoA, planoB: planoB}
}

func limpaPlantas(t *testing.T) {
	t.Helper()

	var sites []database.Site
	database.DB.Where("code IN ?", []string{codigoPlantaA, codigoPlantaB}).Find(&sites)
	ids := make([]uint, 0, len(sites))
	for _, s := range sites {
		ids = append(ids, s.ID)
	}

	if len(ids) > 0 {
		var planos []database.FloorPlan
		database.DB.Where("site_id IN ?", ids).Find(&planos)
		for _, p := range planos {
			database.DB.Where("plan_id = ?", p.ID).Delete(&database.FloorPlanPin{})
		}
		database.DB.Where("site_id IN ?", ids).Delete(&database.FloorPlan{})
		database.DB.Where("site_id IN ?", ids).Delete(&database.NetworkHost{})
	}
	database.DB.Unscoped().Where("id IN ?", []string{srvPlantaA, srvPlantaB}).Delete(&database.Server{})
	database.DB.Where("code IN ?", []string{codigoPlantaA, codigoPlantaB}).Delete(&database.Site{})
}

// O teste decisivo: o mesmo endereço nas duas unidades, uma planta em cada.
func TestPinoResolveOHostDaPropriaUnidade(t *testing.T) {
	c := setupPlantaDB(t)

	casos := []struct {
		nome         string
		plano        database.FloorPlan
		querHostname string
		querTipo     string
		querOnline   bool
		querServerID string
	}{
		{"planta da filial A", c.planoA, "maquina-da-filial-a", "linux", true, srvPlantaA},
		{"planta da filial B", c.planoB, "maquina-da-filial-b", "printer", false, srvPlantaB},
	}

	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			views, err := pinsWithState(caso.plano)
			if err != nil {
				t.Fatalf("pinsWithState: %v", err)
			}
			if len(views) != 1 {
				t.Fatalf("marcadores = %d, esperado 1", len(views))
			}

			v := views[0]
			if !v.Known {
				t.Fatal("o marcador não resolveu contra o inventário da unidade")
			}
			if v.Hostname != caso.querHostname {
				t.Errorf("hostname = %q, esperado %q: resolveu o host da unidade errada",
					v.Hostname, caso.querHostname)
			}
			if v.DeviceType != caso.querTipo {
				t.Errorf("tipo = %q, esperado %q", v.DeviceType, caso.querTipo)
			}
			if v.Online != caso.querOnline {
				t.Errorf("online = %v, esperado %v", v.Online, caso.querOnline)
			}
			if v.ServerID != caso.querServerID {
				t.Errorf("server_id = %q, esperado %q: o marcador abriria a tela da máquina errada",
					v.ServerID, caso.querServerID)
			}
		})
	}
}

// A planta da filial A não pode ler nenhuma linha de inventário da filial B.
// Um endereço que só existe na outra unidade tem que aparecer como
// desconhecido, e não emprestar o estado do vizinho.
func TestPlantaNaoEnxergaInventarioDeOutraUnidade(t *testing.T) {
	c := setupPlantaDB(t)

	const soNaFilialB = "192.168.77.99"
	agora := time.Now().UTC()
	hostB := database.NetworkHost{
		IP: soNaFilialB, Hostname: "exclusivo-da-b", DeviceType: "windows",
		SiteID: &c.siteB, FirstSeen: agora, LastSeen: agora,
	}
	if err := database.DB.Create(&hostB).Error; err != nil {
		t.Fatalf("criar host exclusivo da filial B: %v", err)
	}

	pino := database.FloorPlanPin{PlanID: c.planoA.ID, HostIP: soNaFilialB, Label: "vizinho", X: 50, Y: 50}
	if err := database.DB.Create(&pino).Error; err != nil {
		t.Fatalf("criar pino: %v", err)
	}

	views, err := pinsWithState(c.planoA)
	if err != nil {
		t.Fatalf("pinsWithState: %v", err)
	}

	for _, v := range views {
		if v.HostIP != soNaFilialB {
			continue
		}
		if v.Known {
			t.Errorf("o marcador da filial A resolveu %q contra o inventário da filial B (hostname %q)",
				soNaFilialB, v.Hostname)
		}
		return
	}
	t.Fatalf("o marcador de %s não voltou na resposta", soNaFilialB)
}

// Endereço malformado é erro do cliente e nunca resolveria; endereço bem
// formado ainda fora do inventário é estado legítimo, porque o operador
// posiciona a máquina antes de a varredura chegar nela.
func TestGravacaoDePinoRecusaEnderecoMalformado(t *testing.T) {
	c := setupPlantaDB(t)

	casos := []struct {
		nome    string
		hostIP  string
		querSts int
	}{
		{"endereço malformado", "nao-e-um-ip", http.StatusBadRequest},
		{"endereço válido fora do inventário", "192.168.77.201", http.StatusOK},
	}

	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			corpo, err := json.Marshal(map[string]any{
				"pins": []map[string]any{{"host_ip": caso.hostIP, "label": "novo", "x": 1.0, "y": 1.0}},
			})
			if err != nil {
				t.Fatal(err)
			}

			alvo := "/api/floorplans/" + strconv.FormatUint(uint64(c.planoA.ID), 10) + "/pins"
			req := httptest.NewRequest(http.MethodPut, alvo, bytes.NewReader(corpo))
			req = withSession(req, operadorGlobal)
			rec := httptest.NewRecorder()
			floorPlanPinsHandler(rec, req)

			if rec.Code != caso.querSts {
				t.Errorf("status = %d, esperado %d (corpo: %s)", rec.Code, caso.querSts, rec.Body.String())
			}
		})
	}
}

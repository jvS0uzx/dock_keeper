package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/joaov/vd_stats/internal/auth"
	"github.com/joaov/vd_stats/internal/database"
)

const (
	ipCompartilhado = "192.168.88.20"
	srvInvB         = "00000000-0000-0000-0000-0000000000b1"
)

// O índice de servidores da tela de inventário não era recortado: ele lia
// database.DB.Find(&servers) inteiro. Como Server.HostIP não é único e o
// inventário passou a admitir o mesmo IP em duas unidades, o host da filial A
// era anotado com o server_id do servidor homônimo da filial B — a mesma
// confusão de unidade do marcador da planta baixa, numa segunda tela.
func TestInventarioNaoAnotaServidorDeOutraUnidade(t *testing.T) {
	sedeA, sedeB := setupInventarioDB(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/network/hosts", nil)
	req = comSessaoDaUnidade(req, sedeA)

	networkHostsHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, esperado 200: %s", rec.Code, rec.Body.String())
	}

	var body struct {
		Hosts []struct {
			IP        string `json:"ip"`
			Monitored bool   `json:"monitored"`
			Kind      string `json:"kind"`
		} `json:"hosts"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("resposta não é JSON: %v", err)
	}

	var visto bool
	for _, h := range body.Hosts {
		if h.IP != ipCompartilhado {
			continue
		}
		visto = true
		// O servidor com esse endereço é da unidade B. Para a unidade A esta
		// máquina não está monitorada, e dizer que está manda o operador parar
		// de instalar agente numa máquina que não tem nenhum.
		if h.Monitored {
			t.Errorf("o host da unidade A saiu como monitorado por causa do servidor da unidade B")
		}
		if h.Kind != "" {
			t.Errorf("kind = %q, esperado vazio: veio do servidor da unidade B", h.Kind)
		}
	}
	if !visto {
		t.Fatalf("o host %s da unidade A não apareceu no inventário; o teste não mediu nada", ipCompartilhado)
	}

	_ = sedeB
}

func setupInventarioDB(t *testing.T) (uint, uint) {
	t.Helper()

	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("DATABASE_URL não definido; pulando teste de inventário")
	}
	if database.DB == nil {
		if err := database.Connect(); err != nil {
			t.Skipf("banco indisponível: %v", err)
		}
	}
	limparInventario(t)
	t.Cleanup(func() { limparInventario(t) })

	sedeA := database.Site{Name: "qa-inv-a", Code: "qa-inv-a"}
	sedeB := database.Site{Name: "qa-inv-b", Code: "qa-inv-b"}
	if err := database.DB.Create(&[]database.Site{sedeA, sedeB}).Error; err != nil {
		t.Fatalf("criar unidades: %v", err)
	}
	database.DB.Where("code = ?", "qa-inv-a").First(&sedeA)
	database.DB.Where("code = ?", "qa-inv-b").First(&sedeB)

	agora := time.Now().UTC()
	hosts := []database.NetworkHost{
		{IP: ipCompartilhado, Hostname: "maquina-a", SiteID: &sedeA.ID, FirstSeen: agora, LastSeen: agora},
		{IP: ipCompartilhado, Hostname: "maquina-b", SiteID: &sedeB.ID, FirstSeen: agora, LastSeen: agora},
	}
	if err := database.DB.Create(&hosts).Error; err != nil {
		t.Fatalf("criar hosts: %v", err)
	}

	// O servidor monitorado existe SÓ na unidade B, e a unidade A não tem
	// nenhum. Cadastrar um em cada faria o índice global escolher entre os dois
	// por ordem de retorno do banco, e o teste passaria metade das vezes por
	// acidente — foi assim que a primeira versão dele sobreviveu à mutação.
	servidor := database.Server{
		ID: srvInvB, Name: "srv-inv-b", HostIP: ipCompartilhado, Kind: "agent", SiteID: &sedeB.ID,
	}
	if err := database.DB.Create(&servidor).Error; err != nil {
		t.Fatalf("criar servidor: %v", err)
	}

	return sedeA.ID, sedeB.ID
}

func limparInventario(t *testing.T) {
	t.Helper()
	database.DB.Unscoped().Where("id = ?", srvInvB).Delete(&database.Server{})
	database.DB.Where("ip = ?", ipCompartilhado).Delete(&database.NetworkHost{})
	database.DB.Where("code IN ?", []string{"qa-inv-a", "qa-inv-b"}).Delete(&database.Site{})
}

// comSessaoDaUnidade injeta no contexto uma sessão de viewer restrita a uma
// unidade, no mesmo formato que requireAuth produz.
func comSessaoDaUnidade(r *http.Request, siteID uint) *http.Request {
	sess := auth.Session{
		UserID:   1,
		Username: "viewer-qa",
		Role:     auth.RoleViewer,
		Accesses: []auth.Access{{SiteID: &siteID, Role: auth.RoleViewer}},
	}
	return withSession(r, sess)
}

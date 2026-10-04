package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/auth"
	"github.com/jvS0uzx/dock_keeper/internal/database"
)

type cenarioDeLeitura struct {
	sedeA, sedeB uint
	host         database.NetworkHost
	comLeitura   database.NetworkInterface
	semLeitura   database.NetworkInterface
}

func setupLeituraSNMP(t *testing.T) cenarioDeLeitura {
	t.Helper()

	sedeA, sedeB, _ := setupSNMP(t)
	agora := time.Now().UTC()
	visto := agora.Add(-time.Minute)
	host := database.NetworkHost{IP: "198.51.100.50", Hostname: "sw-leitura", SiteID: &sedeA,
		FirstSeen: agora, LastSeen: agora, SnmpSysName: "sw-core", SnmpVistoEm: &visto}
	if err := database.DB.Create(&host).Error; err != nil {
		t.Fatalf("criar host: %v", err)
	}

	velocidade := int64(1000)
	com := database.NetworkInterface{NetworkHostID: host.ID, IfIndex: 2, IfName: "ge-0/0/2", SpeedMbps: &velocidade,
		OperStatus: "up", AdminStatus: "up", FirstSeen: agora, LastSeen: agora}
	sem := database.NetworkInterface{NetworkHostID: host.ID, IfIndex: 1, IfName: "ge-0/0/1",
		OperStatus: "down", AdminStatus: "down", FirstSeen: agora, LastSeen: agora}
	if err := database.DB.Create(&com).Error; err != nil {
		t.Fatalf("criar interface: %v", err)
	}
	if err := database.DB.Create(&sem).Error; err != nil {
		t.Fatalf("criar interface: %v", err)
	}

	bps := func(v float64) *float64 { return &v }
	erros := int64(3)
	leituras := []database.MetricNetworkInterface{
		{InterfaceID: com.ID, Ts: agora.Add(-2 * time.Hour), InBps: bps(1), OutBps: bps(1), OperStatus: "up"},
		{InterfaceID: com.ID, Ts: agora.Add(-10 * time.Minute), InBps: bps(100), OutBps: bps(50), OperStatus: "up"},
		{InterfaceID: com.ID, Ts: agora.Add(-time.Minute), InBps: bps(300), OutBps: nil, InErrors: &erros, OperStatus: "up"},
	}
	if err := database.DB.Create(&leituras).Error; err != nil {
		t.Fatalf("criar leituras: %v", err)
	}
	return cenarioDeLeitura{sedeA: sedeA, sedeB: sedeB, host: host, comLeitura: com, semLeitura: sem}
}

func getGlobal(t *testing.T, caminho string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, caminho, nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	rec := httptest.NewRecorder()
	Routes(testConfig()).ServeHTTP(rec, req)
	return rec
}

func TestInterfacesDoHostTrazUltimaLeituraENuloSemLeitura(t *testing.T) {
	c := setupLeituraSNMP(t)

	rec := getGlobal(t, fmt.Sprintf("/api/network/hosts/%d/interfaces", c.host.ID))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var corpo struct {
		Host struct {
			ID          uint       `json:"id"`
			SnmpSysName string     `json:"snmp_sys_name"`
			SnmpVistoEm *time.Time `json:"snmp_visto_em"`
			SnmpUptime  *int64     `json:"snmp_uptime_sec"`
		} `json:"host"`
		Interfaces []struct {
			ID     uint            `json:"id"`
			IfName string          `json:"if_name"`
			Ultima json.RawMessage `json:"ultima"`
		} `json:"interfaces"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&corpo); err != nil {
		t.Fatalf("resposta não é JSON: %v", err)
	}
	if corpo.Host.ID != c.host.ID || corpo.Host.SnmpSysName != "sw-core" || corpo.Host.SnmpVistoEm == nil {
		t.Errorf("host = %+v", corpo.Host)
	}
	if corpo.Host.SnmpUptime != nil {
		t.Errorf("uptime não medido virou %d", *corpo.Host.SnmpUptime)
	}
	if len(corpo.Interfaces) != 2 || corpo.Interfaces[0].IfName != "ge-0/0/1" {
		t.Fatalf("interfaces = %+v, esperado as duas ordenadas por if_index", corpo.Interfaces)
	}
	if string(corpo.Interfaces[0].Ultima) != "null" {
		t.Errorf("interface sem leitura trouxe ultima = %s", corpo.Interfaces[0].Ultima)
	}

	var ultima struct {
		InBps     *float64 `json:"in_bps"`
		OutBps    *float64 `json:"out_bps"`
		InErrors  *int64   `json:"in_errors"`
		OutErrors *int64   `json:"out_errors"`
	}
	if err := json.Unmarshal(corpo.Interfaces[1].Ultima, &ultima); err != nil {
		t.Fatalf("ultima: %v", err)
	}
	if ultima.InBps == nil || *ultima.InBps != 300 || ultima.OutBps != nil {
		t.Errorf("ultima = %+v, esperado in_bps 300 e out_bps nulo", ultima)
	}
	if ultima.InErrors == nil || *ultima.InErrors != 3 || ultima.OutErrors != nil {
		t.Errorf("contadores = %+v, esperado in_errors 3 e out_errors nulo", ultima)
	}
}

func TestInterfacesDeHostDeOutraUnidadeRespondem404(t *testing.T) {
	c := setupLeituraSNMP(t)

	sess := auth.Session{UserID: 1, Username: "viewer-snmp", Role: auth.RoleViewer,
		Accesses: []auth.Access{{SiteID: &c.sedeB, Role: auth.RoleViewer}}}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.SetPathValue("id", fmt.Sprint(c.host.ID))
	rec := httptest.NewRecorder()
	interfacesDoHostHandler(rec, withSession(req, sess))
	if rec.Code != http.StatusNotFound {
		t.Errorf("interfaces de outra unidade: status %d, esperado 404", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/?janela=1h", nil)
	req.SetPathValue("id", fmt.Sprint(c.comLeitura.ID))
	rec = httptest.NewRecorder()
	serieDaInterfaceHandler(rec, withSession(req, sess))
	if rec.Code != http.StatusNotFound {
		t.Errorf("série de outra unidade: status %d, esperado 404", rec.Code)
	}

	daUnidade := auth.Session{UserID: 1, Username: "viewer-snmp-a", Role: auth.RoleViewer,
		Accesses: []auth.Access{{SiteID: &c.sedeA, Role: auth.RoleViewer}}}
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.SetPathValue("id", fmt.Sprint(c.host.ID))
	rec = httptest.NewRecorder()
	interfacesDoHostHandler(rec, withSession(req, daUnidade))
	if rec.Code != http.StatusOK {
		t.Errorf("interfaces da própria unidade: status %d, esperado 200", rec.Code)
	}
}

func TestSerieDaInterfaceRespeitaAJanela(t *testing.T) {
	c := setupLeituraSNMP(t)

	pontos := func(janela string) []struct {
		Ts     time.Time `json:"ts"`
		InBps  *float64  `json:"in_bps"`
		OutBps *float64  `json:"out_bps"`
	} {
		t.Helper()
		caminho := fmt.Sprintf("/api/network/interfaces/%d/serie", c.comLeitura.ID)
		if janela != "" {
			caminho += "?janela=" + janela
		}
		rec := getGlobal(t, caminho)
		if rec.Code != http.StatusOK {
			t.Fatalf("janela %q: status %d (%s)", janela, rec.Code, rec.Body.String())
		}
		var corpo struct {
			Pontos []struct {
				Ts     time.Time `json:"ts"`
				InBps  *float64  `json:"in_bps"`
				OutBps *float64  `json:"out_bps"`
			} `json:"pontos"`
		}
		if err := json.NewDecoder(rec.Body).Decode(&corpo); err != nil {
			t.Fatalf("resposta: %v", err)
		}
		return corpo.Pontos
	}

	padrao := pontos("")
	if len(padrao) != 2 {
		t.Fatalf("janela padrão trouxe %d pontos, esperado 2 (a de 2h atrás fica fora de 1h)", len(padrao))
	}
	if !padrao[0].Ts.Before(padrao[1].Ts) {
		t.Errorf("pontos fora de ordem: %v", padrao)
	}
	if padrao[1].OutBps != nil {
		t.Errorf("out_bps nulo virou %v", *padrao[1].OutBps)
	}
	if n := len(pontos("6h")); n != 3 {
		t.Errorf("janela 6h trouxe %d pontos, esperado 3", n)
	}

	rec := getGlobal(t, fmt.Sprintf("/api/network/interfaces/%d/serie?janela=7d", c.comLeitura.ID))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("janela inválida: status %d, esperado 400", rec.Code)
	}
	if rec := getGlobal(t, fmt.Sprintf("/api/network/interfaces/%d/serie", c.semLeitura.ID)); !strings.Contains(rec.Body.String(), `"pontos":[]`) {
		t.Errorf("interface sem leitura: %s, esperado lista vazia", rec.Body.String())
	}
	if rec := getGlobal(t, "/api/network/interfaces/999999999/serie"); rec.Code != http.StatusNotFound {
		t.Errorf("interface inexistente: status %d, esperado 404", rec.Code)
	}
}

func TestRotasDeRedeConvivemNoMux(t *testing.T) {
	c := setupLeituraSNMP(t)

	rec := getGlobal(t, "/api/network/hosts")
	if rec.Code != http.StatusOK {
		t.Fatalf("lista: status %d", rec.Code)
	}
	var lista struct {
		Hosts []struct {
			ID          uint       `json:"id"`
			IP          string     `json:"ip"`
			SnmpVistoEm *time.Time `json:"snmp_visto_em"`
			SnmpErro    *string    `json:"snmp_erro"`
		} `json:"hosts"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&lista); err != nil {
		t.Fatalf("lista: %v", err)
	}
	achou := false
	for _, h := range lista.Hosts {
		if h.IP == c.host.IP {
			achou = h.ID == c.host.ID && h.SnmpVistoEm != nil && h.SnmpErro != nil
		}
	}
	if !achou {
		t.Errorf("a lista não expõe id, snmp_visto_em e snmp_erro do host SNMP")
	}

	if rec := getGlobal(t, fmt.Sprintf("/api/network/hosts/%d/interfaces", c.host.ID)); rec.Code != http.StatusOK {
		t.Errorf("interfaces: status %d", rec.Code)
	}
	if rec := getGlobal(t, "/api/network/hosts/abc/interfaces"); rec.Code != http.StatusNotFound {
		t.Errorf("id não numérico: status %d, esperado 404", rec.Code)
	}
	if rec := getGlobal(t, fmt.Sprintf("/api/network/hosts/%d/outra", c.host.ID)); rec.Code != http.StatusNotFound {
		t.Errorf("subrota desconhecida: status %d, esperado 404", rec.Code)
	}

	admin := sessaoReal(t, "admin-snmp-d1", auth.RoleAdmin)
	req := httptest.NewRequest(http.MethodPatch, "/api/network/host?ip="+c.host.IP, strings.NewReader(`{"rack":"R1"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+admin.Token)
	rec = httptest.NewRecorder()
	Routes(testConfig()).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("PATCH /api/network/host: status %d (%s)", rec.Code, rec.Body.String())
	}
}

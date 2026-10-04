package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/database"
)

const rotaDeTelemetria = "/api/ingest/network-metrics"

func setupSNMP(t *testing.T) (uint, uint, credencialDeTeste) {
	t.Helper()

	sedeA, sedeB := setupEnrollDB(t)
	setupAuditAPI(t)
	limpar := func() { database.DB.Where("ip LIKE ?", "198.51.100.%").Delete(&database.NetworkHost{}) }
	limpar()
	t.Cleanup(limpar)
	return sedeA, sedeB, credencialDoTipo(t, sedeA, kindCollector, "coletor-snmp-d1")
}

func corpoSNMP(dispositivos string) string {
	return `{"schema":1,"site_code":"` + codigoFilialA + `","collector_version":"d1","interval_sec":60,` +
		`"collected_at":"` + time.Now().UTC().Add(-time.Minute).Format(time.RFC3339) + `","devices":[` + dispositivos + `]}`
}

func switchSNMP(ip, interfaces string) string {
	return `{"ip":"` + ip + `","reachable":true,"error":"","sys_name":"sw-core","sys_descr":"switch de teste",` +
		`"uptime_sec":123456,"interfaces":[` + interfaces + `]}`
}

func interfaceSNMP(indice int, nome, oper string, inBps string) string {
	return fmt.Sprintf(`{"if_index":%d,"if_name":%q,"if_descr":"","if_alias":"uplink","speed_mbps":1000,`+
		`"oper_status":%q,"admin_status":"up","in_bps":%s,"out_bps":99.0,"in_errors":0,"out_errors":null,`+
		`"in_discards":0,"out_discards":null}`, indice, nome, oper, inBps)
}

func hostSNMP(t *testing.T, ip string, siteID uint) database.NetworkHost {
	t.Helper()

	var h database.NetworkHost
	if err := database.DB.Where("ip = ? AND site_id = ?", ip, siteID).First(&h).Error; err != nil {
		t.Fatalf("host %s não gravado: %v", ip, err)
	}
	return h
}

func interfacesSNMP(t *testing.T, hostID uint) []database.NetworkInterface {
	t.Helper()

	var lista []database.NetworkInterface
	database.DB.Where("network_host_id = ?", hostID).Order("id").Find(&lista)
	return lista
}

func metricasSNMP(t *testing.T, interfaceID uint) []database.MetricNetworkInterface {
	t.Helper()

	var lista []database.MetricNetworkInterface
	database.DB.Where("interface_id = ?", interfaceID).Order("ts").Find(&lista)
	return lista
}

func TestTelemetriaSNMPCriaHostInterfacesEMetricas(t *testing.T) {
	sedeA, _, cred := setupSNMP(t)

	corpo := corpoSNMP(switchSNMP("198.51.100.1",
		interfaceSNMP(1, "ge-0/0/1", "up", "1234.5")+","+interfaceSNMP(2, "ge-0/0/2", "LOWERLAYERDOWN", "null")+","+
			interfaceSNMP(3, "ge-0/0/3", "quebrado", "0")))
	rec := enviarComCredencial(t, rotaDeTelemetria, corpo, cred)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, esperado 200: %s", rec.Code, rec.Body.String())
	}
	var resposta struct{ Devices, Interfaces int }
	if err := json.NewDecoder(rec.Body).Decode(&resposta); err != nil {
		t.Fatalf("resposta não é JSON: %v", err)
	}
	if resposta.Devices != 1 || resposta.Interfaces != 3 {
		t.Errorf("resposta = %+v, esperado 1 dispositivo e 3 interfaces", resposta)
	}

	h := hostSNMP(t, "198.51.100.1", sedeA)
	if h.SnmpSysName != "sw-core" || h.SnmpVistoEm == nil || h.SnmpErro != "" {
		t.Errorf("host sem os campos SNMP: %+v", h)
	}
	if h.SnmpUptimeSec == nil || *h.SnmpUptimeSec != 123456 {
		t.Errorf("uptime = %v, esperado 123456", h.SnmpUptimeSec)
	}
	if h.DeviceType != "" {
		t.Errorf("device_type = %q, esperado vazio no host criado pelo SNMP", h.DeviceType)
	}

	lista := interfacesSNMP(t, h.ID)
	if len(lista) != 3 {
		t.Fatalf("%d interfaces gravadas, esperadas 3", len(lista))
	}
	if lista[1].OperStatus != "lowerLayerDown" || lista[2].OperStatus != "unknown" {
		t.Errorf("estados = %q e %q, esperado lowerLayerDown e unknown", lista[1].OperStatus, lista[2].OperStatus)
	}

	m := metricasSNMP(t, lista[0].ID)
	if len(m) != 1 || m[0].InBps == nil || *m[0].InBps != 1234.5 {
		t.Fatalf("métrica da ge-0/0/1 = %+v", m)
	}
	if m[0].OutErrors != nil || m[0].OutDiscards != nil {
		t.Errorf("contador nulo virou número: out_errors=%v out_discards=%v", m[0].OutErrors, m[0].OutDiscards)
	}
	if nula := metricasSNMP(t, lista[1].ID); len(nula) != 1 || nula[0].InBps != nil {
		t.Errorf("in_bps nulo virou número: %+v", nula)
	}
}

func TestTelemetriaSNMPReconciliaPorNomeEPreservaHistorico(t *testing.T) {
	sedeA, _, cred := setupSNMP(t)

	primeiro := corpoSNMP(switchSNMP("198.51.100.2",
		interfaceSNMP(1, "ge-0/0/1", "up", "10")+","+interfaceSNMP(2, "ge-0/0/2", "up", "20")))
	if rec := enviarComCredencial(t, rotaDeTelemetria, primeiro, cred); rec.Code != http.StatusOK {
		t.Fatalf("primeiro envio: %d %s", rec.Code, rec.Body.String())
	}
	h := hostSNMP(t, "198.51.100.2", sedeA)
	antes := interfacesSNMP(t, h.ID)

	segundo := corpoSNMP(switchSNMP("198.51.100.2", interfaceSNMP(7, "ge-0/0/1", "down", "30")))
	if rec := enviarComCredencial(t, rotaDeTelemetria, segundo, cred); rec.Code != http.StatusOK {
		t.Fatalf("segundo envio: %d %s", rec.Code, rec.Body.String())
	}

	depois := interfacesSNMP(t, h.ID)
	if len(depois) != 2 {
		t.Fatalf("%d interfaces depois da renumeração, esperadas 2: a que sumiu não pode ser apagada", len(depois))
	}
	if depois[0].ID != antes[0].ID || depois[0].IfIndex != 7 || depois[0].OperStatus != "down" {
		t.Errorf("ge-0/0/1 renumerada = %+v, esperado mesmo id %d com if_index 7", depois[0], antes[0].ID)
	}
	if n := len(metricasSNMP(t, antes[0].ID)); n != 2 {
		t.Errorf("ge-0/0/1 tem %d leituras, esperadas 2: a renumeração perdeu o histórico", n)
	}
	if !depois[1].LastSeen.Equal(antes[1].LastSeen) {
		t.Errorf("ge-0/0/2 ausente do envio teve last_seen atualizado")
	}
}

func TestTelemetriaSNMPSemNomeCasaPorIndice(t *testing.T) {
	sedeA, _, cred := setupSNMP(t)

	for range 2 {
		corpo := corpoSNMP(switchSNMP("198.51.100.3", interfaceSNMP(4, "", "up", "1")))
		if rec := enviarComCredencial(t, rotaDeTelemetria, corpo, cred); rec.Code != http.StatusOK {
			t.Fatalf("envio: %d %s", rec.Code, rec.Body.String())
		}
	}
	lista := interfacesSNMP(t, hostSNMP(t, "198.51.100.3", sedeA).ID)
	if len(lista) != 1 {
		t.Fatalf("%d interfaces, esperada 1: sem nome a identidade é o if_index", len(lista))
	}
	if n := len(metricasSNMP(t, lista[0].ID)); n != 2 {
		t.Errorf("%d leituras, esperadas 2", n)
	}
}

func TestTelemetriaSNMPInalcancavelSoGravaOErro(t *testing.T) {
	sedeA, _, cred := setupSNMP(t)

	ok := corpoSNMP(switchSNMP("198.51.100.4", interfaceSNMP(1, "ge-0/0/1", "up", "1")))
	if rec := enviarComCredencial(t, rotaDeTelemetria, ok, cred); rec.Code != http.StatusOK {
		t.Fatalf("envio alcançável: %d %s", rec.Code, rec.Body.String())
	}
	antes := hostSNMP(t, "198.51.100.4", sedeA)

	falha := corpoSNMP(`{"ip":"198.51.100.4","reachable":false,"error":"timeout","interfaces":[` +
		interfaceSNMP(9, "ge-0/0/9", "up", "1") + `]}`)
	rec := enviarComCredencial(t, rotaDeTelemetria, falha, cred)
	if rec.Code != http.StatusOK {
		t.Fatalf("envio inalcançável: %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"interfaces":0`) {
		t.Errorf("inalcançável contou interfaces: %s", rec.Body.String())
	}

	depois := hostSNMP(t, "198.51.100.4", sedeA)
	if depois.SnmpErro != "timeout" || depois.SnmpErroEm == nil {
		t.Errorf("erro não gravado: %q em %v", depois.SnmpErro, depois.SnmpErroEm)
	}
	if depois.SnmpVistoEm == nil || !depois.SnmpVistoEm.Equal(*antes.SnmpVistoEm) {
		t.Errorf("snmp_visto_em mudou num envio inalcançável")
	}
	lista := interfacesSNMP(t, depois.ID)
	if len(lista) != 1 || len(metricasSNMP(t, lista[0].ID)) != 1 {
		t.Errorf("inalcançável mexeu em interface ou métrica: %+v", lista)
	}

	if rec := enviarComCredencial(t, rotaDeTelemetria, ok, cred); rec.Code != http.StatusOK {
		t.Fatalf("envio de volta: %d %s", rec.Code, rec.Body.String())
	}
	if voltou := hostSNMP(t, "198.51.100.4", sedeA); voltou.SnmpErro != "" || voltou.SnmpErroEm != nil {
		t.Errorf("o erro ficou depois de voltar a responder: %q", voltou.SnmpErro)
	}
}

func TestTelemetriaSNMPNaoSobrescreveTipoTravado(t *testing.T) {
	sedeA, _, cred := setupSNMP(t)

	agora := time.Now().UTC()
	existente := database.NetworkHost{IP: "198.51.100.5", Hostname: "sw-andar", SiteID: &sedeA,
		DeviceType: "switch", DeviceTypeLocked: true, FirstSeen: agora.Add(-time.Hour), LastSeen: agora.Add(-time.Hour)}
	if err := database.DB.Create(&existente).Error; err != nil {
		t.Fatalf("criar host: %v", err)
	}

	corpo := corpoSNMP(switchSNMP("198.51.100.5", interfaceSNMP(1, "ge-0/0/1", "up", "1")))
	if rec := enviarComCredencial(t, rotaDeTelemetria, corpo, cred); rec.Code != http.StatusOK {
		t.Fatalf("envio: %d %s", rec.Code, rec.Body.String())
	}

	h := hostSNMP(t, "198.51.100.5", sedeA)
	if h.ID != existente.ID {
		t.Errorf("criou outro host (%d) em vez de usar o existente (%d)", h.ID, existente.ID)
	}
	if h.DeviceType != "switch" || !h.DeviceTypeLocked || h.Hostname != "sw-andar" {
		t.Errorf("cadastro sobrescrito: tipo %q travado=%v nome %q", h.DeviceType, h.DeviceTypeLocked, h.Hostname)
	}
}

func TestTelemetriaSNMPTruncaTextoNaFronteira(t *testing.T) {
	sedeA, _, cred := setupSNMP(t)

	longo := strings.Repeat("á", 400)
	corpo := corpoSNMP(`{"ip":"198.51.100.6","reachable":true,"sys_name":"sw","sys_descr":"` + longo + `","interfaces":[` +
		`{"if_index":1,"if_name":"` + longo + `","if_alias":"` + longo + `","oper_status":"up","admin_status":"up"}]}`)
	if rec := enviarComCredencial(t, rotaDeTelemetria, corpo, cred); rec.Code != http.StatusOK {
		t.Fatalf("envio: %d %s", rec.Code, rec.Body.String())
	}
	h := hostSNMP(t, "198.51.100.6", sedeA)
	if n := len([]rune(h.SnmpSysDescr)); n != 255 {
		t.Errorf("sys_descr com %d caracteres, esperado 255", n)
	}
	lista := interfacesSNMP(t, h.ID)
	if len(lista) != 1 || len([]rune(lista[0].IfName)) != 128 || len([]rune(lista[0].IfAlias)) != 128 {
		t.Errorf("nome/alias não truncados em 128: %+v", lista)
	}
	if lista[0].SpeedMbps != nil {
		t.Errorf("speed_mbps ausente virou %v", *lista[0].SpeedMbps)
	}
}

func TestTelemetriaSNMPHorarioNoFuturoUsaORelogioDoPainel(t *testing.T) {
	sedeA, _, cred := setupSNMP(t)

	corpo := strings.Replace(corpoSNMP(switchSNMP("198.51.100.7", interfaceSNMP(1, "ge-0/0/1", "up", "1"))),
		`"collected_at":"`, `"collected_at":"2099-01-01T00:00:00Z","x":"`, 1)
	if rec := enviarComCredencial(t, rotaDeTelemetria, corpo, cred); rec.Code != http.StatusOK {
		t.Fatalf("envio: %d %s", rec.Code, rec.Body.String())
	}
	lista := interfacesSNMP(t, hostSNMP(t, "198.51.100.7", sedeA).ID)
	m := metricasSNMP(t, lista[0].ID)
	if len(m) != 1 || m[0].Ts.After(time.Now().Add(time.Minute)) {
		t.Errorf("ts = %v, esperado o horário do painel", m)
	}
}

func TestTelemetriaSNMPRecusaEnvioInvalido(t *testing.T) {
	_, _, cred := setupSNMP(t)

	muitos := make([]string, maxDispositivosPorEnvio+1)
	for i := range muitos {
		muitos[i] = `{"ip":"198.51.100.200","reachable":false}`
	}
	muitasInterfaces := make([]string, maxInterfacesPorAparelho+1)
	for i := range muitasInterfaces {
		muitasInterfaces[i] = `{"if_index":1}`
	}

	casos := []struct {
		nome, corpo string
		status      int
	}{
		{"json quebrado", `{"schema":1,`, http.StatusBadRequest},
		{"schema 2", strings.Replace(corpoSNMP(""), `"schema":1`, `"schema":2`, 1), http.StatusBadRequest},
		{"sem schema", strings.Replace(corpoSNMP(""), `"schema":1,`, ``, 1), http.StatusBadRequest},
		{"ip inválido", corpoSNMP(switchSNMP("999.1.1.1", "")), http.StatusBadRequest},
		{"bps negativo", corpoSNMP(switchSNMP("198.51.100.8", interfaceSNMP(1, "ge-0/0/1", "up", "-1"))), http.StatusBadRequest},
		{"dispositivos demais", corpoSNMP(strings.Join(muitos, ",")), http.StatusRequestEntityTooLarge},
		{"interfaces demais", corpoSNMP(switchSNMP("198.51.100.9", strings.Join(muitasInterfaces, ","))), http.StatusRequestEntityTooLarge},
	}
	for _, c := range casos {
		rec := enviarComCredencial(t, rotaDeTelemetria, c.corpo, cred)
		if rec.Code != c.status {
			t.Errorf("%s: status %d, esperado %d (%s)", c.nome, rec.Code, c.status, rec.Body.String())
		}
	}

	var n int64
	database.DB.Model(&database.NetworkHost{}).Where("ip LIKE ?", "198.51.100.%").Count(&n)
	if n != 0 {
		t.Errorf("envio recusado gravou %d host(s)", n)
	}
}

func TestTelemetriaSNMPCorpoAcimaDoTetoDaIngestao(t *testing.T) {
	_, _, cred := setupSNMP(t)

	corpo := `{"schema":1,"x":"` + strings.Repeat("a", maxIngestBodyBytes) + `"}`
	if rec := enviarComCredencial(t, rotaDeTelemetria, corpo, cred); rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("status %d, esperado 413", rec.Code)
	}
}

func TestTelemetriaSNMPSemCredencialRecebe401(t *testing.T) {
	setupSNMP(t)

	rec := enviarComCredencial(t, rotaDeTelemetria, corpoSNMP(""), credencialDeTeste{DeviceID: "nao-existe", Token: "errado"})
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status %d, esperado 401", rec.Code)
	}
}

func TestCredencialDeAgenteNaoEnviaTelemetriaDeRede(t *testing.T) {
	sedeA, _, _ := setupSNMP(t)
	agente := credencialDoTipo(t, sedeA, kindAgent, "agente-snmp-d1")

	rec := enviarComCredencial(t, rotaDeTelemetria, corpoSNMP(switchSNMP("198.51.100.10", "")), agente)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status %d, esperado 403 (%s)", rec.Code, rec.Body.String())
	}
	unicaRecusa(t, "network_metrics.kind_mismatch", agente.DeviceID)
	if n := len(linhasDe(t, "ingest.network-metrics")); n != 0 {
		t.Errorf("o middleware gravou %d linha(s) extra(s)", n)
	}
}

func TestTelemetriaSNMPDeOutraUnidadeRecebe409EAuditaSoARecusa(t *testing.T) {
	_, _, cred := setupSNMP(t)

	corpo := strings.Replace(corpoSNMP(switchSNMP("198.51.100.11", "")), codigoFilialA, codigoFilialB, 1)
	rec := enviarComCredencial(t, rotaDeTelemetria, corpo, cred)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status %d, esperado 409 (%s)", rec.Code, rec.Body.String())
	}
	unicaRecusa(t, "network_metrics.site_mismatch", cred.DeviceID)

	ok := corpoSNMP(switchSNMP("198.51.100.11", ""))
	if rec := enviarComCredencial(t, rotaDeTelemetria, ok, cred); rec.Code != http.StatusOK {
		t.Fatalf("envio válido: %d %s", rec.Code, rec.Body.String())
	}
	if n := len(linhasDe(t, "ingest.network-metrics")); n != 0 {
		t.Errorf("envio aceito gerou %d linha(s) de auditoria; só a recusa é auditada", n)
	}
}

package api

import (
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/jvS0uzx/dock_keeper/internal/database"
	"github.com/jvS0uzx/dock_keeper/internal/discovery"
)

func listaDePortas(de, ate int) string {
	var b strings.Builder
	for p := de; p <= ate; p++ {
		if p > de {
			b.WriteByte(',')
		}
		b.WriteString(strconv.Itoa(p))
	}
	return b.String()
}

func corpoComPortas(ip, portas string) string {
	return `{"site_code":"qa-n3-cap","hosts":[{"ip":"` + ip + `","hostname":"qa-portas","open_ports":[` + portas + `]}]}`
}

func portasGravadas(t *testing.T, ip string) string {
	t.Helper()
	var host database.NetworkHost
	if err := database.DB.Where("ip = ?", ip).First(&host).Error; err != nil {
		t.Fatalf("host %s não encontrado: %v", ip, err)
	}
	return host.OpenPorts
}

func TestHostComPortasDemaisEDescartadoSemDerrubarOLote(t *testing.T) {
	setupInventarioCap(t)

	corpo := `{"site_code":"qa-n3-cap","hosts":[` +
		`{"ip":"10.93.7.51","hostname":"qa-portas-demais","open_ports":[` + listaDePortas(1, maxPortasPorHost+1) + `]},` +
		`{"ip":"10.93.7.52","hostname":"qa-portas-ok","open_ports":[22,80]}]}`
	rec := requisicaoDeInventario(t, corpo)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, esperado 200: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"stored":1`) {
		t.Fatalf("esperava só o host válido gravado: %s", rec.Body.String())
	}

	var descartado int64
	database.DB.Model(&database.NetworkHost{}).Where("ip = ?", "10.93.7.51").Count(&descartado)
	if descartado != 0 {
		t.Errorf("host com portas demais foi gravado")
	}
	if portas := portasGravadas(t, "10.93.7.52"); portas != "22,80" {
		t.Errorf("portas do host válido = %q, esperado \"22,80\"", portas)
	}
}

func TestPortasAcimaDoTetoSaoCortadasNaGravacao(t *testing.T) {
	setupInventarioCap(t)

	rec := requisicaoDeInventario(t, corpoComPortas("10.93.7.53", listaDePortas(1, 300)))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, esperado 200: %s", rec.Code, rec.Body.String())
	}

	portas := strings.Split(portasGravadas(t, "10.93.7.53"), ",")
	if len(portas) != discovery.MaxPortasGravadas {
		t.Fatalf("portas gravadas = %d, esperado %d", len(portas), discovery.MaxPortasGravadas)
	}
	if portas[0] != "1" {
		t.Errorf("primeira porta = %q, esperado \"1\"", portas[0])
	}
}

func TestPortasDuplicadasEForaDeFaixaNaoChegamNoBanco(t *testing.T) {
	setupInventarioCap(t)

	rec := requisicaoDeInventario(t, corpoComPortas("10.93.7.54", "443,80,80,0,70000,-1,22,443"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, esperado 200: %s", rec.Code, rec.Body.String())
	}

	if portas := portasGravadas(t, "10.93.7.54"); portas != "22,80,443" {
		t.Errorf("portas = %q, esperado \"22,80,443\"", portas)
	}
}

package database

import (
	"context"
	"testing"
	"time"
)

const ipDaPodaSNMP = "198.51.100.250"

func interfaceDePoda(t *testing.T) NetworkInterface {
	t.Helper()

	setupRetentionDB(t)
	limpar := func() { DB.Where("ip = ?", ipDaPodaSNMP).Delete(&NetworkHost{}) }
	limpar()
	t.Cleanup(limpar)

	agora := time.Now().UTC()
	host := NetworkHost{IP: ipDaPodaSNMP, FirstSeen: agora, LastSeen: agora}
	if err := DB.Create(&host).Error; err != nil {
		t.Fatalf("criar host: %v", err)
	}
	itf := NetworkInterface{NetworkHostID: host.ID, IfIndex: 1, IfName: "ge-0/0/1",
		OperStatus: "up", AdminStatus: "up", FirstSeen: agora, LastSeen: agora}
	if err := DB.Create(&itf).Error; err != nil {
		t.Fatalf("criar interface: %v", err)
	}
	return itf
}

func TestPodaDeMetricaDeInterfaceRespeitaARetencao(t *testing.T) {
	itf := interfaceDePoda(t)

	agora := time.Now().UTC()
	leituras := []MetricNetworkInterface{
		{InterfaceID: itf.ID, Ts: agora.Add(-80 * time.Hour), OperStatus: "up"},
		{InterfaceID: itf.ID, Ts: agora.Add(-73 * time.Hour), OperStatus: "up"},
		{InterfaceID: itf.ID, Ts: agora.Add(-time.Hour), OperStatus: "up"},
	}
	if err := DB.Create(&leituras).Error; err != nil {
		t.Fatalf("criar leituras: %v", err)
	}

	PodarMetricasDeInterface(context.Background(), defaultNetworkMetricRetention)

	var restantes int64
	DB.Model(&MetricNetworkInterface{}).Where("interface_id = ?", itf.ID).Count(&restantes)
	if restantes != 1 {
		t.Errorf("%d leituras depois da poda, esperada 1 (só a de 1h atrás)", restantes)
	}
	var interfaces int64
	DB.Model(&NetworkInterface{}).Where("id = ?", itf.ID).Count(&interfaces)
	if interfaces != 1 {
		t.Errorf("a poda de métrica apagou a interface")
	}
}

func TestApagarHostLevaInterfacesELeituras(t *testing.T) {
	itf := interfaceDePoda(t)

	if err := DB.Create(&MetricNetworkInterface{InterfaceID: itf.ID, Ts: time.Now().UTC(), OperStatus: "up"}).Error; err != nil {
		t.Fatalf("criar leitura: %v", err)
	}
	DB.Where("ip = ?", ipDaPodaSNMP).Delete(&NetworkHost{})

	var interfaces, leituras int64
	DB.Model(&NetworkInterface{}).Where("id = ?", itf.ID).Count(&interfaces)
	DB.Model(&MetricNetworkInterface{}).Where("interface_id = ?", itf.ID).Count(&leituras)
	if interfaces != 0 || leituras != 0 {
		t.Errorf("sobraram %d interface(s) e %d leitura(s) de host apagado", interfaces, leituras)
	}
}

func TestEstadoDeInterfaceForaDoEnumEhRecusadoPeloBanco(t *testing.T) {
	itf := interfaceDePoda(t)

	err := DB.Model(&NetworkInterface{}).Where("id = ?", itf.ID).Update("oper_status", "quebrado").Error
	if err == nil {
		t.Error("o banco aceitou oper_status fora do enum")
	}
}

func TestNomeDeInterfaceEhUnicoPorHostSoQuandoPreenchido(t *testing.T) {
	itf := interfaceDePoda(t)

	agora := time.Now().UTC()
	repetida := NetworkInterface{NetworkHostID: itf.NetworkHostID, IfIndex: 2, IfName: itf.IfName,
		OperStatus: "up", AdminStatus: "up", FirstSeen: agora, LastSeen: agora}
	if err := DB.Create(&repetida).Error; err == nil {
		t.Error("o banco aceitou duas interfaces com o mesmo nome no mesmo host")
	}

	for i := range 2 {
		semNome := NetworkInterface{NetworkHostID: itf.NetworkHostID, IfIndex: 10 + i,
			OperStatus: "up", AdminStatus: "up", FirstSeen: agora, LastSeen: agora}
		if err := DB.Create(&semNome).Error; err != nil {
			t.Errorf("interface sem nome %d recusada: %v", i, err)
		}
	}
}

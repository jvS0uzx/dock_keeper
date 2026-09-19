package discovery

import (
	"os"
	"testing"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/database"
)

const (
	testIPKnown   = "10.255.255.1"
	testIPUnnamed = "10.255.255.2"
)

func setupDB(t *testing.T) {
	t.Helper()

	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("DATABASE_URL não definido; pulando teste de persistência")
	}
	if database.DB == nil {
		if err := database.Connect(); err != nil {
			t.Skipf("banco indisponível: %v", err)
		}
	}
	clean(t)
	t.Cleanup(func() { clean(t) })
}

func clean(t *testing.T) {
	t.Helper()
	database.DB.Where("ip IN ?", []string{testIPKnown, testIPUnnamed}).Delete(&database.NetworkHost{})
}

func fetch(t *testing.T, ip string) database.NetworkHost {
	t.Helper()
	var host database.NetworkHost
	if err := database.DB.Where("ip = ?", ip).First(&host).Error; err != nil {
		t.Fatalf("host %s não encontrado: %v", ip, err)
	}
	return host
}

func TestPersistUpsertNaoDuplica(t *testing.T) {
	setupDB(t)

	persist([]Host{{IP: testIPKnown, Hostname: "PC-RH", MAC: "aa:bb:cc:dd:ee:ff", OpenPorts: []int{445, 3389}}}, nil)
	primeira := fetch(t, testIPKnown)

	time.Sleep(10 * time.Millisecond)

	persist([]Host{{IP: testIPKnown, Hostname: "PC-RH", MAC: "aa:bb:cc:dd:ee:ff", OpenPorts: []int{445, 3389, 22}}}, nil)

	var count int64
	database.DB.Model(&database.NetworkHost{}).Where("ip = ?", testIPKnown).Count(&count)
	if count != 1 {
		t.Fatalf("linhas para %s = %d, esperado 1", testIPKnown, count)
	}

	segunda := fetch(t, testIPKnown)
	if segunda.OpenPorts != "445,3389,22" {
		t.Errorf("open_ports = %q, esperado a lista nova", segunda.OpenPorts)
	}
	if !segunda.LastSeen.After(primeira.LastSeen) {
		t.Errorf("last_seen não avançou: %v -> %v", primeira.LastSeen, segunda.LastSeen)
	}
	if !segunda.FirstSeen.Equal(primeira.FirstSeen) {
		t.Errorf("first_seen foi sobrescrito: %v -> %v", primeira.FirstSeen, segunda.FirstSeen)
	}
}

func TestPersistNaoApagaNomeNemMACConhecidos(t *testing.T) {
	setupDB(t)

	persist([]Host{{IP: testIPKnown, Hostname: "PC-FINANCEIRO", MAC: "11:22:33:44:55:66", OpenPorts: []int{445}}}, nil)
	persist([]Host{{IP: testIPKnown, Hostname: "", MAC: "", OpenPorts: []int{445}}}, nil)

	host := fetch(t, testIPKnown)
	if host.Hostname != "PC-FINANCEIRO" {
		t.Errorf("hostname = %q, esperado o valor preservado", host.Hostname)
	}
	if host.MAC != "11:22:33:44:55:66" {
		t.Errorf("mac = %q, esperado o valor preservado", host.MAC)
	}
}

func TestPersistPreencheNomeDescobertoDepois(t *testing.T) {
	setupDB(t)

	persist([]Host{{IP: testIPUnnamed, OpenPorts: []int{22}}}, nil)
	if got := fetch(t, testIPUnnamed).Hostname; got != "" {
		t.Fatalf("hostname inicial = %q, esperado vazio", got)
	}

	persist([]Host{{IP: testIPUnnamed, Hostname: "nas-secao", MAC: "aa:aa:aa:bb:bb:bb", OpenPorts: []int{22}}}, nil)

	host := fetch(t, testIPUnnamed)
	if host.Hostname != "nas-secao" {
		t.Errorf("hostname = %q, esperado nas-secao", host.Hostname)
	}
	if host.MAC != "aa:aa:aa:bb:bb:bb" {
		t.Errorf("mac = %q", host.MAC)
	}
}

func TestPersistVazioNaoFazNada(t *testing.T) {
	setupDB(t)
	persist(nil, nil)
}

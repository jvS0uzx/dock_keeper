package discovery

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"
)

func TestExpandCIDRDescartaRedeEBroadcast(t *testing.T) {
	ips, err := ExpandCIDR("192.168.10.0/29")
	if err != nil {
		t.Fatalf("ExpandCIDR: %v", err)
	}
	// /29 são 8 endereços; sobram 6 utilizáveis.
	if len(ips) != 6 {
		t.Fatalf("len = %d, esperado 6: %v", len(ips), ips)
	}
	if ips[0] != "192.168.10.1" || ips[5] != "192.168.10.6" {
		t.Fatalf("faixa = %v", ips)
	}
}

func TestExpandCIDRAtravessaOctetos(t *testing.T) {
	ips, err := ExpandCIDR("10.0.0.0/23")
	if err != nil {
		t.Fatalf("ExpandCIDR: %v", err)
	}
	if len(ips) != 510 {
		t.Fatalf("len = %d, esperado 510", len(ips))
	}
	if ips[254] != "10.0.0.255" || ips[255] != "10.0.1.0" {
		t.Fatalf("virada de octeto errada: %s -> %s", ips[254], ips[255])
	}
}

// A varredura existe para inventariar a rede da própria seção. Recusar
// endereço público impede que o painel seja apontado para rede de terceiros.
func TestExpandCIDRRecusaFaixaPublica(t *testing.T) {
	for _, cidr := range []string{"8.8.8.0/24", "203.0.113.0/24"} {
		if _, err := ExpandCIDR(cidr); err == nil {
			t.Errorf("%s foi aceita", cidr)
		} else if !strings.Contains(err.Error(), "privada") {
			t.Errorf("%s: erro inesperado %v", cidr, err)
		}
	}
}

func TestExpandCIDRRecusaFaixaGigante(t *testing.T) {
	if _, err := ExpandCIDR("10.0.0.0/8"); err == nil {
		t.Fatal("/8 foi aceita; seriam 16 milhões de hosts")
	}
}

func TestExpandCIDRRecusaEntradaInvalida(t *testing.T) {
	for _, cidr := range []string{"nao-e-cidr", "192.168.1.1", "fd00::/64"} {
		if _, err := ExpandCIDR(cidr); err == nil {
			t.Errorf("%q foi aceita", cidr)
		}
	}
}

// Uma faixa inválida no meio da lista não pode abortar a varredura das outras.
func TestScanIgnoraFaixaInvalidaEContinua(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	port := listener.Addr().(*net.TCPAddr).Port
	hosts, errs := Scan(context.Background(), Config{
		CIDRs:   []string{"8.8.8.0/24", "127.0.0.1/32"},
		Ports:   []int{port},
		Timeout: 300 * time.Millisecond,
	})

	if len(errs) != 1 {
		t.Fatalf("erros = %v, esperado 1 (a faixa pública)", errs)
	}
	if len(hosts) != 1 || hosts[0].IP != "127.0.0.1" {
		t.Fatalf("hosts = %+v, esperado só 127.0.0.1", hosts)
	}
	if len(hosts[0].OpenPorts) != 1 || hosts[0].OpenPorts[0] != port {
		t.Fatalf("portas = %v, esperado [%d]", hosts[0].OpenPorts, port)
	}
}

func TestScanNaoRetornaHostSemPortaAberta(t *testing.T) {
	// Porta 1 em loopback não tem nada escutando.
	hosts, errs := Scan(context.Background(), Config{
		CIDRs:   []string{"127.0.0.1/32"},
		Ports:   []int{1},
		Timeout: 200 * time.Millisecond,
	})
	if len(errs) != 0 {
		t.Fatalf("erros = %v", errs)
	}
	if len(hosts) != 0 {
		t.Fatalf("hosts = %+v, esperado nenhum", hosts)
	}
}

func TestScanRespeitaCancelamento(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	started := time.Now()
	Scan(ctx, Config{CIDRs: []string{"10.10.0.0/24"}, Timeout: time.Second})

	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("varredura cancelada demorou %s", elapsed)
	}
}

func TestOrdenacaoPorEndereco(t *testing.T) {
	if !lessIP("192.168.1.9", "192.168.1.10") {
		t.Error("ordenação caiu para comparação de string: .9 deve vir antes de .10")
	}
	if lessIP("192.168.2.1", "192.168.1.1") {
		t.Error("ordem entre octetos errada")
	}
}

func TestJoinPorts(t *testing.T) {
	if got := joinPorts([]int{22, 3389}); got != "22,3389" {
		t.Errorf("joinPorts = %q", got)
	}
	if got := joinPorts(nil); got != "" {
		t.Errorf("joinPorts(nil) = %q", got)
	}
}

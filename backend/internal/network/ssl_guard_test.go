package network

import (
	"net"
	"strings"
	"testing"
	"time"
)

// A guarda desligada é o padrão e não pode mudar o fluxo atual: monitorar
// serviço da rede interna é o uso normal do painel auto-hospedado.
func TestGuardaDesligadaMantemFluxoAtual(t *testing.T) {
	agora := time.Now()
	ca, caKey := emitirCert(t, certOpts{
		commonName: "CA Teste", isCA: true,
		notBefore: agora.Add(-time.Hour), notAfter: agora.Add(24 * time.Hour),
	})
	leaf, leafKey := emitirCert(t, certOpts{
		commonName: "painel", ips: []net.IP{net.ParseIP("127.0.0.1")},
		notBefore: agora.Add(-time.Hour), notAfter: agora.Add(24 * time.Hour),
		parent: ca, parentKey: caKey,
	})
	host, port := servirTLS(t, leafKey, leaf)

	info := checkSSLGuarded(host, port, 5*time.Second, poolCom(ca), false)
	if !info.Valid {
		t.Fatalf("guarda desligada deveria manter o fluxo atual, veio %+v", info)
	}
}

func TestGuardaBloqueiaEnderecoPrivadoOuLocal(t *testing.T) {
	casos := []string{
		"127.0.0.1", "10.0.0.8", "172.16.0.1", "192.168.1.1",
		"169.254.10.10", "0.0.0.0", "::1", "fe80::1", "fd00::1", "::",
	}
	for _, host := range casos {
		info := checkSSLGuarded(host, 443, time.Second, nil, true)
		if info.Valid || info.InvalidReason != ReasonAlvoPrivado {
			t.Errorf("%s: esperava recusa %s, veio %+v", host, ReasonAlvoPrivado, info)
			continue
		}
		if !strings.Contains(info.ErrorMsg, "SSL_FORBID_PRIVATE_TARGETS") {
			t.Errorf("%s: mensagem não cita a variável que causou a recusa: %q", host, info.ErrorMsg)
		}
	}
}

// Nome que resolve para loopback é o caso real do ataque: o hostname parece
// inofensivo e o DNS aponta para dentro.
func TestGuardaBloqueiaNomeQueResolveParaLoopback(t *testing.T) {
	info := checkSSLGuarded("localhost", 443, 2*time.Second, nil, true)
	if info.Valid || info.InvalidReason != ReasonAlvoPrivado {
		t.Fatalf("localhost deveria ser recusado como %s, veio %+v", ReasonAlvoPrivado, info)
	}
}

func TestIPBloqueadoDistingueLocalDePublico(t *testing.T) {
	bloqueados := []string{"127.0.0.1", "10.1.2.3", "172.31.255.255", "192.168.0.1", "169.254.1.1", "0.0.0.0", "::1", "::", "fe80::1", "fd12::1"}
	for _, v := range bloqueados {
		if !ipBloqueado(net.ParseIP(v)) {
			t.Errorf("%s deveria ser bloqueado", v)
		}
	}
	publicos := []string{"8.8.8.8", "1.1.1.1", "203.0.113.25", "2001:4860:4860::8888"}
	for _, v := range publicos {
		if ipBloqueado(net.ParseIP(v)) {
			t.Errorf("%s é público e não deveria ser bloqueado", v)
		}
	}
}

// IP público literal passa pela guarda sem abrir conexão nenhuma na resolução.
func TestResolverAlvoDevolveIPPublicoParaDiscar(t *testing.T) {
	dial, bloqueado, err := resolverAlvo("8.8.8.8", time.Second)
	if err != nil || bloqueado != nil || dial != "8.8.8.8" {
		t.Fatalf("esperava dial=8.8.8.8 sem bloqueio, veio dial=%q bloqueado=%v err=%v", dial, bloqueado, err)
	}
}

// O dial vai no endereço informado e a verificação continua sendo do nome:
// é o que garante que a guarda pode conectar no IP conferido sem re-resolver.
func TestCheckSSLAtVerificaONomeENaoOEndereco(t *testing.T) {
	agora := time.Now()
	ca, caKey := emitirCert(t, certOpts{
		commonName: "CA Teste", isCA: true,
		notBefore: agora.Add(-time.Hour), notAfter: agora.Add(24 * time.Hour),
	})
	leaf, leafKey := emitirCert(t, certOpts{
		commonName: "painel.example", dnsNames: []string{"painel.example"},
		notBefore: agora.Add(-time.Hour), notAfter: agora.Add(24 * time.Hour),
		parent: ca, parentKey: caKey,
	})
	addr, port := servirTLS(t, leafKey, leaf)

	info := checkSSLAt("painel.example", addr, port, 5*time.Second, poolCom(ca))
	if !info.Valid {
		t.Fatalf("certificado cobre o nome verificado e a cadeia é confiável, veio %+v", info)
	}
}

func TestCheckSSLLeAGuardaDoAmbiente(t *testing.T) {
	t.Setenv("SSL_FORBID_PRIVATE_TARGETS", "true")
	if info := CheckSSL("127.0.0.1"); info.InvalidReason != ReasonAlvoPrivado {
		t.Fatalf("CheckSSL deveria aplicar a guarda lida do ambiente, veio %+v", info)
	}
}

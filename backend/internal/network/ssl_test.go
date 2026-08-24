package network

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

type certOpts struct {
	commonName string
	dnsNames   []string
	ips        []net.IP
	notBefore  time.Time
	notAfter   time.Time
	isCA       bool
	parent     *x509.Certificate
	parentKey  *ecdsa.PrivateKey
}

// emitirCert gera um certificado em memória. Assinado pelo par quando parent é
// informado, autoassinado quando não é.
func emitirCert(t *testing.T, o certOpts) (*x509.Certificate, *ecdsa.PrivateKey) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("gerar chave: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: o.commonName},
		NotBefore:             o.notBefore,
		NotAfter:              o.notAfter,
		DNSNames:              o.dnsNames,
		IPAddresses:           o.ips,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	if o.isCA {
		tmpl.IsCA = true
		tmpl.KeyUsage |= x509.KeyUsageCertSign
	}

	parent, parentKey := tmpl, key
	if o.parent != nil {
		parent, parentKey = o.parent, o.parentKey
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, parent, &key.PublicKey, parentKey)
	if err != nil {
		t.Fatalf("assinar certificado: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("reler certificado: %v", err)
	}
	return cert, key
}

// servirTLS sobe um servidor TLS local com a cadeia informada e devolve host e
// porta. A folha vem primeiro, como manda o protocolo.
func servirTLS(t *testing.T, key *ecdsa.PrivateKey, cadeia ...*x509.Certificate) (string, int) {
	t.Helper()

	par := tls.Certificate{PrivateKey: key}
	for _, c := range cadeia {
		par.Certificate = append(par.Certificate, c.Raw)
	}

	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{par}}
	srv.StartTLS()
	t.Cleanup(srv.Close)

	host, portStr, err := net.SplitHostPort(srv.Listener.Addr().String())
	if err != nil {
		t.Fatalf("endereço do servidor: %v", err)
	}
	port, _ := strconv.Atoi(portStr)
	return host, port
}

func poolCom(certs ...*x509.Certificate) *x509.CertPool {
	pool := x509.NewCertPool()
	for _, c := range certs {
		pool.AddCert(c)
	}
	return pool
}

func TestCheckSSLCadeiaConfiavel(t *testing.T) {
	agora := time.Now()
	ca, caKey := emitirCert(t, certOpts{
		commonName: "CA de Teste", isCA: true,
		notBefore: agora.Add(-time.Hour), notAfter: agora.Add(24 * time.Hour),
	})
	folha, folhaKey := emitirCert(t, certOpts{
		commonName: "host de teste", ips: []net.IP{net.ParseIP("127.0.0.1")},
		notBefore: agora.Add(-time.Hour), notAfter: agora.Add(90 * 24 * time.Hour),
		parent: ca, parentKey: caKey,
	})

	host, port := servirTLS(t, folhaKey, folha, ca)
	info := checkSSL(host, port, 5*time.Second, poolCom(ca))

	if !info.Valid {
		t.Fatalf("certificado íntegro reprovado: motivo=%q msg=%q", info.InvalidReason, info.ErrorMsg)
	}
	if info.InvalidReason != "" || info.ErrorMsg != "" {
		t.Errorf("certificado válido não devia trazer motivo: %+v", info)
	}
	if info.Issuer != "CA de Teste" {
		t.Errorf("emissor = %q", info.Issuer)
	}
}

func TestCheckSSLAutoassinadoEhInvalido(t *testing.T) {
	agora := time.Now()
	folha, folhaKey := emitirCert(t, certOpts{
		commonName: "interno.local", ips: []net.IP{net.ParseIP("127.0.0.1")},
		notBefore: agora.Add(-time.Hour), notAfter: agora.Add(60 * 24 * time.Hour),
	})

	host, port := servirTLS(t, folhaKey, folha)
	info := checkSSL(host, port, 5*time.Second, x509.NewCertPool())

	if info.Valid {
		t.Fatal("certificado autoassinado apareceu como válido")
	}
	if info.InvalidReason != ReasonAutoassinado {
		t.Errorf("motivo = %q, esperado %q (msg: %q)", info.InvalidReason, ReasonAutoassinado, info.ErrorMsg)
	}
	// O painel precisa exibir o certificado mesmo reprovado: a coleta continua.
	if info.Issuer != "interno.local" {
		t.Errorf("emissor não foi coletado: %q", info.Issuer)
	}
	if info.DaysLeft < 55 || info.DaysLeft > 60 {
		t.Errorf("dias restantes não foram coletados: %d", info.DaysLeft)
	}
}

func TestCheckSSLExpiradoTemPrecedenciaEListaOsDemais(t *testing.T) {
	agora := time.Now()
	folha, folhaKey := emitirCert(t, certOpts{
		commonName: "vencido.local", ips: []net.IP{net.ParseIP("127.0.0.1")},
		notBefore: agora.Add(-48 * time.Hour), notAfter: agora.Add(-24 * time.Hour),
	})

	host, port := servirTLS(t, folhaKey, folha)
	info := checkSSL(host, port, 5*time.Second, x509.NewCertPool())

	if info.Valid {
		t.Fatal("certificado expirado apareceu como válido")
	}
	if info.InvalidReason != ReasonExpirado {
		t.Errorf("motivo = %q, esperado %q", info.InvalidReason, ReasonExpirado)
	}
	if info.DaysLeft >= 0 {
		t.Errorf("dias restantes = %d, esperado negativo", info.DaysLeft)
	}
	// Um certificado com dois defeitos não pode perder o segundo na mensagem.
	if !strings.Contains(info.ErrorMsg, "autoassinado") {
		t.Errorf("mensagem perdeu o segundo problema: %q", info.ErrorMsg)
	}
}

func TestCheckSSLHostnameDivergente(t *testing.T) {
	agora := time.Now()
	ca, caKey := emitirCert(t, certOpts{
		commonName: "CA de Teste", isCA: true,
		notBefore: agora.Add(-time.Hour), notAfter: agora.Add(24 * time.Hour),
	})
	folha, folhaKey := emitirCert(t, certOpts{
		commonName: "outro.exemplo.com", dnsNames: []string{"outro.exemplo.com"},
		notBefore: agora.Add(-time.Hour), notAfter: agora.Add(90 * 24 * time.Hour),
		parent: ca, parentKey: caKey,
	})

	host, port := servirTLS(t, folhaKey, folha, ca)
	info := checkSSL(host, port, 5*time.Second, poolCom(ca))

	if info.Valid {
		t.Fatal("certificado de outro host apareceu como válido")
	}
	if info.InvalidReason != ReasonHostnameDivergente {
		t.Errorf("motivo = %q, esperado %q (msg: %q)", info.InvalidReason, ReasonHostnameDivergente, info.ErrorMsg)
	}
	if !strings.Contains(info.ErrorMsg, "outro.exemplo.com") {
		t.Errorf("mensagem não diz para quem o certificado foi emitido: %q", info.ErrorMsg)
	}
}

func TestCheckSSLCadeiaNaoConfiavel(t *testing.T) {
	agora := time.Now()
	ca, caKey := emitirCert(t, certOpts{
		commonName: "CA Desconhecida", isCA: true,
		notBefore: agora.Add(-time.Hour), notAfter: agora.Add(24 * time.Hour),
	})
	folha, folhaKey := emitirCert(t, certOpts{
		commonName: "host de teste", ips: []net.IP{net.ParseIP("127.0.0.1")},
		notBefore: agora.Add(-time.Hour), notAfter: agora.Add(90 * 24 * time.Hour),
		parent: ca, parentKey: caKey,
	})

	host, port := servirTLS(t, folhaKey, folha, ca)
	info := checkSSL(host, port, 5*time.Second, x509.NewCertPool())

	if info.Valid {
		t.Fatal("certificado de autoridade desconhecida apareceu como válido")
	}
	if info.InvalidReason != ReasonCadeiaNaoConfiavel {
		t.Errorf("motivo = %q, esperado %q (msg: %q)", info.InvalidReason, ReasonCadeiaNaoConfiavel, info.ErrorMsg)
	}
}

func TestCheckSSLFalhaDeConexao(t *testing.T) {
	if info := checkSSL("", 443, time.Second, nil); info.Valid || info.ErrorMsg != "Domínio vazio" {
		t.Errorf("domínio vazio = %+v", info)
	}

	// Porta fechada: erro de handshake, não classificação de certificado.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("abrir porta: %v", err)
	}
	_, portStr, _ := net.SplitHostPort(ln.Addr().String())
	port, _ := strconv.Atoi(portStr)
	ln.Close()

	info := checkSSL("127.0.0.1", port, time.Second, nil)
	if info.Valid || info.InvalidReason != ReasonHandshake {
		t.Errorf("porta fechada = %+v", info)
	}
}

func TestSSLPortaETimeoutParametrizados(t *testing.T) {
	if sslPort() != defaultSSLPort || sslTimeout() != defaultSSLTimeout {
		t.Fatal("sem variável de ambiente o padrão devia valer")
	}

	t.Setenv("SSL_CHECK_PORT", "8443")
	t.Setenv("SSL_CHECK_TIMEOUT", "12s")
	if got := sslPort(); got != 8443 {
		t.Errorf("SSL_CHECK_PORT = %d", got)
	}
	if got := sslTimeout(); got != 12*time.Second {
		t.Errorf("SSL_CHECK_TIMEOUT = %v", got)
	}

	// Valor sem sentido cai no padrão em vez de derrubar a verificação inteira.
	t.Setenv("SSL_CHECK_PORT", "99999")
	t.Setenv("SSL_CHECK_TIMEOUT", "amanhã")
	if sslPort() != defaultSSLPort || sslTimeout() != defaultSSLTimeout {
		t.Error("valor inválido devia cair no padrão")
	}
}

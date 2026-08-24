package network

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

// Motivos de invalidez do certificado. São códigos estáveis: ErrorMsg carrega o
// texto para o operador, que pode ser reescrito, e InvalidReason carrega a
// classificação, que outro código pode comparar.
const (
	ReasonExpirado           = "expirado"
	ReasonAindaNaoValido     = "ainda_nao_valido"
	ReasonHostnameDivergente = "hostname_divergente"
	ReasonAutoassinado       = "autoassinado"
	ReasonCadeiaNaoConfiavel = "cadeia_nao_confiavel"
	ReasonSemCertificado     = "sem_certificado"
	ReasonHandshake          = "handshake"
	// ReasonAlvoPrivado não fala do certificado: o alvo resolveu para endereço
	// privado ou local com SSL_FORBID_PRIVATE_TARGETS ativo e o handshake nem
	// chegou a abrir.
	ReasonAlvoPrivado = "alvo_privado_bloqueado"
)

const (
	defaultSSLPort    = 443
	defaultSSLTimeout = 5 * time.Second
)

type SSLInfo struct {
	Domain   string `json:"domain"`
	Valid    bool   `json:"valid"`
	Issuer   string `json:"issuer"`
	DaysLeft int    `json:"days_left"`
	// InvalidReason fica vazio quando Valid é verdadeiro. Quando o certificado
	// tem mais de um problema, guarda o mais grave; ErrorMsg lista todos.
	InvalidReason string `json:"invalid_reason,omitempty"`
	ErrorMsg      string `json:"error_msg,omitempty"`
}

// CheckSSL abre o handshake TLS no domínio e classifica o certificado servido.
func CheckSSL(domain string) SSLInfo {
	return checkSSLGuarded(domain, sslPort(), sslTimeout(), sslRoots(), sslForbidPrivate())
}

// checkSSLGuarded aplica a guarda de alvo privado antes do handshake. Quando a
// guarda barra o alvo, a conexão nem é aberta: o objetivo é impedir que a tela
// de SSL vire sonda da rede onde o painel roda (SSRF), não classificar
// certificado.
func checkSSLGuarded(host string, port int, timeout time.Duration, roots *x509.CertPool, forbidPrivate bool) SSLInfo {
	host = strings.TrimSpace(host)
	if !forbidPrivate || host == "" {
		return checkSSL(host, port, timeout, roots)
	}
	dial, bloqueado, err := resolverAlvo(host, timeout)
	if err != nil {
		return SSLInfo{Domain: host, Valid: false, InvalidReason: ReasonHandshake, ErrorMsg: "Falha ao resolver o domínio: " + err.Error()}
	}
	if bloqueado != nil {
		return SSLInfo{
			Domain:        host,
			Valid:         false,
			InvalidReason: ReasonAlvoPrivado,
			ErrorMsg:      fmt.Sprintf("Alvo bloqueado: %s resolve para endereço privado ou local (%s) e SSL_FORBID_PRIVATE_TARGETS está ativo", host, bloqueado),
		}
	}
	// Disca no IP que acabou de ser validado, não no nome: re-resolver dentro
	// do handshake abriria a janela para o DNS trocar a resposta (rebinding).
	return checkSSLAt(host, dial, port, timeout, roots)
}

// checkSSL recebe as raízes explicitamente para o teste conseguir montar uma
// cadeia confiável sem depender do truststore da máquina que roda o teste.
func checkSSL(host string, port int, timeout time.Duration, roots *x509.CertPool) SSLInfo {
	return checkSSLAt(host, host, port, timeout, roots)
}

// checkSSLAt separa o nome verificado (host) do endereço discado (dialHost).
// No fluxo normal os dois são iguais; com a guarda de alvo privado ligada o
// dial vai no IP já conferido e o nome segue sendo o que o certificado precisa
// cobrir.
func checkSSLAt(host, dialHost string, port int, timeout time.Duration, roots *x509.CertPool) SSLInfo {
	host = strings.TrimSpace(host)
	if host == "" {
		return SSLInfo{Valid: false, InvalidReason: ReasonHandshake, ErrorMsg: "Domínio vazio"}
	}

	// InsecureSkipVerify é proposital e a verificação vem logo abaixo, campo a
	// campo. O handshake precisa concluir mesmo com certificado ruim: a tela
	// existe para mostrar o problema, e recusar a conexão esconderia justamente
	// o certificado que o operador precisa ver.
	dialer := &net.Dialer{Timeout: timeout}
	conn, err := tls.DialWithDialer(dialer, "tcp", net.JoinHostPort(dialHost, strconv.Itoa(port)), &tls.Config{
		InsecureSkipVerify: true,
		ServerName:         host,
	})
	if err != nil {
		return SSLInfo{Domain: host, Valid: false, InvalidReason: ReasonHandshake, ErrorMsg: err.Error()}
	}
	defer conn.Close()

	certs := conn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return SSLInfo{Domain: host, Valid: false, InvalidReason: ReasonSemCertificado, ErrorMsg: "Nenhum certificado recebido"}
	}

	leaf := certs[0]
	now := time.Now()
	info := SSLInfo{
		Domain:   host,
		Issuer:   issuerName(leaf),
		DaysLeft: int(leaf.NotAfter.Sub(now).Hours() / 24),
	}

	// O servidor manda folha e intermediárias, nunca a raiz. Sem juntar as
	// intermediárias num pool, cadeia legítima falharia por falta de elo.
	intermediates := x509.NewCertPool()
	for _, c := range certs[1:] {
		intermediates.AddCert(c)
	}
	selfSigned := bytes.Equal(leaf.RawIssuer, leaf.RawSubject)

	// Os problemas são coletados em ordem de gravidade decrescente: validade
	// vencida é o que urge, hostname divergente é o que o usuário final já está
	// vendo no navegador, e a cadeia vem por último por ser decisão de infra.
	var reasons, problems []string

	switch {
	case now.After(leaf.NotAfter):
		reasons = append(reasons, ReasonExpirado)
		problems = append(problems, "expirado em "+leaf.NotAfter.Format("02/01/2006"))
	case now.Before(leaf.NotBefore):
		reasons = append(reasons, ReasonAindaNaoValido)
		problems = append(problems, "só passa a valer em "+leaf.NotBefore.Format("02/01/2006"))
	}

	if err := leaf.VerifyHostname(host); err != nil {
		reasons = append(reasons, ReasonHostnameDivergente)
		problems = append(problems, fmt.Sprintf("não cobre o host %s (emitido para %s)", host, strings.Join(certNames(leaf), ", ")))
	}

	if _, err := leaf.Verify(x509.VerifyOptions{
		Roots:         roots,
		Intermediates: intermediates,
		CurrentTime:   now,
	}); err != nil {
		var invalid x509.CertificateInvalidError
		switch {
		case selfSigned:
			reasons = append(reasons, ReasonAutoassinado)
			problems = append(problems, "autoassinado: o emissor é o próprio titular")
		case errors.As(err, &invalid) && invalid.Reason == x509.Expired:
			// A validade já entrou na lista acima; repetir aqui como falha de
			// cadeia diria que o problema é a autoridade, e não é.
		default:
			reasons = append(reasons, ReasonCadeiaNaoConfiavel)
			problems = append(problems, "cadeia não confiável: "+err.Error())
		}
	}

	if len(reasons) > 0 {
		info.InvalidReason = reasons[0]
		info.ErrorMsg = "Certificado " + strings.Join(problems, "; ")
	}
	info.Valid = len(reasons) == 0
	return info
}

func issuerName(cert *x509.Certificate) string {
	if cert.Issuer.CommonName != "" {
		return cert.Issuer.CommonName
	}
	if len(cert.Issuer.Organization) > 0 {
		return cert.Issuer.Organization[0]
	}
	return "Desconhecido"
}

// certNames devolve para quem o certificado foi emitido, para a mensagem de
// hostname divergente dizer qual é o host certo em vez de só negar o errado.
func certNames(cert *x509.Certificate) []string {
	names := append([]string{}, cert.DNSNames...)
	for _, ip := range cert.IPAddresses {
		names = append(names, ip.String())
	}
	if len(names) == 0 && cert.Subject.CommonName != "" {
		names = append(names, cert.Subject.CommonName)
	}
	if len(names) == 0 {
		return []string{"nenhum nome"}
	}
	return names
}

func sslPort() int {
	if v, err := strconv.Atoi(strings.TrimSpace(os.Getenv("SSL_CHECK_PORT"))); err == nil && v > 0 && v <= 65535 {
		return v
	}
	return defaultSSLPort
}

func sslTimeout() time.Duration {
	if d, err := time.ParseDuration(strings.TrimSpace(os.Getenv("SSL_CHECK_TIMEOUT"))); err == nil && d > 0 {
		return d
	}
	return defaultSSLTimeout
}

// sslRoots acrescenta as CAs internas da instalação às do sistema. Sem essa
// saída, todo serviço atrás de CA própria — o normal em rede interna — passaria
// a aparecer como cadeia não confiável assim que a verificação real entrou.
func sslRoots() *x509.CertPool {
	path := strings.TrimSpace(os.Getenv("SSL_EXTRA_CA"))
	if path == "" {
		return nil
	}
	pem, err := os.ReadFile(path)
	if err != nil {
		log.Printf("[SSL] erro ao ler SSL_EXTRA_CA (%s): %v", path, err)
		return nil
	}
	pool, err := x509.SystemCertPool()
	if err != nil {
		pool = x509.NewCertPool()
	}
	if !pool.AppendCertsFromPEM(pem) {
		log.Printf("[SSL] SSL_EXTRA_CA (%s) não contém nenhum certificado PEM válido", path)
	}
	return pool
}

// sslForbidPrivate lê SSL_FORBID_PRIVATE_TARGETS. Desligada por padrão porque
// o painel é auto-hospedado e monitorar serviço da rede interna é o uso normal
// da tela de SSL. A guarda existe para quem expõe o painel a vários
// operadores, cenário em que o cadastro de domínio viraria sonda da rede do
// servidor.
func sslForbidPrivate() bool {
	v, _ := strconv.ParseBool(strings.TrimSpace(os.Getenv("SSL_FORBID_PRIVATE_TARGETS")))
	return v
}

// ipBloqueado diz se o endereço pertence à máquina ou à rede local: privado
// (RFC 1918 e fc00::/7), loopback, link-local ou não especificado.
func ipBloqueado(ip net.IP) bool {
	return ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsUnspecified()
}

// resolverAlvo resolve o host e aplica a política da guarda: basta UM endereço
// privado para recusar, porque quem cadastra o nome controla o DNS dele e uma
// resposta mista é exatamente a forma do ataque.
func resolverAlvo(host string, timeout time.Duration) (dial string, bloqueado net.IP, err error) {
	if ip := net.ParseIP(host); ip != nil {
		if ipBloqueado(ip) {
			return "", ip, nil
		}
		return host, nil, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return "", nil, err
	}
	if len(addrs) == 0 {
		return "", nil, errors.New("nenhum endereço resolvido")
	}
	for _, a := range addrs {
		if ipBloqueado(a.IP) {
			return "", a.IP, nil
		}
	}
	return addrs[0].IP.String(), nil, nil
}

package network

import (
	"crypto/tls"
	"net"
	"strings"
	"time"
)

type SSLInfo struct {
	Domain   string `json:"domain"`
	Valid    bool   `json:"valid"`
	Issuer   string `json:"issuer"`
	DaysLeft int    `json:"days_left"`
	ErrorMsg string `json:"error_msg,omitempty"`
}

func CheckSSL(domain string) SSLInfo {
	domain = strings.TrimSpace(domain)
	if domain == "" {
		return SSLInfo{ErrorMsg: "Domínio vazio"}
	}

	dialer := &net.Dialer{Timeout: 5 * time.Second}
	conn, err := tls.DialWithDialer(dialer, "tcp", domain+":443", &tls.Config{
		InsecureSkipVerify: true, // We still verify manually to catch expiration
	})

	if err != nil {
		return SSLInfo{Domain: domain, Valid: false, ErrorMsg: err.Error()}
	}
	defer conn.Close()

	certs := conn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return SSLInfo{Domain: domain, Valid: false, ErrorMsg: "Nenhum certificado recebido"}
	}

	cert := certs[0]
	daysLeft := int(time.Until(cert.NotAfter).Hours() / 24)

	// Issuer Common Name
	issuer := cert.Issuer.CommonName
	if issuer == "" {
		if len(cert.Issuer.Organization) > 0 {
			issuer = cert.Issuer.Organization[0]
		} else {
			issuer = "Desconhecido"
		}
	}

	return SSLInfo{
		Domain:   domain,
		Valid:    daysLeft > 0,
		Issuer:   issuer,
		DaysLeft: daysLeft,
	}
}

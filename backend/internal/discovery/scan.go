// Package discovery inventaria os hosts ativos de uma rede local.
//
// A varredura é um TCP connect scan: para cada IP da faixa tenta abrir conexão
// numa lista curta de portas comuns. Quem aceita (ou recusa explicitamente com
// RST) está ligado. Não usa ICMP nem ARP cru porque os dois exigem socket raw,
// ou seja, root — e o painel não precisa disso para inventariar.
package discovery

import (
	"context"
	"fmt"
	"net"
	"sort"
	"sync"
	"time"
)

// Portas sondadas por host. Lista curta de propósito: cobre estação Windows
// (445/3389), Linux (22), impressora (515/631/9100), NAS (5000) e o próprio
// painel.
//
// Precisa conter TODA porta que fingerprint.go usa para classificar. Ela não
// continha 515, 631 nem 5000, então a varredura local nunca sondava o que a
// própria tabela de classificação consultava: impressora que só publica 631
// caía como "web-device", e NAS como estação Windows por causa do 445. O
// coletor remoto já sondava as doze — a divergência aparecia como o mesmo
// equipamento mudando de tipo conforme quem o encontrasse.
var DefaultPorts = []int{22, 80, 135, 139, 443, 445, 515, 631, 3389, 5000, 8080, 9100}

const (
	DefaultTimeout     = 400 * time.Millisecond
	DefaultConcurrency = 256

	// Uma faixa maior que /16 são 65 mil hosts: varredura longa demais para o
	// caso de uso (uma seção da rede) e provavelmente erro de digitação.
	minPrefixLen = 16
)

// Host é um endereço que respondeu à varredura.
type Host struct {
	IP        string
	Hostname  string
	MAC       string
	OpenPorts []int
}

// Config parametriza uma varredura.
type Config struct {
	CIDRs       []string
	Ports       []int
	Timeout     time.Duration
	Concurrency int
}

func (c Config) withDefaults() Config {
	if len(c.Ports) == 0 {
		c.Ports = DefaultPorts
	}
	if c.Timeout <= 0 {
		c.Timeout = DefaultTimeout
	}
	if c.Concurrency <= 0 {
		c.Concurrency = DefaultConcurrency
	}
	return c
}

// ExpandCIDR devolve os endereços utilizáveis da faixa.
//
// Só aceita faixa privada (RFC1918 / CGNAT / link-local): este recurso existe
// para inventariar a rede da própria seção, e recusar endereço público impede
// que o painel seja apontado para redes de terceiros.
func ExpandCIDR(cidr string) ([]string, error) {
	ip, network, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, fmt.Errorf("faixa inválida %q: %w", cidr, err)
	}
	if ip.To4() == nil {
		return nil, fmt.Errorf("faixa %q: só IPv4 é suportado", cidr)
	}
	if !isPrivate(network.IP) {
		return nil, fmt.Errorf("faixa %q não é privada; a varredura só cobre a rede interna", cidr)
	}

	ones, bits := network.Mask.Size()
	if ones < minPrefixLen {
		return nil, fmt.Errorf("faixa %q é grande demais (máximo /%d)", cidr, minPrefixLen)
	}

	var ips []string
	for addr := network.IP.Mask(network.Mask); network.Contains(addr); addr = nextIP(addr) {
		ips = append(ips, addr.String())
	}

	// Em /31 e /32 todo endereço é utilizável; nas demais o primeiro é a rede
	// e o último é broadcast.
	if ones < bits-1 && len(ips) > 2 {
		ips = ips[1 : len(ips)-1]
	}
	return ips, nil
}

func isPrivate(ip net.IP) bool {
	return ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLoopback()
}

// nextIP devolve uma cópia do endereço seguinte, sem alterar o original.
func nextIP(ip net.IP) net.IP {
	next := make(net.IP, len(ip))
	copy(next, ip)
	for i := len(next) - 1; i >= 0; i-- {
		next[i]++
		if next[i] != 0 {
			break
		}
	}
	return next
}

// Scan varre todas as faixas e devolve os hosts que responderam, ordenados por
// endereço. Uma faixa inválida não aborta a varredura das outras.
func Scan(ctx context.Context, cfg Config) ([]Host, []error) {
	cfg = cfg.withDefaults()

	var targets []string
	var errs []error
	for _, cidr := range cfg.CIDRs {
		ips, err := ExpandCIDR(cidr)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		targets = append(targets, ips...)
	}
	if len(targets) == 0 {
		return nil, errs
	}

	var (
		mu    sync.Mutex
		found []Host
		wg    sync.WaitGroup
	)
	sem := make(chan struct{}, cfg.Concurrency)

	for _, ip := range targets {
		if ctx.Err() != nil {
			break
		}
		wg.Add(1)
		go func(ip string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			ports := probe(ctx, ip, cfg.Ports, cfg.Timeout)
			if len(ports) == 0 {
				return
			}

			host := Host{IP: ip, OpenPorts: ports}
			host.Hostname = reverseDNS(ctx, ip)

			mu.Lock()
			found = append(found, host)
			mu.Unlock()
		}(ip)
	}
	wg.Wait()

	// A tabela ARP é lida depois: foram as conexões TCP desta varredura que a
	// preencheram. Lendo antes, o MAC de um host novo só apareceria no ciclo
	// seguinte.
	arp := arpTable()
	for i := range found {
		found[i].MAC = arp[found[i].IP]
	}

	sort.Slice(found, func(i, j int) bool { return lessIP(found[i].IP, found[j].IP) })
	return found, errs
}

// probe testa as portas do host e devolve as que aceitaram conexão.
func probe(ctx context.Context, ip string, ports []int, timeout time.Duration) []int {
	var open []int
	dialer := net.Dialer{Timeout: timeout}

	for _, port := range ports {
		if ctx.Err() != nil {
			break
		}
		conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(ip, fmt.Sprint(port)))
		if err != nil {
			continue
		}
		conn.Close()
		open = append(open, port)
	}
	return open
}

// reverseDNS resolve o nome do host, com prazo curto: numa rede sem DNS
// reverso cada consulta esperaria o timeout inteiro do resolver.
func reverseDNS(ctx context.Context, ip string) string {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	names, err := net.DefaultResolver.LookupAddr(ctx, ip)
	if err != nil || len(names) == 0 {
		return ""
	}
	return trimDot(names[0])
}

func trimDot(s string) string {
	if len(s) > 0 && s[len(s)-1] == '.' {
		return s[:len(s)-1]
	}
	return s
}

func lessIP(a, b string) bool {
	ipA, ipB := net.ParseIP(a).To4(), net.ParseIP(b).To4()
	if ipA == nil || ipB == nil {
		return a < b
	}
	for i := range ipA {
		if ipA[i] != ipB[i] {
			return ipA[i] < ipB[i]
		}
	}
	return false
}

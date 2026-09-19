package main

import (
	"net"
	"sort"
)

func enderecosDoHost(interfaces func() ([]net.Interface, error)) []string {
	lista, err := interfaces()
	if err != nil {
		return nil
	}

	vistos := map[string]bool{}
	for _, iface := range lista {
		if iface.Flags&net.FlagUp == 0 || ehInterfaceVirtual(iface.Name) {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ip := ipDeEndereco(addr)
			if ip == "" {
				continue
			}
			vistos[ip] = true
		}
	}

	fora := make([]string, 0, len(vistos))
	for ip := range vistos {
		fora = append(fora, ip)
	}
	sort.Strings(fora)
	return fora
}

func ipDeEndereco(addr net.Addr) string {
	var ip net.IP
	switch v := addr.(type) {
	case *net.IPNet:
		ip = v.IP
	case *net.IPAddr:
		ip = v.IP
	default:
		return ""
	}
	if ip == nil || ip.To4() == nil {
		return ""
	}
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
		return ""
	}
	return ip.String()
}

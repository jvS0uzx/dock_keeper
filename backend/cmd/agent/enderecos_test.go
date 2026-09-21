package main

import (
	"errors"
	"net"
	"testing"
)

type enderecoFalso struct{ ip string }

func (e enderecoFalso) Network() string { return "ip+net" }
func (e enderecoFalso) String() string  { return e.ip }

func TestEnderecosDoHostIgnoraLoopbackEVirtuais(t *testing.T) {
	interfaces := func() ([]net.Interface, error) {
		return []net.Interface{
			{Index: 1, Name: "lo", Flags: net.FlagUp},
			{Index: 2, Name: "eth0", Flags: net.FlagUp},
			{Index: 3, Name: "docker0", Flags: net.FlagUp},
			{Index: 4, Name: "tailscale0", Flags: net.FlagUp},
		}, nil
	}

	lista := enderecosDoHost(interfaces)
	for _, indesejado := range []string{"127.0.0.1", "172.17.0.1"} {
		for _, achado := range lista {
			if achado == indesejado {
				t.Errorf("endereço %q não deveria sair: %v", indesejado, lista)
			}
		}
	}
}

func TestEnderecosDoHostComErroDevolveVazio(t *testing.T) {
	lista := enderecosDoHost(func() ([]net.Interface, error) {
		return nil, errors.New("sem permissão")
	})
	if len(lista) != 0 {
		t.Errorf("com erro na leitura a lista deve ficar vazia, veio %v", lista)
	}
}

func TestIpDeEnderecoDescartaIPv6ELoopback(t *testing.T) {
	casos := map[string]string{
		"10.0.0.5/24":     "10.0.0.5",
		"127.0.0.1/8":     "",
		"169.254.1.1/16":  "",
		"fe80::1/64":      "",
		"100.100.0.11/32": "100.100.0.11",
	}
	for entrada, esperado := range casos {
		ip, rede, err := net.ParseCIDR(entrada)
		if err != nil {
			t.Fatalf("fixture %q: %v", entrada, err)
		}
		rede.IP = ip
		if achado := ipDeEndereco(rede); achado != esperado {
			t.Errorf("ipDeEndereco(%q) = %q, esperado %q", entrada, achado, esperado)
		}
	}
}

func TestEnderecosDoHostLeOSistemaDeVerdade(t *testing.T) {
	lista := enderecosDoHost(net.Interfaces)
	for _, ip := range lista {
		if net.ParseIP(ip) == nil {
			t.Errorf("leitura real devolveu %q, que não é IP", ip)
		}
	}
}

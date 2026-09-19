package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/shirou/gopsutil/v3/net"
)

type leituraFalsa struct {
	rx, tx uint64
	err    error
}

func medidorDeTeste(leituras []leituraFalsa, passo time.Duration) *medidorDeRede {
	inicio := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	i := 0
	return &medidorDeRede{
		ler: func() (uint64, uint64, error) {
			l := leituras[i]
			return l.rx, l.tx, l.err
		},
		agora: func() time.Time {
			t := inicio.Add(time.Duration(i) * passo)
			i++
			return t
		},
	}
}

func valor(p *float64) string {
	if p == nil {
		return "nil"
	}
	return fmt.Sprintf("%g", *p)
}

func TestSomaIgnoraLoopback(t *testing.T) {
	rx, tx := somaSemLoopback([]net.IOCountersStat{
		{Name: "lo", BytesRecv: 1000, BytesSent: 1000},
		{Name: "lo0", BytesRecv: 1000, BytesSent: 1000},
		{Name: "Loopback Pseudo-Interface 1", BytesRecv: 1000, BytesSent: 1000},
		{Name: "eth0", BytesRecv: 10, BytesSent: 20},
		{Name: "wlan0", BytesRecv: 5, BytesSent: 7},
	})
	if rx != 15 || tx != 27 {
		t.Errorf("soma = (%d, %d), esperado (15, 27)", rx, tx)
	}
}

func TestSomaIgnoraInterfacesVirtuaisDoDocker(t *testing.T) {
	rx, tx := somaSemLoopback([]net.IOCountersStat{
		{Name: "eth0", BytesRecv: 100, BytesSent: 200},
		{Name: "veth123", BytesRecv: 1000, BytesSent: 1000},
		{Name: "docker0", BytesRecv: 1000, BytesSent: 1000},
		{Name: "br-abc", BytesRecv: 1000, BytesSent: 1000},
		{Name: "virbr0", BytesRecv: 1000, BytesSent: 1000},
		{Name: "lo", BytesRecv: 1000, BytesSent: 1000},
		{Name: "Loopback Pseudo-Interface 1", BytesRecv: 1000, BytesSent: 1000},
		{Name: "Local Area Connection", BytesRecv: 3, BytesSent: 4},
	})
	if rx != 103 || tx != 204 {
		t.Errorf("soma = (%d, %d), esperado (103, 204): só eth0 e o adaptador do Windows contam", rx, tx)
	}
}

func TestPrimeiroCicloNaoEnviaTaxa(t *testing.T) {
	m := medidorDeTeste([]leituraFalsa{{rx: 1000, tx: 500}}, 2*time.Second)
	if rx, tx := m.taxas(); rx != nil || tx != nil {
		t.Errorf("primeiro ciclo = (%s, %s), esperado sem taxa", valor(rx), valor(tx))
	}
}

func TestTaxaEhDeltaPorSegundo(t *testing.T) {
	m := medidorDeTeste([]leituraFalsa{{rx: 1000, tx: 500}, {rx: 3048, tx: 1524}}, 2*time.Second)
	m.taxas()
	rx, tx := m.taxas()
	if rx == nil || tx == nil || *rx != 1024 || *tx != 512 {
		t.Errorf("taxas = (%s, %s), esperado (1024, 512)", valor(rx), valor(tx))
	}
}

func TestContadorQueVoltaNaoGeraTaxaNegativa(t *testing.T) {
	m := medidorDeTeste([]leituraFalsa{{rx: 5000, tx: 500}, {rx: 100, tx: 700}, {rx: 300, tx: 900}}, time.Second)
	m.taxas()
	rx, tx := m.taxas()
	if rx != nil {
		t.Errorf("contador de RX que voltou gerou taxa %s", valor(rx))
	}
	if tx == nil || *tx != 200 {
		t.Errorf("TX independente = %s, esperado 200", valor(tx))
	}
	rx, _ = m.taxas()
	if rx == nil || *rx != 200 {
		t.Errorf("RX depois do reinício do contador = %s, esperado 200", valor(rx))
	}
}

func TestErroDeLeituraNaoEnviaTaxa(t *testing.T) {
	m := medidorDeTeste([]leituraFalsa{
		{rx: 1000, tx: 1000},
		{err: errors.New("sem interfaces")},
		{rx: 4000, tx: 4000},
	}, time.Second)
	m.taxas()
	if rx, tx := m.taxas(); rx != nil || tx != nil {
		t.Errorf("leitura com erro = (%s, %s), esperado sem taxa", valor(rx), valor(tx))
	}
	rx, _ := m.taxas()
	if rx == nil || *rx != 1500 {
		t.Errorf("taxa depois do erro = %s, esperado 1500 (3000 B em 2 s)", valor(rx))
	}
}

func TestPayloadOmiteTaxaAusente(t *testing.T) {
	sem, _ := json.Marshal(metricsPayload{})
	if strings.Contains(string(sem), "net_rx_bps") || strings.Contains(string(sem), "net_tx_bps") {
		t.Errorf("payload sem taxa trouxe o campo: %s", sem)
	}

	rx, tx := 1024.0, 0.0
	com, _ := json.Marshal(metricsPayload{NetRxBps: &rx, NetTxBps: &tx})
	if !strings.Contains(string(com), `"net_rx_bps":1024`) || !strings.Contains(string(com), `"net_tx_bps":0`) {
		t.Errorf("payload com taxa = %s", com)
	}
}

func TestFormatTaxa(t *testing.T) {
	if got := formatTaxa(nil); got != "-" {
		t.Errorf("sem taxa = %q", got)
	}
	v := 1536.4
	if got := formatTaxa(&v); got != "1536B/s" {
		t.Errorf("1536.4 = %q", got)
	}
}

func TestMedidorRealLeContadoresDoSistema(t *testing.T) {
	m := novoMedidorDeRede()
	if rx, tx := m.taxas(); rx != nil || tx != nil {
		t.Fatalf("primeira leitura real devolveu taxa (%s, %s)", valor(rx), valor(tx))
	}
	time.Sleep(20 * time.Millisecond)
	if rx, tx := m.taxas(); rx == nil || tx == nil || *rx < 0 || *tx < 0 {
		t.Errorf("segunda leitura real = (%s, %s), esperado taxa não negativa", valor(rx), valor(tx))
	}
}

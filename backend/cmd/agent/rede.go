package main

import (
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/net"
)

type amostraDeRede struct {
	rx, tx uint64
	em     time.Time
}

type medidorDeRede struct {
	ler   func() (rx, tx uint64, err error)
	agora func() time.Time
	antes *amostraDeRede
}

func novoMedidorDeRede() *medidorDeRede {
	return &medidorDeRede{ler: lerContadoresDeRede, agora: time.Now}
}

func lerContadoresDeRede() (uint64, uint64, error) {
	stats, err := net.IOCounters(true)
	if err != nil {
		return 0, 0, err
	}
	rx, tx := somaSemLoopback(stats)
	return rx, tx, nil
}

var prefixosVirtuais = []string{"lo", "veth", "docker", "br-", "virbr"}

func ehInterfaceVirtual(nome string) bool {
	if strings.HasPrefix(strings.ToLower(nome), "loopback") {
		return true
	}
	for _, p := range prefixosVirtuais {
		if strings.HasPrefix(nome, p) {
			return true
		}
	}
	return false
}

func somaSemLoopback(stats []net.IOCountersStat) (rx, tx uint64) {
	for _, s := range stats {
		if ehInterfaceVirtual(s.Name) {
			continue
		}
		rx += s.BytesRecv
		tx += s.BytesSent
	}
	return rx, tx
}

func (m *medidorDeRede) taxas() (rx, tx *float64) {
	lidoRx, lidoTx, err := m.ler()
	em := m.agora()
	if err != nil {
		return nil, nil
	}

	anterior := m.antes
	m.antes = &amostraDeRede{rx: lidoRx, tx: lidoTx, em: em}
	if anterior == nil {
		return nil, nil
	}

	segundos := em.Sub(anterior.em).Seconds()
	if segundos <= 0 {
		return nil, nil
	}
	return taxa(anterior.rx, lidoRx, segundos), taxa(anterior.tx, lidoTx, segundos)
}

func taxa(antes, agora uint64, segundos float64) *float64 {
	if agora < antes {
		return nil
	}
	v := float64(agora-antes) / segundos
	return &v
}

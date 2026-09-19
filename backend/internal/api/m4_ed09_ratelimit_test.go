package api

import (
	"fmt"
	"testing"
	"time"
)

func TestLimiteDeChavesDoRateLimiter(t *testing.T) {
	l := newLoginLimiter(15*time.Minute, 30, 8)
	l.maxKeys = 200

	for i := 0; i < 5000; i++ {
		l.fail(fmt.Sprintf("203.0.113.%d", i), fmt.Sprintf("usuario-%d", i))
	}

	l.mu.Lock()
	tamanho := len(l.failures)
	l.mu.Unlock()

	if tamanho > l.maxKeys {
		t.Errorf("o limitador guardou %d chaves com teto de %d — memória cresce sem limite", tamanho, l.maxKeys)
	}
}

func TestPodaDoLimitadorPreservaQuemEstaBloqueado(t *testing.T) {
	l := newLoginLimiter(15*time.Minute, 3, 2)
	l.maxKeys = 50

	for i := 0; i < 3; i++ {
		l.fail("198.51.100.7", "alvo")
	}
	if l.allowed("198.51.100.7", "alvo") {
		t.Fatal("o IP deveria estar bloqueado antes da poda")
	}

	for i := 0; i < 500; i++ {
		l.fail(fmt.Sprintf("203.0.113.%d", i), fmt.Sprintf("ruido-%d", i))
	}

	if l.allowed("198.51.100.7", "alvo") {
		t.Error("a poda liberou um IP que estava bloqueado na janela")
	}
}

func TestChavesVelhasSaemNaPoda(t *testing.T) {
	agora := time.Now()
	l := newLoginLimiter(15*time.Minute, 30, 8)
	l.maxKeys = 130
	l.now = func() time.Time { return agora }

	for i := 0; i < 60; i++ {
		l.fail(fmt.Sprintf("192.0.2.%d", i), fmt.Sprintf("antigo-%d", i))
	}

	agora = agora.Add(30 * time.Minute)
	for i := 0; i < 60; i++ {
		l.fail(fmt.Sprintf("198.51.100.%d", i), fmt.Sprintf("novo-%d", i))
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	if _, existe := l.failures[ipKey("192.0.2.1")]; existe {
		t.Error("chave fora da janela continuou no mapa depois da poda")
	}
	if _, existe := l.failures[ipKey("198.51.100.1")]; !existe {
		t.Error("a poda removeu uma chave dentro da janela")
	}
}

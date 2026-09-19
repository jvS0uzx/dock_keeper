package api

import (
	"log"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/config"
)

const (
	defaultLoginWindow     = 15 * time.Minute
	defaultLoginMaxPerIP   = 30
	defaultLoginMaxPerUser = 8
	defaultLoginMaxKeys    = 10000
)

type loginLimiter struct {
	mu       sync.Mutex
	failures map[string][]time.Time

	window  time.Duration
	maxIP   int
	maxUser int
	maxKeys int

	now func() time.Time
}

func newLoginLimiter(window time.Duration, maxIP, maxUser int) *loginLimiter {
	return &loginLimiter{
		failures: make(map[string][]time.Time),
		window:   window,
		maxIP:    maxIP,
		maxUser:  maxUser,
		maxKeys:  config.Inteiro("LOGIN_RATE_MAX_KEYS", defaultLoginMaxKeys),
		now:      time.Now,
	}
}

func ipKey(ip string) string     { return "ip:" + ip }
func userKey(name string) string { return "user:" + normalizeUsername(name) }

func normalizeUsername(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func (l *loginLimiter) allowed(ip, username string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.countLocked(ipKey(ip)) < l.maxIP && l.countLocked(userKey(username)) < l.maxUser
}

func (l *loginLimiter) fail(ip, username string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.failures[ipKey(ip)] = append(l.failures[ipKey(ip)], now)
	l.failures[userKey(username)] = append(l.failures[userKey(username)], now)

	if len(l.failures[ipKey(ip)]) == l.maxIP {
		log.Printf("[Auth] %s atingiu o teto de %d tentativas de login na janela", ip, l.maxIP)
	}
	if len(l.failures[userKey(username)]) == l.maxUser {
		log.Printf("[Auth] a conta %q atingiu o teto de %d tentativas de login na janela",
			normalizeUsername(username), l.maxUser)
	}

	l.podarLocked()
}

func (l *loginLimiter) podarLocked() {
	if l.maxKeys <= 0 || len(l.failures) <= l.maxKeys {
		return
	}

	cutoff := l.now().Add(-l.window)
	for chave, marcas := range l.failures {
		if len(marcas) == 0 || !marcas[len(marcas)-1].After(cutoff) {
			delete(l.failures, chave)
		}
	}
	if len(l.failures) <= l.maxKeys {
		return
	}

	type candidata struct {
		chave  string
		falhas int
		ultima time.Time
	}
	restantes := make([]candidata, 0, len(l.failures))
	for chave, marcas := range l.failures {
		restantes = append(restantes, candidata{chave, len(marcas), marcas[len(marcas)-1]})
	}
	sort.Slice(restantes, func(i, j int) bool {
		if restantes[i].falhas != restantes[j].falhas {
			return restantes[i].falhas < restantes[j].falhas
		}
		return restantes[i].ultima.Before(restantes[j].ultima)
	})

	excedente := len(l.failures) - l.maxKeys
	for _, c := range restantes[:excedente] {
		delete(l.failures, c.chave)
	}
	log.Printf("[Auth] limitador de login podado: %d chaves descartadas, teto de %d", excedente, l.maxKeys)
}

func (l *loginLimiter) succeed(username string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.failures, userKey(username))
}

func (l *loginLimiter) countLocked(key string) int {
	cutoff := l.now().Add(-l.window)
	kept := l.failures[key][:0]
	for _, at := range l.failures[key] {
		if at.After(cutoff) {
			kept = append(kept, at)
		}
	}
	if len(kept) == 0 {
		delete(l.failures, key)
		return 0
	}
	l.failures[key] = kept
	return len(kept)
}

func clientIP(r *http.Request, trustProxy bool) string {
	if trustProxy {
		if v := strings.TrimSpace(r.Header.Get("X-Real-IP")); v != "" {
			return v
		}
		if parts := strings.Split(r.Header.Get("X-Forwarded-For"), ","); len(parts) > 0 {
			if last := strings.TrimSpace(parts[len(parts)-1]); last != "" {
				return last
			}
		}
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

const (
	defaultIngestWindow = time.Minute
	defaultIngestMax    = 120
	defaultEnrollMax    = 10
)

var ingestLimiter = newLoginLimiter(defaultIngestWindow, defaultIngestMax, defaultIngestMax)

func janelaDeIngestao() time.Duration {
	return config.Duracao("INGEST_RATE_WINDOW", defaultIngestWindow)
}

func tetoDeIngestao() int {
	return config.Inteiro("INGEST_RATE_MAX", defaultIngestMax)
}

func tetoDeEnroll() int {
	return config.Inteiro("INGEST_RATE_MAX_ENROLL", defaultEnrollMax)
}

func (l *loginLimiter) usar(chave string, teto int, janela time.Duration) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	agora := l.now()
	corte := agora.Add(-janela)
	marcas := l.failures[chave][:0]
	for _, m := range l.failures[chave] {
		if m.After(corte) {
			marcas = append(marcas, m)
		}
	}
	l.failures[chave] = marcas

	if len(marcas) >= teto {
		espera := marcas[0].Add(janela).Sub(agora)
		if espera < time.Second {
			espera = time.Second
		}
		return false, espera
	}

	l.failures[chave] = append(marcas, agora)
	l.podarLocked()
	return true, 0
}

func chaveDeIngestao(r *http.Request, cred deviceAuth) string {
	if cred.DeviceID != "" {
		return "ingest:" + cred.DeviceID
	}
	return "ingest-ip:" + clientIP(r, config.Booleano("TRUST_PROXY_HEADERS", false))
}

func limitarTaxa(w http.ResponseWriter, chave string, teto int) bool {
	permitido, espera := ingestLimiter.usar(chave, teto, janelaDeIngestao())
	if permitido {
		return true
	}

	w.Header().Set("Retry-After", strconv.Itoa(int(espera.Seconds()+0.999)))
	writeError(w, http.StatusTooManyRequests, "limite de envios atingido; tente de novo depois da janela")
	return false
}

func zerarLimiteDeIngestao() {
	ingestLimiter.mu.Lock()
	defer ingestLimiter.mu.Unlock()
	clear(ingestLimiter.failures)
}

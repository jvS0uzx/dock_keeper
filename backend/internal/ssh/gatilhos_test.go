package ssh

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/alert"
)

type avisoCapturado struct {
	chave  string
	texto  string
	origem alert.Entrada
}

func capturarAvisos(t *testing.T) func() []avisoCapturado {
	t.Helper()

	var mu sync.Mutex
	var avisos []avisoCapturado
	original := notifyAlert
	notifyAlert = func(e alert.Entrada) bool {
		mu.Lock()
		defer mu.Unlock()
		avisos = append(avisos, avisoCapturado{e.Key, e.Text, e})
		return true
	}
	t.Cleanup(func() { notifyAlert = original })
	return func() []avisoCapturado {
		mu.Lock()
		defer mu.Unlock()
		return append([]avisoCapturado(nil), avisos...)
	}
}

func TestFalhaDeLoginExtraiOIPDeOrigem(t *testing.T) {
	casos := map[string]string{
		"Sep 18 10:00:01 vps sshd[811]: Failed password for root from 198.51.100.7 port 51422 ssh2": "198.51.100.7",
		"Sep 18 10:00:03 vps sshd[813]: Invalid user oracle from 2001:db8::7 port 5555":             "2001:db8::7",
	}
	for linha, ip := range casos {
		got, ok := failedLoginIP(linha)
		if !ok || got != ip {
			t.Errorf("failedLoginIP(%q) = (%q, %v), esperado %q", linha, got, ok, ip)
		}
	}
	for _, linha := range []string{
		"Sep 18 10:00:04 vps sshd[814]: Accepted publickey for root from 198.51.100.9 port 22 ssh2",
		"Sep 18 10:00:05 vps sshd[815]: Failed password for root from nao-e-ip port 22 ssh2",
		"Sep 18 10:00:02 vps sshd[812]: Failed password for invalid user admin from 198.51.100.8 port 4242 ssh2",
	} {
		if _, ok := failedLoginIP(linha); ok {
			t.Errorf("linha que não é falha de login contou: %q", linha)
		}
	}
}

func falha(ip string) string {
	return "sshd[1]: Failed password for root from " + ip + " port 22 ssh2"
}

func TestForcaBrutaDisparaNaDecimaFalhaDaJanela(t *testing.T) {
	avisos := capturarAvisos(t)
	t.Setenv("BRUTEFORCE_THRESHOLD", "")
	t.Setenv("BRUTEFORCE_WINDOW", "")
	d := newBruteForceDetector(Target{ID: "srv-bf", Name: "vps-bf", Host: "203.0.113.5"})

	base := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	for i := range 9 {
		d.observe(falha("198.51.100.7"), base.Add(time.Duration(i)*20*time.Second))
	}
	if n := len(avisos()); n != 0 {
		t.Fatalf("9 falhas dispararam %d aviso(s); o limiar padrão é 10", n)
	}

	d.observe(falha("198.51.100.7"), base.Add(4*time.Minute))
	got := avisos()
	if len(got) != 1 || got[0].chave != "bruteforce:srv-bf:198.51.100.7" || !strings.HasPrefix(got[0].texto, "[ALERTA]") {
		t.Fatalf("avisos = %+v, esperado um [ALERTA] com a chave bruteforce:srv-bf:198.51.100.7", got)
	}
}

func TestForcaBrutaEsqueceOQueSaiuDaJanela(t *testing.T) {
	avisos := capturarAvisos(t)
	t.Setenv("BRUTEFORCE_THRESHOLD", "10")
	t.Setenv("BRUTEFORCE_WINDOW", "5m")
	d := newBruteForceDetector(Target{ID: "srv-bf"})

	base := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	for i := range 9 {
		d.observe(falha("198.51.100.7"), base.Add(time.Duration(i)*time.Second))
	}
	d.observe(falha("198.51.100.7"), base.Add(6*time.Minute))
	d.observe(falha("198.51.100.8"), base.Add(6*time.Minute))

	if n := len(avisos()); n != 0 {
		t.Errorf("falhas espalhadas além da janela de 5 min dispararam %d aviso(s)", n)
	}
}

func TestTentativaComUsuarioInexistenteContaUmaVez(t *testing.T) {
	avisos := capturarAvisos(t)
	t.Setenv("BRUTEFORCE_THRESHOLD", "2")
	t.Setenv("BRUTEFORCE_WINDOW", "5m")
	d := newBruteForceDetector(Target{ID: "srv-bf"})

	base := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	d.observe("sshd[9]: Invalid user admin from 198.51.100.7 port 4242", base)
	d.observe("sshd[9]: Failed password for invalid user admin from 198.51.100.7 port 4242 ssh2", base.Add(time.Second))
	if n := len(avisos()); n != 0 {
		t.Fatalf("as duas linhas do sshd para a mesma tentativa contaram como 2 falhas (%d aviso com limiar 2)", n)
	}

	d.observe(falha("198.51.100.7"), base.Add(2*time.Second))
	if n := len(avisos()); n != 1 {
		t.Errorf("a segunda tentativa não completou 2 falhas: %d aviso(s), esperado 1", n)
	}
}

func TestForcaBrutaConfiguravel(t *testing.T) {
	avisos := capturarAvisos(t)
	t.Setenv("BRUTEFORCE_THRESHOLD", "3")
	t.Setenv("BRUTEFORCE_WINDOW", "1m")
	d := newBruteForceDetector(Target{ID: "srv-bf"})

	base := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	for i := range 3 {
		d.observe(falha("198.51.100.7"), base.Add(time.Duration(i)*10*time.Second))
	}
	if n := len(avisos()); n != 1 {
		t.Errorf("com BRUTEFORCE_THRESHOLD=3, três falhas em 20 s geraram %d aviso(s), esperado 1", n)
	}
}

func TestVigiaDoAuthLogSegueONomeSemRecontar(t *testing.T) {
	avisos := capturarAvisos(t)
	t.Setenv("BRUTEFORCE_THRESHOLD", "")
	t.Setenv("BRUTEFORCE_WINDOW", "")
	t.Setenv("SSH_AUTH_LOG_PATH", "")

	var linhas strings.Builder
	for range 10 {
		linhas.WriteString(falha("198.51.100.7") + "\n")
	}
	linhas.WriteString(falha("198.51.100.9") + "\n")

	var comando string
	alvo := alvoComServidor(t, func(cmd string) respostaExec {
		comando = cmd
		return respostaExec{stdout: linhas.String()}
	})

	if err := StartAuthWatch(context.Background(), alvo); err != nil {
		t.Fatalf("vigia do auth.log: %v", err)
	}
	if comando != "tail -n 0 -F /var/log/auth.log" {
		t.Errorf("vigia rodou %q, esperado tail -n 0 -F /var/log/auth.log (segue o nome no logrotate e não relê linhas antigas)", comando)
	}
	got := avisos()
	if len(got) != 1 || got[0].chave != "bruteforce:"+alvo.ID+":198.51.100.7" {
		t.Errorf("avisos = %+v, esperado um só, para 198.51.100.7", got)
	}
}

func TestAuthlogWatchDesligavel(t *testing.T) {
	casos := map[string]bool{"": true, "true": true, "false": false, "0": false, "talvez": true}
	for valor, esperado := range casos {
		t.Setenv("AUTHLOG_WATCH", valor)
		if got := authWatchEnabled(); got != esperado {
			t.Errorf("AUTHLOG_WATCH=%q: vigia ligado = %v, esperado %v", valor, got, esperado)
		}
	}
}

func TestParseNginxExtraiOCodigoReal(t *testing.T) {
	casos := map[string]int{
		`10.0.0.9 - app.exemplo.com to: 10.0.0.2:8080: GET /api 502 120`:      502,
		`10.0.0.9 - app.exemplo.com to: 10.0.0.2:8080: GET /api 501 120`:      501,
		`10.0.0.9 - app.exemplo.com to: 10.0.0.2:8080: GET /v1/500 200 404`:   200,
		`10.0.0.9 - app.exemplo.com to: 10.0.0.2:8080: GET /x HTTP/1.1 301 0`: 301,
	}
	for linha, codigo := range casos {
		e, ok := parseNginxEntry(linha)
		if !ok || e.Code != codigo {
			t.Errorf("parseNginxEntry(%q) = (%+v, %v), esperado código %d", linha, e, ok, codigo)
		}
	}
	if k, _ := parseNginxLine(`10.0.0.9 - app.exemplo.com to: 10.0.0.2:8080: GET /x HTTP/1.1 301 0`); k.Status != "200" {
		t.Errorf("status fora da lista acompanhada virou %q; o balde gravado continua sendo 200", k.Status)
	}
}

func TestUpstreamCom5xxDisparaAPartirDe20Requisicoes(t *testing.T) {
	avisos := capturarAvisos(t)
	t.Setenv("LB_ERROR_RATIO", "")
	t.Setenv("LB_WINDOW", "")
	t.Setenv("LB_MIN_REQUESTS", "")
	h := newLBHealth(Target{ID: "srv-lb", Name: "lb", Host: "203.0.113.6"})

	base := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	for i := range 19 {
		codigo := 200
		if i%2 == 0 {
			codigo = 502
		}
		h.observe("10.0.0.2:8080", codigo, base.Add(time.Duration(i)*time.Second))
	}
	if n := len(avisos()); n != 0 {
		t.Fatalf("19 requisições com 10 5xx dispararam %d aviso(s); o mínimo é 20", n)
	}

	h.observe("10.0.0.2:8080", 200, base.Add(20*time.Second))
	got := avisos()
	if len(got) != 1 || got[0].chave != "lb_upstream_5xx:srv-lb:10.0.0.2:8080" || !strings.HasPrefix(got[0].texto, "[ALERTA]") {
		t.Fatalf("avisos = %+v, esperado um [ALERTA] com chave lb_upstream_5xx:srv-lb:10.0.0.2:8080 (10 de 20 = 50%%)", got)
	}
}

func TestLBMinRequestsConfiguravel(t *testing.T) {
	avisos := capturarAvisos(t)
	t.Setenv("LB_ERROR_RATIO", "0.5")
	t.Setenv("LB_WINDOW", "5m")
	t.Setenv("LB_MIN_REQUESTS", "5")
	h := newLBHealth(Target{ID: "srv-lb"})

	base := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	for i, codigo := range []int{502, 200, 503, 200, 500} {
		h.observe("10.0.0.4:8080", codigo, base.Add(time.Duration(i)*time.Second))
	}
	if n := len(avisos()); n != 1 {
		t.Errorf("com LB_MIN_REQUESTS=5, 3 de 5 requisições com 5xx geraram %d aviso(s), esperado 1", n)
	}

	for _, valor := range []string{"", "zero", "0", "-3"} {
		t.Setenv("LB_MIN_REQUESTS", valor)
		if got := newLBHealth(Target{}).minRequests; got != 20 {
			t.Errorf("LB_MIN_REQUESTS=%q resultou em %d; o padrão é 20", valor, got)
		}
	}
}

func TestUpstreamAbaixoDaProporcaoNaoDispara(t *testing.T) {
	avisos := capturarAvisos(t)
	t.Setenv("LB_ERROR_RATIO", "0.5")
	t.Setenv("LB_WINDOW", "5m")
	h := newLBHealth(Target{ID: "srv-lb"})

	base := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	for i := range 40 {
		codigo := 200
		if i%5 < 2 {
			codigo = 503
		}
		h.observe("10.0.0.3:8080", codigo, base.Add(time.Duration(i)*time.Second))
	}
	if n := len(avisos()); n != 0 {
		t.Errorf("40%% de 5xx disparou %d aviso(s) com LB_ERROR_RATIO=0.5", n)
	}
}

func TestStreamDoNginxCaindoGeraCritico(t *testing.T) {
	avisos := capturarAvisos(t)
	t.Setenv("AUTHLOG_WATCH", "false")
	alvo := Target{ID: "srv-nginx-down", Name: "lb", Host: "127.0.0.1", Port: 1, User: "root",
		KeyPath: filepath.Join(t.TempDir(), "inexistente"), CollectNginx: true}

	Manager.Start(alvo)
	t.Cleanup(func() { Manager.Stop(alvo.ID) })

	prazo := time.Now().Add(3 * time.Second)
	for time.Now().Before(prazo) {
		for _, a := range avisos() {
			if a.chave == "nginx_down:"+alvo.ID {
				if !strings.HasPrefix(a.texto, "[CRITICO]") {
					t.Errorf("aviso de nginx caído sem prefixo [CRITICO]: %q", a.texto)
				}
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("stream do nginx caiu e nenhum aviso nginx_down:%s saiu; avisos: %v", alvo.ID, fmt.Sprint(avisos()))
}

package ssh

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"io"
	"net"
	"runtime"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/jvS0uzx/dock_keeper/internal/database"
)

type servidorKeepalive struct {
	addr      string
	hostPub   ssh.PublicKey
	conexoes  atomic.Int32
	responder atomic.Bool
}

func novoServidorKeepalive(t *testing.T) *servidorKeepalive {
	t.Helper()

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("chave de host: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatalf("signer: %v", err)
	}
	cfg := &ssh.ServerConfig{NoClientAuth: true}
	cfg.AddHostKey(signer)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })

	s := &servidorKeepalive{addr: ln.Addr().String(), hostPub: signer.PublicKey()}
	s.responder.Store(true)
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			s.conexoes.Add(1)
			go s.atender(conn, cfg)
		}
	}()
	return s
}

func (s *servidorKeepalive) atender(conn net.Conn, cfg *ssh.ServerConfig) {
	sconn, chans, reqs, err := ssh.NewServerConn(conn, cfg)
	if err != nil {
		return
	}
	defer sconn.Close()
	go func() {
		for req := range reqs {
			if s.responder.Load() && req.WantReply {
				req.Reply(false, nil)
			}
		}
	}()
	for novo := range chans {
		ch, sessReqs, err := novo.Accept()
		if err != nil {
			continue
		}
		go func() {
			for req := range sessReqs {
				req.Reply(req.Type == "exec", nil)
			}
		}()
		go io.Copy(io.Discard, ch)
	}
}

func (s *servidorKeepalive) alvo(t *testing.T, id string) Target {
	t.Helper()
	usaPoliticaHostKey(t, knownHostsPara(t, s.addr, s.hostPub))
	host, porta, _ := net.SplitHostPort(s.addr)
	p, _ := strconv.Atoi(porta)
	return Target{ID: id, Name: id, Host: host, Port: p, User: "root", KeyPath: chaveClienteTemporaria(t)}
}

func keepaliveRapido(t *testing.T) {
	t.Helper()
	original := keepaliveTimeoutNs.Load()
	keepaliveTimeoutNs.Store(int64(60 * time.Millisecond))
	t.Cleanup(func() { keepaliveTimeoutNs.Store(original) })
}

func TestKeepaliveMedeRTTNaConexaoAberta(t *testing.T) {
	srv := novoServidorKeepalive(t)
	alvo := srv.alvo(t, "srv-ka-rtt")
	t.Cleanup(func() { forgetRTT(alvo.ID) })
	client, err := dial(alvo)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go runKeepalive(ctx, alvo, client, 20*time.Millisecond, keepaliveTimeout(), 3, true)

	prazo := time.Now().Add(2 * time.Second)
	for latestRTT(alvo.ID) == nil && time.Now().Before(prazo) {
		time.Sleep(10 * time.Millisecond)
	}
	if v := latestRTT(alvo.ID); v == nil || *v <= 0 {
		t.Fatalf("RTT pelo keepalive = %v, esperado valor maior que zero", v)
	}
	if n := srv.conexoes.Load(); n != 1 {
		t.Errorf("a medida abriu %d conexões; o keepalive usa a que já está aberta", n)
	}
}

func TestTresKeepalivesPerdidosFecham(t *testing.T) {
	keepaliveRapido(t)
	srv := novoServidorKeepalive(t)
	srv.responder.Store(false)
	alvo := srv.alvo(t, "srv-ka-mudo")
	t.Cleanup(func() { forgetRTT(alvo.ID) })
	client, err := dial(alvo)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	fim := make(chan struct{})
	go func() {
		runKeepalive(ctx, alvo, client, 20*time.Millisecond, keepaliveTimeout(), 3, true)
		close(fim)
	}()

	select {
	case <-fim:
	case <-time.After(3 * time.Second):
		t.Fatal("três keepalives sem resposta não encerraram a vigia em 3 s")
	}
	fechada := make(chan struct{})
	go func() {
		client.Wait()
		close(fechada)
	}()
	select {
	case <-fechada:
	case <-time.After(time.Second):
		t.Error("a conexão meio-aberta continuou aberta depois de 3 perdas")
	}
	if v := latestRTT(alvo.ID); v != nil {
		t.Errorf("sem resposta o RTT ficou %v; esperado NULL", *v)
	}
}

func TestConexaoMudaReconectaPeloSupervise(t *testing.T) {
	keepaliveRapido(t)
	t.Setenv("RTT_PROBE_INTERVAL", "20ms")
	t.Setenv("SSH_KEEPALIVE_MAX_MISSES", "3")
	srv := novoServidorKeepalive(t)
	srv.responder.Store(false)
	alvo := srv.alvo(t, "srv-ka-reconecta")
	t.Cleanup(func() { forgetRTT(alvo.ID) })

	ctx, cancel := context.WithCancel(context.Background())
	fim := make(chan struct{})
	go func() {
		supervise(ctx, "metricas", alvo, StartStream, nil)
		close(fim)
	}()
	t.Cleanup(func() {
		cancel()
		<-fim
	})

	prazo := time.Now().Add(9 * time.Second)
	for srv.conexoes.Load() < 2 && time.Now().Before(prazo) {
		time.Sleep(50 * time.Millisecond)
	}
	if n := srv.conexoes.Load(); n < 2 {
		t.Fatalf("depois das perdas de keepalive houve %d conexão(ões); esperado que o supervise reconectasse", n)
	}
}

func TestRTTProbeDesligadoMantemADeteccao(t *testing.T) {
	keepaliveRapido(t)
	srv := novoServidorKeepalive(t)
	alvo := srv.alvo(t, "srv-ka-sem-rtt")
	t.Cleanup(func() { forgetRTT(alvo.ID) })
	client, err := dial(alvo)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	fim := make(chan struct{})
	go func() {
		runKeepalive(ctx, alvo, client, 20*time.Millisecond, keepaliveTimeout(), 3, false)
		close(fim)
	}()
	time.Sleep(150 * time.Millisecond)
	if v := latestRTT(alvo.ID); v != nil {
		t.Errorf("com a gravação desligada o RTT virou %v", *v)
	}

	srv.responder.Store(false)
	select {
	case <-fim:
	case <-time.After(3 * time.Second):
		cancel()
		t.Fatal("com RTT_PROBE=false a detecção de conexão meio-aberta parou de funcionar")
	}
	cancel()
}

func TestStopEncerraAVigiaSemVazarGoroutine(t *testing.T) {
	t.Setenv("RTT_PROBE_INTERVAL", "20ms")
	t.Setenv("AUTHLOG_WATCH", "false")
	srv := novoServidorKeepalive(t)
	alvo := srv.alvo(t, "srv-ka-stop")

	base := runtime.NumGoroutine()
	m := &ServerManager{cancelFuncs: map[string]context.CancelFunc{}}
	m.Start(alvo)

	prazo := time.Now().Add(3 * time.Second)
	for latestRTT(alvo.ID) == nil && time.Now().Before(prazo) {
		time.Sleep(10 * time.Millisecond)
	}
	if latestRTT(alvo.ID) == nil {
		t.Fatal("o stream subiu e o keepalive não mediu RTT em 3 s")
	}

	m.Stop(alvo.ID)
	prazo = time.Now().Add(3 * time.Second)
	for runtime.NumGoroutine() > base && time.Now().Before(prazo) {
		time.Sleep(20 * time.Millisecond)
	}
	if n := runtime.NumGoroutine(); n > base {
		t.Errorf("goroutines depois do Stop = %d, antes do Start = %d", n, base)
	}
	if latestRTT(alvo.ID) != nil {
		t.Error("o RTT do servidor parado continuou em memória")
	}
}

func TestKeepaliveMaxMissesConfiguravel(t *testing.T) {
	for valor, esperado := range map[string]int{"": 3, "5": 5, "0": 3, "x": 3} {
		t.Setenv("SSH_KEEPALIVE_MAX_MISSES", valor)
		if got := keepaliveMaxMisses(); got != esperado {
			t.Errorf("SSH_KEEPALIVE_MAX_MISSES=%q resultou em %d, esperado %d", valor, got, esperado)
		}
	}
}

func TestRTTProbeDesligavelEIntervaloConfiguravel(t *testing.T) {
	for valor, esperado := range map[string]bool{"": true, "true": true, "false": false, "0": false, "talvez": true} {
		t.Setenv("RTT_PROBE", valor)
		if got := rttProbeEnabled(); got != esperado {
			t.Errorf("RTT_PROBE=%q: ligado = %v, esperado %v", valor, got, esperado)
		}
	}
	t.Setenv("RTT_PROBE_INTERVAL", "")
	if got := rttInterval(); got != 30*time.Second {
		t.Errorf("intervalo padrão = %s, esperado 30s", got)
	}
	t.Setenv("RTT_PROBE_INTERVAL", "10s")
	if got := rttInterval(); got != 10*time.Second {
		t.Errorf("RTT_PROBE_INTERVAL=10s resultou em %s", got)
	}
}

func TestStoreHostMetricGravaAUltimaMedidaDeRTT(t *testing.T) {
	srv := servidorDeTeste(t)
	alvo := Target{ID: srv.ID, Host: srv.HostIP}
	t.Cleanup(func() { forgetRTT(alvo.ID) })

	medida := 12.5
	setRTT(alvo.ID, &medida)
	storeHostMetric(alvo, SysPayload{}, 10)
	setRTT(alvo.ID, nil)
	storeHostMetric(alvo, SysPayload{}, 10)

	var linhas []database.MetricServer
	database.DB.Where("server_id = ?", srv.ID).Order("id asc").Find(&linhas)
	if len(linhas) != 2 {
		t.Fatalf("amostras = %d, esperado 2", len(linhas))
	}
	if linhas[0].RTTMs == nil || *linhas[0].RTTMs != 12.5 {
		t.Errorf("rtt_ms gravado = %v, esperado 12.5", linhas[0].RTTMs)
	}
	if linhas[1].RTTMs != nil {
		t.Errorf("sem medida o rtt_ms virou %v; esperado NULL", *linhas[1].RTTMs)
	}
}

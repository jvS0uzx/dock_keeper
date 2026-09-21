package ssh

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/alert"
	"github.com/jvS0uzx/dock_keeper/internal/config"
	"github.com/jvS0uzx/dock_keeper/internal/database"
)

const (
	defaultBruteForceThreshold = 10
	defaultBruteForceWindow    = 5 * time.Minute
	defaultLBErrorRatio        = 0.5
	defaultLBWindow            = 5 * time.Minute
	defaultLBMinRequests       = 20
	amostrasParaEstabilizar    = 3
	crescimentosParaLoop       = 2
)

var (
	notifyAlert   = alert.Enqueue
	resolveAlert  = alert.Recovered
	openAlertKeys = alert.ChavesAbertas

	streamEstavelApos = 30 * time.Second
)

type alvoAlerta struct {
	tipo    string
	id      string
	nome    string
	metrica string
	valor   *float64
	limiar  *float64
	unidade string
}

func alvoDoHost(t Target) alvoAlerta {
	return alvoAlerta{tipo: database.AlvoTipoHost, id: t.ID, nome: t.Name, metrica: "estado"}
}

func alvoDoServico(nome string) alvoAlerta {
	return alvoAlerta{tipo: database.AlvoTipoServico, id: nome, nome: nome, metrica: "estado"}
}

func alvoDoContainer(c DockerPSPayload) alvoAlerta {
	return alvoAlerta{tipo: database.AlvoTipoContainer, id: c.DockerID, nome: c.Name, metrica: "estado"}
}

func alertaDe(t Target, chave, severidade, texto string, a alvoAlerta) alert.Entrada {
	e := alert.Entrada{
		Key: chave, Text: texto, Severity: severidade, SiteID: t.SiteID,
		AlvoTipo: a.tipo, AlvoID: a.id, AlvoNome: a.nome,
		Metrica: a.metrica, Valor: a.valor, Limiar: a.limiar, Unidade: a.unidade,
	}
	if t.ID != "" {
		id := t.ID
		e.ServerID = &id
	}
	return e
}

type incidentes struct {
	t       Target
	prefixo string
	abertos map[string]bool
}

func novosIncidentes(t Target, gatilho string) *incidentes {
	in := &incidentes{t: t, prefixo: gatilho + ":" + t.ID + ":", abertos: map[string]bool{}}
	for _, chave := range openAlertKeys(in.prefixo) {
		in.abertos[strings.TrimPrefix(chave, in.prefixo)] = true
	}
	return in
}

func (in *incidentes) abrir(item string, a alvoAlerta, severidade, texto string) {
	in.abertos[item] = true
	notifyAlert(alertaDe(in.t, in.prefixo+item, severidade, texto, a))
}

func (in *incidentes) fechar(item string, a alvoAlerta, texto string) {
	if !in.abertos[item] {
		return
	}
	delete(in.abertos, item)
	resolveAlert(alertaDe(in.t, in.prefixo+item, "info", texto, a))
}

func aoFicarDePe(apos time.Duration, avisar func()) func() {
	timer := time.AfterFunc(apos, avisar)
	return func() { timer.Stop() }
}

func hostDeVolta(t Target) {
	resolveAlert(alertaDe(t, "host_unreachable:"+t.ID, "info",
		fmt.Sprintf("[INFO] Recuperado - VPS %s (%s) voltou a responder", t.Name, t.Host), alvoDoHost(t)))
}

func nginxDeVolta(t Target) {
	resolveAlert(alertaDe(t, "nginx_down:"+t.ID, "info",
		fmt.Sprintf("[INFO] Recuperado - Stream do nginx em %s (%s) está de pé", t.Name, t.Host), alvoDoServico("nginx")))
}

type vigiaDeContainers struct {
	t            Target
	caidos       *incidentes
	reinicios    map[string]int
	crescimentos map[string]int
	emLoop       map[string]bool
	estaveis     map[string]int
}

func newVigiaDeContainers(t Target) *vigiaDeContainers {
	return &vigiaDeContainers{
		t:            t,
		caidos:       novosIncidentes(t, "container_down"),
		reinicios:    map[string]int{},
		crescimentos: map[string]int{},
		emLoop:       map[string]bool{},
		estaveis:     map[string]int{},
	}
}

func (v *vigiaDeContainers) registrarReinicios(c DockerPSPayload, insp DockerInspectPayload, inspecionado bool) bool {
	if !inspecionado || insp.RestartCount == nil {
		return false
	}
	anterior, conhecido := v.reinicios[c.DockerID]
	v.reinicios[c.DockerID] = *insp.RestartCount
	if conhecido && *insp.RestartCount > anterior {
		v.crescimentos[c.DockerID]++
		v.estaveis[c.DockerID] = 0
		if v.crescimentos[c.DockerID] >= crescimentosParaLoop {
			v.emLoop[c.DockerID] = true
		}
		return true
	}
	if v.crescimentos[c.DockerID] == 0 {
		return false
	}
	v.estaveis[c.DockerID]++
	if v.estaveis[c.DockerID] >= amostrasParaEstabilizar {
		delete(v.emLoop, c.DockerID)
		delete(v.estaveis, c.DockerID)
		delete(v.crescimentos, c.DockerID)
	}
	return false
}

func (v *vigiaDeContainers) observe(ps []DockerPSPayload, inspecao []DockerInspectPayload) {
	porID := make(map[string]DockerInspectPayload, len(inspecao))
	for _, in := range inspecao {
		porID[in.DockerID] = in
	}

	for _, c := range ps {
		insp, inspecionado := porID[c.DockerID]
		cresceu := v.registrarReinicios(c, insp, inspecionado)

		switch c.State {
		case "":
		case "running":
			switch {
			case v.emLoop[c.DockerID]:
				v.caidos.abrir(c.Name, alvoDoContainer(c), "high", fmt.Sprintf(
					"[ALERTA] Container %s está reiniciando em ciclo em %s (%d reinícios acumulados)",
					c.Name, v.t.Host, v.reinicios[c.DockerID]))
			case cresceu && insp.OOMKilled != nil && *insp.OOMKilled:
				v.caidos.abrir(c.Name, alvoDoContainer(c), "high", fmt.Sprintf(
					"[ALERTA] Container %s foi morto por falta de memória em %s", c.Name, v.t.Host))
			case inspecionado && insp.Health == "unhealthy":
				v.caidos.abrir(c.Name, alvoDoContainer(c), "high", fmt.Sprintf(
					"[ALERTA] Container %s está rodando com healthcheck unhealthy em %s", c.Name, v.t.Host))
			default:
				v.caidos.fechar(c.Name, alvoDoContainer(c), fmt.Sprintf("[INFO] Recuperado - Container %s voltou a rodar em %s", c.Name, v.t.Host))
			}
		default:
			v.caidos.abrir(c.Name, alvoDoContainer(c), "high", fmt.Sprintf("[ALERTA] Container %s está %s em %s", c.Name, c.State, v.t.Host))
		}
	}
}

func failedLoginIP(line string) (string, bool) {
	if !strings.Contains(line, "Failed password") && !strings.Contains(line, "Invalid user") {
		return "", false
	}
	if strings.Contains(line, "Failed password for invalid user") {
		return "", false
	}
	_, rest, ok := strings.Cut(line, " from ")
	if !ok {
		return "", false
	}
	fields := strings.Fields(rest)
	if len(fields) == 0 || net.ParseIP(fields[0]) == nil {
		return "", false
	}
	return fields[0], true
}

type bruteForceDetector struct {
	t         Target
	threshold int
	span      time.Duration

	mu      sync.Mutex
	window  *slidingWindow
	ataques *incidentes
}

func newBruteForceDetector(t Target) *bruteForceDetector {
	span := database.EnvDuration("BRUTEFORCE_WINDOW", defaultBruteForceWindow)
	return &bruteForceDetector{
		t:         t,
		threshold: config.Inteiro("BRUTEFORCE_THRESHOLD", defaultBruteForceThreshold),
		span:      span,
		window:    newSlidingWindow(span),
		ataques:   novosIncidentes(t, "bruteforce"),
	}
}

func (d *bruteForceDetector) observe(line string, now time.Time) {
	ip, ok := failedLoginIP(line)
	if !ok {
		return
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	_, failures := d.window.add(ip, now, true)
	if failures < d.threshold {
		return
	}
	alvoAtacado := alvoDoHost(d.t)
	alvoAtacado.metrica = "falhas_de_login"
	observado := float64(failures)
	limite := float64(d.threshold)
	alvoAtacado.valor = &observado
	alvoAtacado.limiar = &limite

	d.ataques.abrir(ip, alvoAtacado, "high", fmt.Sprintf(
		"[ALERTA] Força bruta em %s (%s): %d falhas de login vindas de %s em %s",
		d.t.Name, d.t.Host, failures, ip, d.span))
}

func (d *bruteForceDetector) revisar(now time.Time) {
	d.mu.Lock()
	defer d.mu.Unlock()

	for ip := range d.ataques.abertos {
		if d.window.hits(ip, now) > 0 {
			continue
		}
		d.ataques.fechar(ip, alvoDoHost(d.t), fmt.Sprintf(
			"[INFO] Recuperado - Força bruta em %s (%s): nenhuma falha de login de %s nos últimos %s",
			d.t.Name, d.t.Host, ip, d.span))
	}
}

func (d *bruteForceDetector) vigiar(ctx context.Context) {
	ticker := time.NewTicker(d.window.step)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.revisar(time.Now())
		}
	}
}

func StartAuthWatch(ctx context.Context, t Target) error {
	client, session, err := openSession(t)
	if err != nil {
		return err
	}
	defer client.Close()
	defer session.Close()

	stopOnCancel(ctx, client, session)

	stdout, err := session.StdoutPipe()
	if err != nil {
		return err
	}
	if err := session.Start(authWatchCommand(t)); err != nil {
		return err
	}

	detector := newBruteForceDetector(t)
	vigiaCtx, pararVigia := context.WithCancel(ctx)
	defer pararVigia()
	go detector.vigiar(vigiaCtx)

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		detector.observe(scanner.Text(), time.Now())
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return session.Wait()
}

func authWatchEnabled() bool {
	raw := strings.TrimSpace(os.Getenv("AUTHLOG_WATCH"))
	if raw == "" {
		return true
	}
	on, err := strconv.ParseBool(raw)
	if err != nil {
		log.Printf("[SSH] AUTHLOG_WATCH=%q inválido, vigia de força bruta continua ligado", raw)
		return true
	}
	return on
}

type nginxEntry struct {
	Upstream   string
	ServerName string
	Code       int
}

func parseNginxEntry(line string) (nginxEntry, bool) {
	idxTo := strings.Index(line, " to: ")
	if idxTo == -1 {
		return nginxEntry{}, false
	}

	prefixParts := strings.Split(line[:idxTo], " - ")
	serverName := strings.TrimSpace(prefixParts[len(prefixParts)-1])

	parts := strings.SplitN(line[idxTo+len(" to: "):], ": ", 2)
	if len(parts) < 2 {
		return nginxEntry{}, false
	}

	upstream := strings.TrimSpace(parts[0])
	if upstream == "-" {
		upstream = "Local (Nginx/Cache)"
	}
	if upstream == "" {
		return nginxEntry{}, false
	}

	return nginxEntry{Upstream: upstream, ServerName: serverName, Code: statusCode(parts[1])}, true
}

func statusCode(request string) int {
	fields := strings.Fields(request)
	for i := 2; i < len(fields); i++ {
		if len(fields[i]) != 3 {
			continue
		}
		if code, err := strconv.Atoi(fields[i]); err == nil && code >= 100 && code <= 599 {
			return code
		}
	}
	return 0
}

type lbHealth struct {
	t           Target
	minRequests int
	ratio       float64
	span        time.Duration
	window      *slidingWindow
	doentes     *incidentes
}

func newLBHealth(t Target) *lbHealth {
	span := database.EnvDuration("LB_WINDOW", defaultLBWindow)
	return &lbHealth{t: t, minRequests: config.Inteiro("LB_MIN_REQUESTS", defaultLBMinRequests), ratio: lbErrorRatio(), span: span, window: newSlidingWindow(span), doentes: novosIncidentes(t, "lb_upstream_5xx")}
}

func (h *lbHealth) observe(upstream string, code int, now time.Time) {
	total, errors := h.window.add(upstream, now, code >= 500)

	if total < h.minRequests {
		return
	}
	taxa := float64(errors) / float64(total)
	limite := h.ratio
	alvoUpstream := alvoDoServico(upstream)
	alvoUpstream.metrica = "taxa_de_erro"
	alvoUpstream.valor = &taxa
	alvoUpstream.limiar = &limite

	if taxa < h.ratio {
		h.doentes.fechar(upstream, alvoUpstream, fmt.Sprintf(
			"[INFO] Recuperado - Upstream %s em %s (%s): %d de %d requisições com erro 5xx nos últimos %s",
			upstream, h.t.Name, h.t.Host, errors, total, h.span))
		return
	}
	h.doentes.abrir(upstream, alvoUpstream, "high", fmt.Sprintf(
		"[ALERTA] Upstream %s em %s (%s): %d de %d requisições com erro 5xx nos últimos %s",
		upstream, h.t.Name, h.t.Host, errors, total, h.span))
}

func lbErrorRatio() float64 {
	raw := strings.TrimSpace(os.Getenv("LB_ERROR_RATIO"))
	if raw == "" {
		return defaultLBErrorRatio
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil || v <= 0 || v > 1 {
		log.Printf("[SSH] LB_ERROR_RATIO=%q inválido, usando %.2f", raw, defaultLBErrorRatio)
		return defaultLBErrorRatio
	}
	return v
}

func nginxDownAlert(t Target) func(error) {
	return func(err error) {
		notifyAlert(alertaDe(t, "nginx_down:"+t.ID, "critical", fmt.Sprintf(
			"[CRITICO] Stream do nginx em %s (%s) caiu: %v", t.Name, t.Host, err), alvoDoServico("nginx")))
	}
}

func authWatchCommand(t Target) string {
	cmd := "tail -n 0 -F " + AuthLogPath()
	if useSudo(t) {
		return sudoPrefix + "/usr/bin/" + cmd
	}
	return cmd
}

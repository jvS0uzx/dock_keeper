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
)

var notifyAlert = alert.Notify

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
	window    *slidingWindow
}

func newBruteForceDetector(t Target) *bruteForceDetector {
	span := database.EnvDuration("BRUTEFORCE_WINDOW", defaultBruteForceWindow)
	return &bruteForceDetector{
		t:         t,
		threshold: config.Inteiro("BRUTEFORCE_THRESHOLD", defaultBruteForceThreshold),
		span:      span,
		window:    newSlidingWindow(span),
	}
}

func (d *bruteForceDetector) observe(line string, now time.Time) {
	ip, ok := failedLoginIP(line)
	if !ok {
		return
	}
	_, failures := d.window.add(ip, now, true)
	if failures < d.threshold {
		return
	}
	notifyAlert("bruteforce:"+d.t.ID+":"+ip, fmt.Sprintf(
		"[ALERTA] Força bruta em %s (%s): %d falhas de login vindas de %s em %s",
		d.t.Name, d.t.Host, failures, ip, d.span))
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
}

func newLBHealth(t Target) *lbHealth {
	span := database.EnvDuration("LB_WINDOW", defaultLBWindow)
	return &lbHealth{t: t, minRequests: config.Inteiro("LB_MIN_REQUESTS", defaultLBMinRequests), ratio: lbErrorRatio(), span: span, window: newSlidingWindow(span)}
}

func (h *lbHealth) observe(upstream string, code int, now time.Time) {
	total, errors := h.window.add(upstream, now, code >= 500)

	if total < h.minRequests || float64(errors)/float64(total) < h.ratio {
		return
	}
	notifyAlert("lb_upstream_5xx:"+h.t.ID+":"+upstream, fmt.Sprintf(
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
		notifyAlert("nginx_down:"+t.ID, fmt.Sprintf(
			"[CRITICO] Stream do nginx em %s (%s) caiu: %v", t.Name, t.Host, err))
	}
}

func authWatchCommand(t Target) string {
	cmd := "tail -n 0 -F " + AuthLogPath()
	if useSudo(t) {
		return sudoPrefix + "/usr/bin/" + cmd
	}
	return cmd
}

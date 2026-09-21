package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/load"
	"github.com/shirou/gopsutil/v3/mem"
)

const Version = "1.1.0"

const (
	defaultIntervalSec = 5
	httpTimeout        = 10 * time.Second

	cpuSampleWindow = 500 * time.Millisecond
)

type metricsPayload struct {
	Hostname  string   `json:"hostname"`
	CPU       *float64 `json:"cpu,omitempty"`
	MemUsed   int64    `json:"mem_used"`
	MemTotal  int64    `json:"mem_total"`
	Load1     *float64 `json:"load1,omitempty"`
	DiskUsed  int64    `json:"disk_used"`
	DiskTotal int64    `json:"disk_total"`
	Uptime    float64  `json:"uptime"`

	TemperatureC *float64 `json:"temperature_c,omitempty"`

	NetRxBps *float64 `json:"net_rx_bps,omitempty"`
	NetTxBps *float64 `json:"net_tx_bps,omitempty"`

	Addresses []string `json:"addresses,omitempty"`

	OS           string `json:"os"`
	Platform     string `json:"platform"`
	Arch         string `json:"arch"`
	LoggedUser   string `json:"logged_user"`
	SiteCode     string `json:"site_code"`
	AgentVersion string `json:"agent_version"`

	MachineID string `json:"machine_id"`

	ReportIntervalSec int `json:"report_interval_sec"`
}

type agentConfig struct {
	serverURL string
	hostname  string
	siteCode  string
	interval  time.Duration
	inseguro  bool
}

func alvoInseguro(bruto string) (bool, error) {
	endereco, err := url.Parse(strings.TrimSpace(bruto))
	if err != nil || endereco.Host == "" {
		return false, fmt.Errorf("AGENT_SERVER_URL inválido: %q", bruto)
	}
	if endereco.Scheme != "http" {
		return false, nil
	}
	host := endereco.Hostname()
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return false, nil
	}
	return true, nil
}

func loadConfig(getenv func(string) string) (agentConfig, error) {
	serverURL := strings.TrimRight(strings.TrimSpace(getenv("AGENT_SERVER_URL")), "/")
	if serverURL == "" {
		return agentConfig{}, errors.New("AGENT_SERVER_URL não definido")
	}

	inseguro, err := alvoInseguro(serverURL)
	if err != nil {
		return agentConfig{}, err
	}
	permitido, _ := strconv.ParseBool(strings.TrimSpace(getenv("ALLOW_INSECURE_HTTP")))
	if inseguro && !permitido {
		return agentConfig{}, fmt.Errorf(
			"AGENT_SERVER_URL usa http:// para um painel remoto (%s): a credencial do dispositivo viajaria em claro. Use https, ou defina ALLOW_INSECURE_HTTP=true se a rede for confiável",
			serverURL)
	}
	hostname := getenv("AGENT_HOSTNAME")
	if hostname == "" {
		hostname, _ = os.Hostname()
	}

	interval := defaultIntervalSec
	if raw := getenv("AGENT_INTERVAL"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			interval = n
		}
	}

	return agentConfig{
		serverURL: serverURL,
		hostname:  hostname,
		siteCode:  strings.ToLower(strings.TrimSpace(getenv("AGENT_SITE"))),
		interval:  time.Duration(interval) * time.Second,
		inseguro:  inseguro && permitido,
	}, nil
}

func main() {
	if executarComoServico() {
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	iniciar(ctx)
}

func iniciar(ctx context.Context) {
	cfg, err := loadConfig(os.Getenv)
	if err != nil {
		log.Fatalf("[Agent] %v", err)
	}

	if cfg.inseguro {
		log.Printf("[Agent] AVISO: %s usa http:// e ALLOW_INSECURE_HTTP=true; a credencial do dispositivo viaja em claro nesta rede", cfg.serverURL)
	}

	endpoint := cfg.serverURL + "/api/ingest/metrics"
	client := &http.Client{Timeout: httpTimeout}

	cred, legado := resolverIdentidade(client, cfg.serverURL, cfg.hostname)
	maquina := machineID()
	intervalSec := int(cfg.interval / time.Second)

	log.Printf("[Agent] v%s iniciando: host=%s maquina=%s unidade=%q destino=%s intervalo=%ds",
		Version, cfg.hostname, maquina, cfg.siteCode, endpoint, intervalSec)

	rede := novoMedidorDeRede()
	a := &agente{
		client:   client,
		endpoint: endpoint,
		cred:     cred,
		legado:   legado,
		interval: cfg.interval,
		coletar: func() metricsPayload {
			p := collect(cfg.hostname, cfg.siteCode, intervalSec)
			p.MachineID = maquina
			p.NetRxBps, p.NetTxBps = rede.taxas()
			p.Addresses = enderecosDoHost(net.Interfaces)
			return p
		},
	}
	a.run(ctx)
}

type agente struct {
	client   *http.Client
	endpoint string
	cred     credential
	legado   string
	interval time.Duration
	coletar  func() metricsPayload
}

func (a *agente) run(ctx context.Context) {
	ticker := time.NewTicker(a.interval)
	defer ticker.Stop()

	for {
		payload := a.coletar()
		if ctx.Err() != nil {
			log.Println("[Agent] encerrado")
			return
		}

		err := push(ctx, a.client, a.endpoint, a.cred, a.legado, payload)
		switch {
		case err == nil:
			log.Printf("[Agent] enviado (cpu=%s mem=%d/%d temp=%s rede=%s/%s usuário=%q)",
				formatPct(payload.CPU), payload.MemUsed, payload.MemTotal, formatTemp(payload.TemperatureC),
				formatTaxa(payload.NetRxBps), formatTaxa(payload.NetTxBps), payload.LoggedUser)
		case ctx.Err() != nil:
			log.Println("[Agent] encerrado com envio em andamento cancelado")
			return
		case a.cred.DeviceID == "" && errors.Is(err, errCredencialRecusada):
			log.Printf("[Agent] %s; o token compartilhado so e aceito com ALLOW_LEGACY_INGEST_TOKEN=true "+
				"no painel. Migre para AGENT_ENROLL_TOKEN", err)
		default:
			log.Printf("[Agent] %s", descreverFalha(err))
		}

		select {
		case <-ctx.Done():
			log.Println("[Agent] encerrado")
			return
		case <-ticker.C:
		}
	}
}

func formatTaxa(bps *float64) string {
	if bps == nil {
		return "-"
	}
	return strconv.FormatFloat(*bps, 'f', 0, 64) + "B/s"
}

func formatTemp(t *float64) string {
	if t == nil {
		return "sem sensor"
	}
	return strconv.FormatFloat(*t, 'f', 1, 64) + "°C"
}

func medirCPU(ler func() ([]float64, error)) *float64 {
	pcts, err := ler()
	if err != nil {
		log.Printf("[Agent] cpu indisponível: %v", err)
		return nil
	}
	if len(pcts) == 0 {
		return nil
	}
	v := pcts[0]
	return &v
}

func medirLoad(so string, ler func() (*load.AvgStat, error)) *float64 {
	if so == "windows" {
		return nil
	}
	avg, err := ler()
	if err != nil || avg == nil {
		return nil
	}
	v := avg.Load1
	return &v
}

func formatPct(v *float64) string {
	if v == nil {
		return "sem medida"
	}
	return strconv.FormatFloat(*v, 'f', 1, 64) + "%"
}

func collect(hostname, siteCode string, intervalSec int) metricsPayload {
	p := metricsPayload{
		Hostname:          hostname,
		SiteCode:          siteCode,
		AgentVersion:      Version,
		ReportIntervalSec: intervalSec,
	}

	p.CPU = medirCPU(func() ([]float64, error) { return cpu.Percent(cpuSampleWindow, false) })

	if vm, err := mem.VirtualMemory(); err == nil {
		p.MemUsed = int64(vm.Used)
		p.MemTotal = int64(vm.Total)
	} else {
		log.Printf("[Agent] mem indisponível: %v", err)
	}

	p.Load1 = medirLoad(runtime.GOOS, load.Avg)

	if du, err := disk.Usage(rootPath()); err == nil {
		p.DiskUsed = int64(du.Used)
		p.DiskTotal = int64(du.Total)
	} else {
		log.Printf("[Agent] disco indisponível: %v", err)
	}

	if info, err := host.Info(); err == nil {
		p.Uptime = float64(info.Uptime)
		p.OS = info.OS
		p.Platform = info.Platform + " " + info.PlatformVersion
		p.Arch = info.KernelArch
	} else {
		log.Printf("[Agent] info do host indisponível: %v", err)
	}

	p.TemperatureC = maxTemperature()
	p.LoggedUser = activeUser()
	return p
}

func rootPath() string {
	if os.PathSeparator == '\\' {
		return "C:\\"
	}
	return "/"
}

func maxTemperature() *float64 {
	sensors, err := host.SensorsTemperatures()
	if err != nil || len(sensors) == 0 {
		return nil
	}

	var max float64
	for _, s := range sensors {
		if s.Temperature > max && s.Temperature < 150 {
			max = s.Temperature
		}
	}
	if max == 0 {
		return nil
	}
	return &max
}

func activeUser() string {
	users, err := host.Users()
	if err != nil || len(users) == 0 {
		return ""
	}

	seen := make(map[string]bool, len(users))
	var names []string
	for _, u := range users {
		if u.User == "" || seen[u.User] {
			continue
		}
		seen[u.User] = true
		names = append(names, u.User)
	}
	return strings.Join(names, ", ")
}

var errCredencialRecusada = errors.New("credencial recusada pelo painel")

func errCredencialRecusadaHTTP(status int) error {
	return &falhaCredencial{status: status}
}

type falhaCredencial struct{ status int }

func (e *falhaCredencial) Error() string {
	return errCredencialRecusada.Error() + " (HTTP " + strconv.Itoa(e.status) + ")"
}

func (e *falhaCredencial) Unwrap() error { return errCredencialRecusada }

type falhaDeRede struct{ err error }

func (e *falhaDeRede) Error() string { return "falha de rede: " + e.err.Error() }

func (e *falhaDeRede) Unwrap() error { return e.err }

func descreverFalha(err error) string {
	var credencial *falhaCredencial
	if errors.As(err, &credencial) {
		return credencial.Error() + "; confira se o dispositivo foi revogado ou emita um novo convite. " +
			"O agente segue tentando a cada ciclo"
	}
	var rede *falhaDeRede
	if errors.As(err, &rede) {
		return rede.Error()
	}
	var h *httpError
	if errors.As(err, &h) {
		return "painel respondeu HTTP " + strconv.Itoa(h.status)
	}
	return "erro no envio: " + err.Error()
}

func push(ctx context.Context, client *http.Client, endpoint string, cred credential, legado string, payload metricsPayload) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if cred.DeviceID != "" {
		req.Header.Set("X-Device-Id", cred.DeviceID)
		req.Header.Set("X-Device-Token", cred.Token)
	} else {
		req.Header.Set("X-Agent-Token", legado)
	}

	resp, err := client.Do(req)
	if err != nil {
		return &falhaDeRede{err: err}
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusUnauthorized, http.StatusForbidden:
		return errCredencialRecusadaHTTP(resp.StatusCode)
	default:
		return &httpError{status: resp.StatusCode}
	}
}

type httpError struct{ status int }

func (e *httpError) Error() string {
	return "resposta HTTP inesperada: " + strconv.Itoa(e.status)
}

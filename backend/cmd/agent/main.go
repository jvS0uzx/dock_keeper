// Command agent é um coletor cross-platform que roda em qualquer host
// (Linux/macOS/Windows) e faz push das métricas do sistema para o painel
// vd_stats via POST /api/ingest/metrics.
//
// É o equivalente ao agente do Zabbix: instalado na estação, ele se anuncia
// sozinho no primeiro envio — não é preciso cadastrar a máquina antes.
package main

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/load"
	"github.com/shirou/gopsutil/v3/mem"
)

// Version identifica a build do agente no inventário. Ajuda o suporte a saber
// quais estações ainda rodam uma versão antiga.
const Version = "1.1.0"

const (
	defaultIntervalSec = 5
	httpTimeout        = 10 * time.Second

	// Janela de amostragem do CPU. Percent(0,...) mede desde a última chamada;
	// com intervalo curto uma janela explícita dá um valor útil.
	cpuSampleWindow = 500 * time.Millisecond
)

type metricsPayload struct {
	Hostname  string  `json:"hostname"`
	CPU       float64 `json:"cpu"`
	MemUsed   int64   `json:"mem_used"`
	MemTotal  int64   `json:"mem_total"`
	Load1     float64 `json:"load1"`
	DiskUsed  int64   `json:"disk_used"`
	DiskTotal int64   `json:"disk_total"`
	Uptime    float64 `json:"uptime"`

	// Campos usados pelo painel de suporte: identificam a máquina e o que o
	// técnico precisa saber antes de ir até ela.
	//
	// TemperatureC é ponteiro e sai do JSON quando a máquina não tem sensor
	// (VM, notebook sem hwmon exposto). Mandar 0 fazia o painel gravar zero e
	// exibir "0 °C" como se fosse leitura real.
	TemperatureC *float64 `json:"temperature_c,omitempty"`

	OS           string `json:"os"`
	Platform     string `json:"platform"`
	Arch         string `json:"arch"`
	LoggedUser   string `json:"logged_user"`
	SiteCode     string `json:"site_code"`
	AgentVersion string `json:"agent_version"`

	// MachineID identifica a máquina de forma estável. Hostname muda quando
	// alguém renomeia a estação, e o painel parte o histórico em duas séries
	// quando isso acontece.
	MachineID string `json:"machine_id"`

	// Intervalo configurado neste agente. Só ele sabe o valor: o painel deriva
	// daqui a janela de tolerância antes de dar a máquina como offline.
	ReportIntervalSec int `json:"report_interval_sec"`
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	serverURL := os.Getenv("AGENT_SERVER_URL")
	if serverURL == "" {
		log.Fatal("[Agent] AGENT_SERVER_URL não definido")
	}
	defaultHost, _ := os.Hostname()
	hostname := getenv("AGENT_HOSTNAME", defaultHost)
	// Código da unidade onde a estação fica. O painel usa para agrupar por
	// filial sem que o técnico precise cadastrar máquina por máquina.
	siteCode := strings.ToLower(strings.TrimSpace(os.Getenv("AGENT_SITE")))

	interval := defaultIntervalSec
	if raw := os.Getenv("AGENT_INTERVAL"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			interval = n
		}
	}

	endpoint := serverURL + "/api/ingest/metrics"
	client := &http.Client{Timeout: httpTimeout}

	cred, legado := resolverIdentidade(client, serverURL, hostname)
	maquina := machineID()

	log.Printf("[Agent] v%s iniciando: host=%s maquina=%s unidade=%q destino=%s intervalo=%ds",
		Version, hostname, maquina, siteCode, endpoint, interval)

	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()

	for {
		payload := collect(hostname, siteCode, interval)
		payload.MachineID = maquina
		if err := push(client, endpoint, cred, legado, payload); err != nil {
			log.Printf("[Agent] erro no envio: %v", err)
		} else {
			log.Printf("[Agent] enviado (cpu=%.1f%% mem=%d/%d temp=%s usuário=%q)",
				payload.CPU, payload.MemUsed, payload.MemTotal, formatTemp(payload.TemperatureC), payload.LoggedUser)
		}
		<-ticker.C
	}
}

// formatTemp mantém o log legível quando não há sensor, sem inventar um zero.
func formatTemp(t *float64) string {
	if t == nil {
		return "sem sensor"
	}
	return strconv.FormatFloat(*t, 'f', 1, 64) + "°C"
}

func collect(hostname, siteCode string, intervalSec int) metricsPayload {
	p := metricsPayload{
		Hostname:          hostname,
		SiteCode:          siteCode,
		AgentVersion:      Version,
		ReportIntervalSec: intervalSec,
	}

	if pcts, err := cpu.Percent(cpuSampleWindow, false); err == nil && len(pcts) > 0 {
		p.CPU = pcts[0]
	} else if err != nil {
		log.Printf("[Agent] cpu indisponível: %v", err)
	}

	if vm, err := mem.VirtualMemory(); err == nil {
		p.MemUsed = int64(vm.Used)
		p.MemTotal = int64(vm.Total)
	} else {
		log.Printf("[Agent] mem indisponível: %v", err)
	}

	// Load average não existe no Windows; tratamos o erro e mandamos 0.
	if avg, err := load.Avg(); err == nil {
		p.Load1 = avg.Load1
	}

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

// rootPath devolve o ponto de montagem raiz do sistema. No Windows o caminho
// "/" não existe e disk.Usage falharia.
func rootPath() string {
	if os.PathSeparator == '\\' {
		return "C:\\"
	}
	return "/"
}

// maxTemperature devolve a maior leitura dos sensores, em °C.
//
// Interessa ao suporte a pior temperatura da máquina, não a média: é ela que
// indica estação abafada ou cooler parado. Zero significa indisponível — VM,
// container e boa parte das máquinas virtuais não expõem sensor.
// maxTemperature devolve nil quando a máquina não expõe sensor térmico, para o
// painel distinguir "não medido" de uma leitura real.
func maxTemperature() *float64 {
	sensors, err := host.SensorsTemperatures()
	if err != nil || len(sensors) == 0 {
		return nil
	}

	var max float64
	for _, s := range sensors {
		// Leitura absurda significa sensor com escala errada; descarta em vez
		// de disparar alarme falso.
		if s.Temperature > max && s.Temperature < 150 {
			max = s.Temperature
		}
	}
	// Todos os sensores em zero ou fora de faixa: nenhuma leitura utilizável.
	if max == 0 {
		return nil
	}
	return &max
}

// activeUser devolve quem está com sessão aberta na máquina. Vazio quando
// ninguém está logado ou o sistema não expõe a informação.
func activeUser() string {
	users, err := host.Users()
	if err != nil || len(users) == 0 {
		return ""
	}

	// Sessões duplicadas são comuns (vários terminais do mesmo usuário);
	// devolve nomes distintos para o painel não repetir.
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

func push(client *http.Client, endpoint string, cred credential, legado string, payload metricsPayload) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
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
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &httpError{status: resp.StatusCode}
	}
	return nil
}

type httpError struct{ status int }

func (e *httpError) Error() string {
	return "resposta HTTP inesperada: " + strconv.Itoa(e.status)
}

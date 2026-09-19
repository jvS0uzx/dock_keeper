package ssh

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/alert"
	"github.com/jvS0uzx/dock_keeper/internal/database"
	"github.com/jvS0uzx/dock_keeper/internal/logstore"
	"github.com/jvS0uzx/dock_keeper/scripts"
)

var validContainerName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)

func IsValidContainerName(name string) bool {
	return validContainerName.MatchString(name)
}

const dockerLogsTailLines = 100

const lbFlushInterval = time.Second

const metricsLogEvery = 30

func parseDockerSize(sizeStr string) int64 {
	sizeStr = strings.TrimSpace(sizeStr)
	if sizeStr == "" {
		return 0
	}

	numStr, unitStr := sizeStr, ""
	for i, r := range sizeStr {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			numStr, unitStr = sizeStr[:i], sizeStr[i:]
			break
		}
	}

	val, err := strconv.ParseFloat(strings.TrimSpace(numStr), 64)
	if err != nil {
		return 0
	}

	multiplier := 1.0
	if len(unitStr) > 0 {
		switch unitStr[0] {
		case 'k', 'K':
			multiplier = 1 << 10
		case 'm', 'M':
			multiplier = 1 << 20
		case 'g', 'G':
			multiplier = 1 << 30
		case 't', 'T':
			multiplier = 1 << 40
		}
	}
	return int64(val * multiplier)
}

func parsePercent(p string) float64 {
	val, _ := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(p), "%"), 64)
	return val
}

type DockerPSPayload struct {
	DockerID string `json:"docker_id"`
	Name     string `json:"name"`
	Project  string `json:"project"`
	State    string `json:"state"`
	Status   string `json:"status"`
}

type DockerStatsPayload struct {
	DockerID   string `json:"docker_id"`
	CPUPercent string `json:"cpu_percent"`
	MemUsage   string `json:"mem_usage"`
}

type SysPayload struct {
	Uptime   float64              `json:"uptime"`
	HostCPU  *float64             `json:"host_cpu"`
	MemUsed  int64                `json:"mem_used"`
	MemTotal int64                `json:"mem_total"`
	Load1    *float64             `json:"load1"`
	DiskRoot string               `json:"disk_root"`
	PS       []DockerPSPayload    `json:"ps"`
	Stats    []DockerStatsPayload `json:"stats"`

	TemperatureC *float64 `json:"temperature_c"`

	NetRxBps *float64 `json:"net_rx_bps"`
	NetTxBps *float64 `json:"net_tx_bps"`
}

func runScript(session sessionWriter, t Target, script string) error {
	stdin, err := session.StdinPipe()
	if err != nil {
		return err
	}
	go func() {
		defer stdin.Close()
		if _, err := io.WriteString(stdin, scriptPrelude(t)+script); err != nil {
			log.Printf("[SSH] erro ao enviar script: %v", err)
		}
	}()
	return session.Start("bash -s")
}

type sessionWriter interface {
	StdinPipe() (io.WriteCloser, error)
	Start(cmd string) error
}

func StartStream(ctx context.Context, t Target) error {
	log.Printf("[RealTime] iniciando conexão SSH com %s...", t.addr())

	startHandshake := time.Now()
	client, session, err := openSession(t)
	if err != nil {
		return err
	}
	defer client.Close()
	defer session.Close()

	handshakeMs := float64(time.Since(startHandshake).Milliseconds())
	stopOnCancel(ctx, client, session)

	keepaliveCtx, stopKeepalive := context.WithCancel(ctx)
	defer stopKeepalive()
	defer forgetRTT(t.ID)
	go runKeepalive(keepaliveCtx, t, client, rttInterval(), keepaliveTimeout(), keepaliveMaxMisses(), rttProbeEnabled())

	stdout, err := session.StdoutPipe()
	if err != nil {
		return err
	}
	if err := runScript(session, t, scripts.StreamMetrics); err != nil {
		return err
	}

	containerCache := make(map[string]string)
	cycles := 0

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		var payload SysPayload
		if err := json.Unmarshal(scanner.Bytes(), &payload); err != nil {
			log.Printf("[RealTime] payload inválido de %s: %v", t.Host, err)
			continue
		}

		storeHostMetric(t, payload, handshakeMs)
		notifyStoppedContainers(t, payload.PS)
		storeContainerMetrics(t, payload, containerCache)

		if cycles%metricsLogEvery == 0 {
			log.Printf("[RealTime] %s gravado | handshake %.0fms | %d containers", t.Host, handshakeMs, len(payload.PS))
		}
		cycles++
	}
	if err := scanner.Err(); err != nil {
		return err
	}

	return session.Wait()
}

func storeHostMetric(t Target, payload SysPayload, handshakeMs float64) {
	var used, total int64
	if parts := strings.Split(payload.DiskRoot, ","); len(parts) == 2 {
		used, _ = strconv.ParseInt(parts[0], 10, 64)
		total, _ = strconv.ParseInt(parts[1], 10, 64)
	}

	metric := database.MetricServer{
		ServerID:        t.ID,
		UptimeSeconds:   payload.Uptime,
		DiskUsedBytes:   used,
		DiskTotalBytes:  total,
		CPUUsagePercent: payload.HostCPU,
		MemUsedBytes:    payload.MemUsed,
		MemTotalBytes:   payload.MemTotal,
		LoadAvg1:        payload.Load1,
		SSHHandshakeMs:  &handshakeMs,
		TemperatureC:    payload.TemperatureC,
		NetRxBps:        payload.NetRxBps,
		NetTxBps:        payload.NetTxBps,
		RTTMs:           latestRTT(t.ID),
		Timestamp:       time.Now().UTC(),
	}
	if err := database.DB.Create(&metric).Error; err != nil {
		log.Printf("[RealTime] erro ao gravar métrica de %s: %v", t.Host, err)
	}
}

func notifyStoppedContainers(t Target, ps []DockerPSPayload) {
	for _, c := range ps {
		if c.State != "running" && c.State != "" {
			alert.Notify("container_down:"+t.ID+":"+c.Name,
				fmt.Sprintf("[ALERTA] Container %s está %s em %s", c.Name, c.State, t.Host))
		}
	}
}

func storeContainerMetrics(t Target, payload SysPayload, cache map[string]string) {
	statsByID := make(map[string]DockerStatsPayload, len(payload.Stats))
	for _, s := range payload.Stats {
		statsByID[s.DockerID] = s
	}

	metrics := make([]database.MetricContainer, 0, len(payload.PS))
	for _, ps := range payload.PS {
		containerID, cached := cache[ps.DockerID]
		if !cached {
			var container database.Container
			if err := database.DB.Where("server_id = ? AND docker_id = ?", t.ID, ps.DockerID).
				FirstOrCreate(&container, database.Container{
					ServerID: t.ID, DockerID: ps.DockerID, Name: ps.Name, ProjectDir: ps.Project,
				}).Error; err != nil {
				log.Printf("[RealTime] erro ao registrar container %s: %v", ps.Name, err)
				continue
			}
			if container.ProjectDir != ps.Project {
				database.DB.Model(&container).Update("project_dir", ps.Project)
			}
			containerID = container.ID
			cache[ps.DockerID] = containerID
		}

		var memUsed, memLimit int64
		var cpuPercent float64
		if stat, ok := statsByID[ps.DockerID]; ok {
			if parts := strings.Split(stat.MemUsage, "/"); len(parts) == 2 {
				memUsed = parseDockerSize(parts[0])
				memLimit = parseDockerSize(parts[1])
			}
			cpuPercent = parsePercent(stat.CPUPercent)
		}

		metrics = append(metrics, database.MetricContainer{
			ContainerID: containerID, CPUUsagePercent: cpuPercent,
			MemUsedBytes: memUsed, MemLimitBytes: memLimit,
			State: ps.State, Status: ps.Status, Timestamp: time.Now().UTC(),
		})
	}

	if len(metrics) > 0 {
		if err := database.DB.Create(&metrics).Error; err != nil {
			log.Printf("[RealTime] erro ao gravar métricas de container de %s: %v", t.Host, err)
		}
	}
}

type lbKey struct {
	Upstream   string
	ServerName string
	Status     string
}

type lbCounter struct {
	mu     sync.Mutex
	counts map[lbKey]int

	serverID *string
	siteID   *uint
}

func newLBCounter(serverID *string, siteID *uint) *lbCounter {
	return &lbCounter{
		counts:   make(map[lbKey]int),
		serverID: serverID,
		siteID:   siteID,
	}
}

func (c *lbCounter) add(key lbKey) {
	c.mu.Lock()
	c.counts[key]++
	c.mu.Unlock()
}

func (c *lbCounter) flush() {
	c.mu.Lock()
	pending := c.counts
	c.counts = make(map[lbKey]int)
	c.mu.Unlock()

	for key, count := range pending {
		row := database.MetricLoadBalancer{
			UpstreamAddr:  key.Upstream,
			ServerName:    key.ServerName,
			Status:        key.Status,
			ServerID:      c.serverID,
			SiteID:        c.siteID,
			RequestsCount: count,
			Timestamp:     time.Now().UTC(),
		}
		if err := database.DB.Create(&row).Error; err != nil {
			log.Printf("[Nginx] erro ao gravar contador do LB: %v", err)
		}
	}
}

func lbOrigin(t Target) (*string, *uint) {
	if t.ID == "" || database.DB == nil {
		return nil, nil
	}

	var server database.Server
	if err := database.DB.Select("id", "site_id").First(&server, "id = ?", t.ID).Error; err != nil {
		log.Printf("[Nginx] unidade de %s não resolvida, métrica do LB fica sem recorte: %v", t.Name, err)
		return nil, nil
	}

	id := server.ID
	return &id, server.SiteID
}

func StartNginxStream(ctx context.Context, t Target) error {
	log.Printf("[RealTime] iniciando stream do NGINX em %s...", t.addr())

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
	if err := runScript(session, t, scripts.StreamNginx); err != nil {
		return err
	}

	counter := newLBCounter(lbOrigin(t))
	flushCtx, stopFlush := context.WithCancel(ctx)
	defer stopFlush()
	go func() {
		ticker := time.NewTicker(lbFlushInterval)
		defer ticker.Stop()
		for {
			select {
			case <-flushCtx.Done():
				counter.flush()
				return
			case <-ticker.C:
				counter.flush()
			}
		}
	}()

	health := newLBHealth(t)
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		if e, ok := parseNginxEntry(scanner.Text()); ok {
			counter.add(e.bucket())
			health.observe(e.Upstream, e.Code, time.Now())
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}

	return session.Wait()
}

var trackedStatuses = []string{"500", "502", "503", "504", "429", "404", "400"}

func parseNginxLine(line string) (lbKey, bool) {
	e, ok := parseNginxEntry(line)
	if !ok {
		return lbKey{}, false
	}
	return e.bucket(), true
}

func (e nginxEntry) bucket() lbKey {
	status := "200"
	code := strconv.Itoa(e.Code)
	for _, tracked := range trackedStatuses {
		if code == tracked {
			status = tracked
			break
		}
	}
	return lbKey{Upstream: e.Upstream, ServerName: e.ServerName, Status: status}
}

func StreamDockerLogs(ctx context.Context, t Target, containerName string, w http.ResponseWriter, flusher http.Flusher) error {
	if !validContainerName.MatchString(containerName) {
		return fmt.Errorf("nome de container inválido: %q", containerName)
	}

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
	stderr, err := session.StderrPipe()
	if err != nil {
		return err
	}

	cmd := fmt.Sprintf("docker logs -f --tail %d -- %s", dockerLogsTailLines, containerName)
	if err := session.Start(cmd); err != nil {
		return err
	}

	stream := newSSEWriter(w, flusher)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			stream.send(scanner.Text())
		}
	}()

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		logstore.Save(t.ID, "container", containerName, line)
		stream.send(line)
	}
	wg.Wait()
	if err := scanner.Err(); err != nil {
		return err
	}

	return session.Wait()
}

package ssh

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/joaov/vd_stats/internal/alert"
	"github.com/joaov/vd_stats/internal/database"
	"github.com/joaov/vd_stats/internal/logstore"
	"golang.org/x/crypto/ssh"
)

// Nomes de container Docker só têm [a-zA-Z0-9_.-]. Bloqueia injeção de comando
// via query string no stream de logs (rodaria como root na VPS).
var validContainerName = regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)

func parseDockerSize(sizeStr string) int64 {
	sizeStr = strings.TrimSpace(sizeStr)
	if sizeStr == "" {
		return 0
	}

	// Separa a parte numérica da unidade de medida
	var numStr, unitStr string
	for i, r := range sizeStr {
		// Ao encontrar a primeira letra, dividimos a string
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			numStr = sizeStr[:i]
			unitStr = sizeStr[i:]
			break
		}
	}

	// Se não encontrou unidade, a string inteira é o número
	if numStr == "" {
		numStr = sizeStr
	}

	val, err := strconv.ParseFloat(strings.TrimSpace(numStr), 64)
	if err != nil {
		return 0
	}

	var multiplier float64 = 1
	if len(unitStr) > 0 {
		// Verifica apenas o primeiro caractere da unidade (K, M, G, T)
		switch unitStr[0] {
		case 'k', 'K':
			multiplier = 1024
		case 'm', 'M':
			multiplier = 1024 * 1024
		case 'g', 'G':
			multiplier = 1024 * 1024 * 1024
		case 't', 'T':
			multiplier = 1024 * 1024 * 1024 * 1024
		}
	}

	return int64(val * multiplier)
}

func parsePercent(p string) float64 {
	p = strings.TrimSuffix(strings.TrimSpace(p), "%")
	val, _ := strconv.ParseFloat(p, 64)
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
	HostCPU  float64              `json:"host_cpu"`
	MemUsed  int64                `json:"mem_used"`
	MemTotal int64                `json:"mem_total"`
	Load1    float64              `json:"load1"`
	DiskRoot string               `json:"disk_root"`
	PS       []DockerPSPayload    `json:"ps"`
	Stats    []DockerStatsPayload `json:"stats"`
}

func StartStream(ctx context.Context, serverID, host, user, keyPath string) error {

	keyPath = strings.Replace(keyPath, "~", os.Getenv("HOME"), 1)
	keyBytes, err := os.ReadFile(keyPath)
	if err != nil {
		return err
	}

	signer, err := ssh.ParsePrivateKey(keyBytes)
	if err != nil {
		return err
	}

	config := &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	hostPort := fmt.Sprintf("%s:22", host)
	log.Printf("[RealTime] Iniciando conexão SSH com a VPS %s...", hostPort)

	startPing := time.Now()
	client, err := ssh.Dial("tcp", hostPort, config)
	if err != nil {
		return err
	}
	defer client.Close()

	latencyMs := float64(time.Since(startPing).Milliseconds())

	session, err := client.NewSession()
	if err != nil {
		return err
	}

	go func() {
		<-ctx.Done()
		session.Close()
		client.Close()
	}()
	defer session.Close()

	stdout, err := session.StdoutPipe()
	if err != nil {
		return err
	}

	scriptBytes, err := os.ReadFile("scripts/stream_metrics.sh")
	if err != nil {
		return fmt.Errorf("erro ao ler script de métricas: %w", err)
	}

	stdin, err := session.StdinPipe()
	if err != nil {
		return err
	}

	// Aqui podemos injetar params base64 no futuro (ex: para IPs específicos)
	finalScript := string(scriptBytes)

	go func() {
		defer stdin.Close()
		stdin.Write([]byte(finalScript))
	}()

	err = session.Start("bash -s")
	if err != nil {
		return err
	}

	// Cache de containers na memória para evitar SELECT (FirstOrCreate) a cada loop
	containerCache := make(map[string]string)

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()

		var payload SysPayload
		if err := json.Unmarshal([]byte(line), &payload); err != nil {
			log.Printf("Erro de parse: %v, Linha: %s", err, line)
			continue
		}

		diskParts := strings.Split(payload.DiskRoot, ",")
		var dUsed, dTotal int64
		if len(diskParts) == 2 {
			dUsed, _ = strconv.ParseInt(diskParts[0], 10, 64)
			dTotal, _ = strconv.ParseInt(diskParts[1], 10, 64)
		}

		database.DB.Create(&database.MetricServer{
			ServerID: serverID, UptimeSeconds: payload.Uptime,
			DiskUsedBytes: dUsed, DiskTotalBytes: dTotal,
			CPUUsagePercent: payload.HostCPU,
			MemUsedBytes:    payload.MemUsed, MemTotalBytes: payload.MemTotal,
			LoadAvg1:      payload.Load1,
			PingLatencyMs: latencyMs, Timestamp: time.Now().UTC(),
		})

		// Alerta de container que caiu (fora de 'running'), com cooldown por container.
		for _, ps := range payload.PS {
			if ps.State != "running" && ps.State != "" {
				alert.Notify("container_down:"+serverID+":"+ps.Name,
					fmt.Sprintf("[ALERTA] Container *%s* está *%s* em %s", ps.Name, ps.State, host))
			}
		}

		var containerMetrics []database.MetricContainer

		statsMap := make(map[string]DockerStatsPayload)
		for _, s := range payload.Stats {
			statsMap[s.DockerID] = s
		}

		for _, ps := range payload.PS {
			containerID, exists := containerCache[ps.DockerID]
			if !exists {
				var container database.Container
				database.DB.Where("server_id = ? AND docker_id = ?", serverID, ps.DockerID).FirstOrCreate(&container, database.Container{
					ServerID: serverID, DockerID: ps.DockerID, Name: ps.Name, ProjectDir: ps.Project,
				})
				if container.ProjectDir != ps.Project {
					database.DB.Model(&container).Update("project_dir", ps.Project)
				}
				containerID = container.ID
				containerCache[ps.DockerID] = containerID
			}

			var memUsed, memLimit int64
			var cpuPercent float64

			if stat, ok := statsMap[ps.DockerID]; ok {
				memParts := strings.Split(stat.MemUsage, "/")
				if len(memParts) == 2 {
					memUsed = parseDockerSize(memParts[0])
					memLimit = parseDockerSize(memParts[1])
				}
				cpuPercent = parsePercent(stat.CPUPercent)
			}

			containerMetrics = append(containerMetrics, database.MetricContainer{
				ContainerID: containerID, CPUUsagePercent: cpuPercent,
				MemUsedBytes: memUsed, MemLimitBytes: memLimit,
				State: ps.State, Status: ps.Status, Timestamp: time.Now().UTC(),
			})
		}

		// Batch Insert de todas as métricas dos containers de uma só vez
		if len(containerMetrics) > 0 {
			database.DB.Create(&containerMetrics)
		}

		log.Printf("[GRAVADO] %s | Latência: %.0fms | Containers: %d", host, latencyMs, len(payload.PS))

	}
	return session.Wait()
}

func StartNginxStream(ctx context.Context, serverID, host, user, keyPath string) error {

	keyPath = strings.Replace(keyPath, "~", os.Getenv("HOME"), 1)
	keyBytes, err := os.ReadFile(keyPath)
	if err != nil {
		return err
	}

	signer, err := ssh.ParsePrivateKey(keyBytes)
	if err != nil {
		return err
	}

	config := &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	hostPort := fmt.Sprintf("%s:22", host)
	log.Printf("[RealTime] Iniciando stream do NGINX na VPS %s...", hostPort)

	client, err := ssh.Dial("tcp", hostPort, config)
	if err != nil {
		return err
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return err
	}

	go func() {
		<-ctx.Done()
		session.Close()
		client.Close()
	}()
	defer session.Close()

	stdout, err := session.StdoutPipe()
	if err != nil {
		return err
	}

	scriptBytes, err := os.ReadFile("scripts/stream_nginx.sh")
	if err != nil {
		return fmt.Errorf("erro ao ler script nginx: %w", err)
	}

	stdin, err := session.StdinPipe()
	if err != nil {
		return err
	}

	go func() {
		defer stdin.Close()
		stdin.Write(scriptBytes)
	}()

	err = session.Start("bash -s")
	if err != nil {
		return err
	}

	scanner := bufio.NewScanner(stdout)

	type LbKey struct {
		Upstream   string
		ServerName string
		Status     string
	}
	var mu sync.Mutex
	counts := make(map[LbKey]int)

	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for range ticker.C {
			mu.Lock()
			if len(counts) > 0 {
				for key, count := range counts {
					database.DB.Create(&database.MetricLoadBalancer{
						UpstreamAddr:  key.Upstream,
						ServerName:    key.ServerName,
						Status:        key.Status,
						RequestsCount: count,
						Timestamp:     time.Now().UTC(),
					})
				}
				counts = make(map[LbKey]int)
			}
			mu.Unlock()
		}
	}()

	for scanner.Scan() {
		line := scanner.Text()
		idxTo := strings.Index(line, " to: ")
		if idxTo != -1 {
			prefix := line[:idxTo]
			partsPrefix := strings.Split(prefix, " - ")
			serverName := ""
			if len(partsPrefix) > 0 {
				serverName = strings.TrimSpace(partsPrefix[len(partsPrefix)-1])
			}

			rem := line[idxTo+5:]
			parts := strings.SplitN(rem, ": ", 2)
			if len(parts) >= 2 {
				upstream := strings.TrimSpace(parts[0])
				if upstream == "-" {
					upstream = "Local (Nginx/Cache)"
				}
				if upstream != "" {
					status := "200"
					reqStatusStr := parts[1]
					if strings.Contains(reqStatusStr, " 50") || strings.Contains(reqStatusStr, " 40") {
						if strings.Contains(reqStatusStr, " 500") {
							status = "500"
						}
						if strings.Contains(reqStatusStr, " 502") {
							status = "502"
						}
						if strings.Contains(reqStatusStr, " 503") {
							status = "503"
						}
						if strings.Contains(reqStatusStr, " 504") {
							status = "504"
						}
						if strings.Contains(reqStatusStr, " 404") {
							status = "404"
						}
						if strings.Contains(reqStatusStr, " 400") {
							status = "400"
						}
						if strings.Contains(reqStatusStr, " 429") {
							status = "429"
						}
					}

					key := LbKey{Upstream: upstream, ServerName: serverName, Status: status}
					mu.Lock()
					counts[key]++
					mu.Unlock()
				}
			}
		}
	}
	return session.Wait()
}

func StreamDockerLogs(ctx context.Context, serverID, host, user, keyPath, containerName string, w http.ResponseWriter, flusher http.Flusher) error {
	if !validContainerName.MatchString(containerName) {
		return fmt.Errorf("nome de container inválido: %q", containerName)
	}
	keyPath = strings.Replace(keyPath, "~", os.Getenv("HOME"), 1)
	keyBytes, err := os.ReadFile(keyPath)
	if err != nil {
		return err
	}

	signer, err := ssh.ParsePrivateKey(keyBytes)
	if err != nil {
		return err
	}

	config := &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	hostPort := fmt.Sprintf("%s:22", host)
	client, err := ssh.Dial("tcp", hostPort, config)
	if err != nil {
		return err
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return err
	}

	go func() {
		<-ctx.Done()
		session.Close()
		client.Close()
	}()
	defer session.Close()

	stdout, err := session.StdoutPipe()
	if err != nil {
		return err
	}

	stderr, err := session.StderrPipe()
	if err != nil {
		return err
	}

	err = session.Start(fmt.Sprintf("docker logs -f --tail 100 %s", containerName))
	if err != nil {
		return err
	}

	go func() {
		scannerErr := bufio.NewScanner(stderr)
		for scannerErr.Scan() {
			line := scannerErr.Text()
			fmt.Fprintf(w, "data: %s\n\n", line)
			flusher.Flush()
		}
	}()

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		logstore.Save(serverID, "container", containerName, line)
		fmt.Fprintf(w, "data: %s\n\n", line)
		flusher.Flush()
	}

	return session.Wait()
}

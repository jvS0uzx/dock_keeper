package ssh

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/joaov/vd_stats/internal/database"
	"golang.org/x/crypto/ssh"
)

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

type ContainerPayload struct {
	DockerID   string `json:"docker_id"`
	Name       string `json:"name"`
	CPUPercent string `json:"cpu_percent"`
	MemUsage   string `json:"mem_usage"`
}

type SysPayload struct {
	Uptime     float64            `json:"uptime"`
	DiskRoot   string             `json:"disk_root"`
	Containers []ContainerPayload `json:"containers"`
}

func StartStream(ctx context.Context, serverID, host, user, keyPath string) error {

	keyPath = strings.Replace(keyPath, "~", os.Getenv("HOME"), 1)
	keyBytes, err := os.ReadFile(keyPath)
	if err != nil { return err }

	signer, err := ssh.ParsePrivateKey(keyBytes)
	if err != nil { return err }

	config := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout: 10 * time.Second,
	}

	hostPort := fmt.Sprintf("%s:22", host)
	log.Printf("[RealTime] Iniciando conexão SSH com a VPS %s...", hostPort)
	
	startPing := time.Now()
	client, err := ssh.Dial("tcp", hostPort, config)
	if err != nil { return err }
	defer client.Close()
	
	latencyMs := float64(time.Since(startPing).Milliseconds())

	session, err := client.NewSession()
	if err != nil { return err }

	go func() {
		<-ctx.Done()
		session.Close()
		client.Close()
	}()
	defer session.Close()

	stdout, err := session.StdoutPipe()
	if err != nil { return err }

	scriptBytes, err := os.ReadFile("scripts/stream_metrics.sh")
	if err != nil { return fmt.Errorf("erro ao ler script de métricas: %w", err) }

	stdin, err := session.StdinPipe()
	if err != nil { return err }

	// Aqui podemos injetar params base64 no futuro (ex: para IPs específicos)
	finalScript := string(scriptBytes)

	go func() {
		defer stdin.Close()
		stdin.Write([]byte(finalScript))
	}()

	err = session.Start("bash -s")
	if err != nil { return err }

	// Cache de containers na memória para evitar SELECT (FirstOrCreate) a cada loop
	containerCache := make(map[string]string)

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		
		var payload SysPayload
		if err := json.Unmarshal([]byte(line), &payload); err != nil { continue }

		diskParts := strings.Split(payload.DiskRoot, ",")
		var dUsed, dTotal int64
		if len(diskParts) == 2 {
			dUsed, _ = strconv.ParseInt(diskParts[0], 10, 64)
			dTotal, _ = strconv.ParseInt(diskParts[1], 10, 64)
		}

		database.DB.Create(&database.MetricServer{
			ServerID: serverID, UptimeSeconds: payload.Uptime,
			DiskUsedBytes: dUsed, DiskTotalBytes: dTotal,
			PingLatencyMs: latencyMs, Timestamp: time.Now().UTC(),
		})

		var containerMetrics []database.MetricContainer

		for _, raw := range payload.Containers {
			containerID, exists := containerCache[raw.DockerID]
			if !exists {
				// Só vai ao banco se for um container novo que ainda não está no cache
				var container database.Container
				database.DB.Where("server_id = ? AND docker_id = ?", serverID, raw.DockerID).FirstOrCreate(&container, database.Container{
					ServerID: serverID, DockerID: raw.DockerID, Name: raw.Name,
				})
				containerID = container.ID
				containerCache[raw.DockerID] = containerID
			}

			memParts := strings.Split(raw.MemUsage, "/")
			var memUsed, memLimit int64
			if len(memParts) == 2 {
				memUsed = parseDockerSize(memParts[0])
				memLimit = parseDockerSize(memParts[1])
			}

			containerMetrics = append(containerMetrics, database.MetricContainer{
				ContainerID: containerID, CPUUsagePercent: parsePercent(raw.CPUPercent),
				MemUsedBytes: memUsed, MemLimitBytes: memLimit,
				Status: "running", Timestamp: time.Now().UTC(),
			})
		}
		
		// Batch Insert de todas as métricas dos containers de uma só vez
		if len(containerMetrics) > 0 {
			database.DB.Create(&containerMetrics)
		}

		log.Printf("[GRAVADO] %s | Latência: %.0fms | Containers: %d", host, latencyMs, len(payload.Containers))
	}
	return session.Wait()
}

func StartNginxStream(ctx context.Context, serverID, host, user, keyPath string) error {

	keyPath = strings.Replace(keyPath, "~", os.Getenv("HOME"), 1)
	keyBytes, err := os.ReadFile(keyPath)
	if err != nil { return err }

	signer, err := ssh.ParsePrivateKey(keyBytes)
	if err != nil { return err }

	config := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout: 10 * time.Second,
	}

	hostPort := fmt.Sprintf("%s:22", host)
	log.Printf("[RealTime] Iniciando stream do NGINX na VPS %s...", hostPort)
	
	client, err := ssh.Dial("tcp", hostPort, config)
	if err != nil { return err }
	defer client.Close()

	session, err := client.NewSession()
	if err != nil { return err }

	go func() {
		<-ctx.Done()
		session.Close()
		client.Close()
	}()
	defer session.Close()

	stdout, err := session.StdoutPipe()
	if err != nil { return err }

	scriptBytes, err := os.ReadFile("scripts/stream_nginx.sh")
	if err != nil { return fmt.Errorf("erro ao ler script nginx: %w", err) }

	stdin, err := session.StdinPipe()
	if err != nil { return err }

	go func() {
		defer stdin.Close()
		stdin.Write(scriptBytes)
	}()

	err = session.Start("bash -s")
	if err != nil { return err }

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
						if strings.Contains(reqStatusStr, " 500") { status = "500" }
						if strings.Contains(reqStatusStr, " 502") { status = "502" }
						if strings.Contains(reqStatusStr, " 503") { status = "503" }
						if strings.Contains(reqStatusStr, " 504") { status = "504" }
						if strings.Contains(reqStatusStr, " 404") { status = "404" }
						if strings.Contains(reqStatusStr, " 400") { status = "400" }
						if strings.Contains(reqStatusStr, " 429") { status = "429" }
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

func StreamDockerLogs(ctx context.Context, host, user, keyPath, containerName string, w http.ResponseWriter, flusher http.Flusher) error {
	keyPath = strings.Replace(keyPath, "~", os.Getenv("HOME"), 1)
	keyBytes, err := os.ReadFile(keyPath)
	if err != nil { return err }

	signer, err := ssh.ParsePrivateKey(keyBytes)
	if err != nil { return err }

	config := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout: 10 * time.Second,
	}

	hostPort := fmt.Sprintf("%s:22", host)
	client, err := ssh.Dial("tcp", hostPort, config)
	if err != nil { return err }
	defer client.Close()

	session, err := client.NewSession()
	if err != nil { return err }

	go func() {
		<-ctx.Done()
		session.Close()
		client.Close()
	}()
	defer session.Close()

	stdout, err := session.StdoutPipe()
	if err != nil { return err }

	stderr, err := session.StderrPipe()
	if err != nil { return err }

	err = session.Start(fmt.Sprintf("docker logs -f --tail 100 %s", containerName))
	if err != nil { return err }

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
		fmt.Fprintf(w, "data: %s\n\n", line)
		flusher.Flush()
	}

	return session.Wait()
}

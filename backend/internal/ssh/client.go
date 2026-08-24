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

	"github.com/joaov/vd_stats/internal/alert"
	"github.com/joaov/vd_stats/internal/database"
	"github.com/joaov/vd_stats/internal/logstore"
	"github.com/joaov/vd_stats/scripts"
)

// Nomes de container Docker só têm [a-zA-Z0-9_.-] e começam por alfanumérico.
// Bloqueia injeção de comando via query string no stream de logs (rodaria como
// root na VPS). O primeiro caractere é parte da defesa: um container chamado
// "-f" passaria pela regex antiga e o docker o leria como flag, não como alvo.
// O "--" nos comandos abaixo é a segunda camada, para o dia em que a regex
// afrouxar de novo.
var validContainerName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)

// IsValidContainerName expõe a regex acima para o chamador HTTP.
//
// Existe porque a linha de auditoria da ação é gravada ANTES de
// RunContainerAction ter chance de recusar o nome, e o nome vai no detalhe da
// linha. Sem esta conferência prévia, o corpo da requisição escreveria texto
// arbitrário na tabela de auditoria. Copiar a regex no pacote HTTP faria as
// duas divergirem no primeiro ajuste.
func IsValidContainerName(name string) bool {
	return validContainerName.MatchString(name)
}

// Quantas linhas de histórico o `docker logs` entrega ao abrir o stream.
const dockerLogsTailLines = 100

// Intervalo de gravação dos contadores agregados do access log do Nginx.
const lbFlushInterval = time.Second

// Só loga a gravação de métricas a cada N ciclos: a coleta roda a cada 1-2s por
// host e uma linha por ciclo entope o journal.
const metricsLogEvery = 30

func parseDockerSize(sizeStr string) int64 {
	sizeStr = strings.TrimSpace(sizeStr)
	if sizeStr == "" {
		return 0
	}

	// Separa a parte numérica da unidade de medida.
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
	HostCPU  float64              `json:"host_cpu"`
	MemUsed  int64                `json:"mem_used"`
	MemTotal int64                `json:"mem_total"`
	Load1    float64              `json:"load1"`
	DiskRoot string               `json:"disk_root"`
	PS       []DockerPSPayload    `json:"ps"`
	Stats    []DockerStatsPayload `json:"stats"`

	// Ponteiro porque o script remoto omite o campo em host sem sensor
	// térmico (VM, container). Zero seria confundido com leitura real.
	TemperatureC *float64 `json:"temperature_c"`
}

// runScript sobe o script pelo stdin da sessão remota (`bash -s`).
func runScript(session sessionWriter, script string) error {
	stdin, err := session.StdinPipe()
	if err != nil {
		return err
	}
	go func() {
		defer stdin.Close()
		// O prelúdio vai antes do script: são atribuições de variável que o
		// script lê com fallback embutido, então executá-lo à mão fora do painel
		// continua funcionando.
		if _, err := io.WriteString(stdin, scriptPrelude()+script); err != nil {
			log.Printf("[SSH] erro ao enviar script: %v", err)
		}
	}()
	return session.Start("bash -s")
}

type sessionWriter interface {
	StdinPipe() (io.WriteCloser, error)
	Start(cmd string) error
}

// StartStream abre a sessão de coleta de métricas do host e dos containers.
// Bloqueia até a sessão cair ou o contexto ser cancelado.
func StartStream(ctx context.Context, t Target) error {
	log.Printf("[RealTime] iniciando conexão SSH com %s...", t.addr())

	startHandshake := time.Now()
	client, session, err := openSession(t)
	if err != nil {
		return err
	}
	defer client.Close()
	defer session.Close()

	// Isto mede o handshake SSH inteiro (TCP + troca de chaves), uma única vez,
	// e é o que fica gravado em todas as amostras da sessão — que dura horas.
	// Está correto para o que a métrica passou a se chamar: é o custo de abrir
	// a conexão que produziu estas amostras. Não confundir com RTT: o valor
	// típico aqui é 1000-1400 ms, uma ordem de grandeza acima da latência de
	// rede. Medir RTT de verdade exigiria um prober separado (ICMP/TCP), fora
	// do escopo deste stream.
	handshakeMs := float64(time.Since(startHandshake).Milliseconds())
	stopOnCancel(ctx, client, session)

	stdout, err := session.StdoutPipe()
	if err != nil {
		return err
	}
	if err := runScript(session, scripts.StreamMetrics); err != nil {
		return err
	}

	// Cache de containers em memória para evitar um SELECT por ciclo.
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

// lbKey identifica uma combinação upstream/vhost/status dentro da janela.
type lbKey struct {
	Upstream   string
	ServerName string
	Status     string
}

// lbCounter acumula requisições do access log e descarrega no banco por
// intervalo, em vez de um INSERT por linha de log.
//
// serverID e siteID são resolvidos uma vez, na abertura do stream, e carregados
// aqui: a linha do balanceador precisa saber de que host veio para o painel
// conseguir recortá-la por unidade, e consultar isso no flush — ou pior, por
// linha de log — pagaria uma ida ao banco por informação que não muda enquanto
// o stream vive.
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

// lbOrigin descobre de que host e de que unidade sai este stream.
//
// Roda uma vez por abertura de stream, não por flush: t.ID já é o id do
// database.Server, e a unidade dele não muda enquanto a conexão vive. Falha de
// consulta devolve nulo em vez de abortar — a métrica do balanceador sem
// unidade ainda serve a quem tem concessão global, e derrubar a coleta por
// causa do recorte seria trocar um problema pequeno por um grande.
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

// StartNginxStream acompanha o access log do balanceador e agrega as
// requisições por upstream/status.
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
	if err := runScript(session, scripts.StreamNginx); err != nil {
		return err
	}

	counter := newLBCounter(lbOrigin(t))
	// O flush morre junto com o stream; antes o ticker ficava vivo para sempre
	// e cada reconexão (a cada 5s em caso de falha) vazava mais uma goroutine.
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

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		if key, ok := parseNginxLine(scanner.Text()); ok {
			counter.add(key)
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}

	return session.Wait()
}

// Status HTTP que o painel destaca. Qualquer outro é tratado como 200.
var trackedStatuses = []string{"500", "502", "503", "504", "429", "404", "400"}

// parseNginxLine lê "<vhost> - <...> to: <upstream>: <req> <status> ..." e
// devolve a chave de agregação.
func parseNginxLine(line string) (lbKey, bool) {
	idxTo := strings.Index(line, " to: ")
	if idxTo == -1 {
		return lbKey{}, false
	}

	prefixParts := strings.Split(line[:idxTo], " - ")
	serverName := strings.TrimSpace(prefixParts[len(prefixParts)-1])

	parts := strings.SplitN(line[idxTo+len(" to: "):], ": ", 2)
	if len(parts) < 2 {
		return lbKey{}, false
	}

	upstream := strings.TrimSpace(parts[0])
	if upstream == "-" {
		upstream = "Local (Nginx/Cache)"
	}
	if upstream == "" {
		return lbKey{}, false
	}

	status := "200"
	for _, code := range trackedStatuses {
		if strings.Contains(parts[1], " "+code) {
			status = code
			break
		}
	}
	return lbKey{Upstream: upstream, ServerName: serverName, Status: status}, true
}

// StreamDockerLogs transmite `docker logs -f` de um container por SSE.
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

	// docker logs manda a saída da aplicação em stderr também; as duas pontas
	// escrevem no mesmo SSE, por isso o writer é serializado.
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

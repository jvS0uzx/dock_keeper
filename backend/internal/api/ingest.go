package api

import (
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/joaov/vd_stats/internal/database"
	"gorm.io/gorm"
)

// ingestPayload é o corpo enviado pelo agente de coleta (cmd/agent).
type ingestPayload struct {
	Hostname  string  `json:"hostname"`
	CPU       float64 `json:"cpu"`
	MemUsed   int64   `json:"mem_used"`
	MemTotal  int64   `json:"mem_total"`
	Load1     float64 `json:"load1"`
	DiskUsed  int64   `json:"disk_used"`
	DiskTotal int64   `json:"disk_total"`
	Uptime    float64 `json:"uptime"`

	// Ponteiro: o agente omite o campo quando a máquina não tem sensor. Agente
	// antigo ainda manda 0 nesse caso, tratado em temperatureOf.
	TemperatureC *float64 `json:"temperature_c"`

	OS           string `json:"os"`
	Platform     string `json:"platform"`
	Arch         string `json:"arch"`
	LoggedUser   string `json:"logged_user"`
	SiteCode     string `json:"site_code"`
	AgentVersion string `json:"agent_version"`

	// MachineID é o identificador que o agente gera no primeiro boot e persiste
	// em disco. Vazio em agente antigo, e por isso a chave por hostname continua
	// existindo como fallback.
	MachineID string `json:"machine_id"`

	// Intervalo que o agente diz estar usando entre um push e o próximo.
	// Guardado no Server para derivar a janela de "online" — ver
	// database.LiveWindowFor.
	ReportIntervalSec int `json:"report_interval_sec"`
}

// temperatureOf normaliza a temperatura recebida do agente.
//
// Zero vira nulo: nenhuma máquina em operação está a 0 °C, e um agente sem
// sensor mandava exatamente isso. Gravar o zero fazia o painel exibir "0 °C"
// como se fosse leitura real.
func temperatureOf(p ingestPayload) *float64 {
	if p.TemperatureC == nil || *p.TemperatureC == 0 {
		return nil
	}
	return p.TemperatureC
}

// IngestHandler recebe métricas via push de agentes (Kind="agent").
// Autentica pelo header X-Agent-Token contra a env AGENT_INGEST_TOKEN.
// Não deve ser registrado com CORS de escrita público sem cuidado; é uma rota máquina-a-máquina.
func IngestHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	cred, err := authenticateDevice(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "credencial de dispositivo inválida")
		return
	}

	var p ingestPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	p.Hostname = strings.TrimSpace(p.Hostname)
	if p.Hostname == "" {
		writeError(w, http.StatusBadRequest, "hostname required")
		return
	}

	hostIP := r.RemoteAddr
	if h, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		hostIP = h
	}

	// A unidade sai da CREDENCIAL, não do corpo. Com o token compartilhado
	// qualquer portador declarava a filial que quisesse e injetava métrica falsa
	// nela, disparando ou silenciando alerta de máquina que nem existe.
	declarada := siteOfAgent(p)
	if !cred.siteMatches(declarada) {
		auditIngestSiteMismatch(cred, p, declarada)
		writeError(w, http.StatusConflict, "unidade declarada não confere com a credencial do dispositivo")
		return
	}
	siteID := cred.SiteID
	if siteID == nil {
		siteID = declarada
	}

	server, err := findOrCreateAgentServer(p.Hostname, p.MachineID, hostIP, siteID)
	if err != nil {
		log.Printf("[Ingest] erro no upsert do servidor %s: %v", p.Hostname, err)
		writeError(w, http.StatusInternalServerError, "server upsert failed")
		return
	}

	if err := database.DB.Model(&server).Updates(hostFacts(p, hostIP, siteID)).Error; err != nil {
		log.Printf("[Ingest] erro ao atualizar os dados de %s: %v", p.Hostname, err)
	}

	metric := database.MetricServer{
		ServerID:        server.ID,
		CPUUsagePercent: p.CPU,
		MemUsedBytes:    p.MemUsed,
		MemTotalBytes:   p.MemTotal,
		LoadAvg1:        p.Load1,
		DiskUsedBytes:   p.DiskUsed,
		DiskTotalBytes:  p.DiskTotal,
		UptimeSeconds:   p.Uptime,
		TemperatureC:    temperatureOf(p),
		// SSHHandshakeMs fica nulo: o agente faz push por HTTP, não abre
		// sessão SSH. Antes o campo ia a zero e o gráfico da estação era uma
		// reta no chão, indistinguível de uma medição de verdade.
		Timestamp: time.Now().UTC(),
	}
	if err := database.DB.Create(&metric).Error; err != nil {
		log.Printf("[Ingest] erro ao inserir métrica de %s: %v", p.Hostname, err)
		writeError(w, http.StatusInternalServerError, "metric insert failed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// siteOfAgent resolve a unidade que o agente declarou. Nulo quando ele não
// informou, ou informou código que não existe no painel — cadastrar a unidade
// sozinho deixaria um token vazado poluir o cadastro com filiais inventadas.
func siteOfAgent(p ingestPayload) *uint {
	code := strings.ToLower(strings.TrimSpace(p.SiteCode))
	if code == "" {
		return nil
	}

	var site database.Site
	if err := database.DB.Where("code = ?", code).First(&site).Error; err != nil {
		log.Printf("[Ingest] unidade %q informada por %s não existe", code, p.Hostname)
		return nil
	}
	return &site.ID
}

// findOrCreateAgentServer localiza o host pela chave (unidade, hostname).
//
// O hostname sozinho não identifica máquina: dois DESKTOP-01 em filiais
// diferentes são dois equipamentos, e antes viravam um registro só — com as
// métricas das duas máquinas serradas na mesma série temporal, cada push
// sobrescrevendo o IP do outro.
//
// A chave definitiva seria um identificador gerado e persistido no próprio
// agente, porque hostname muda e identificador não. Isso exige mudar o agente e
// o coletor; até lá, (unidade, hostname) já separa o que estava junto.
func findOrCreateAgentServer(hostname, machineID, hostIP string, siteID *uint) (database.Server, error) {
	var server database.Server

	// O identificador de máquina vence o hostname quando existe: hostname muda
	// quando alguém renomeia o equipamento, e casar por ele parte o histórico da
	// mesma máquina em duas séries no dia da renomeação.
	if machineID != "" {
		q := database.DB.Where("machine_id = ?", machineID)
		if siteID != nil {
			q = q.Where("site_id = ?", *siteID)
		} else {
			q = q.Where("site_id IS NULL")
		}
		err := q.First(&server).Error
		if err == nil {
			return server, nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return server, err
		}
	}

	q := database.DB.Where("name = ?", hostname)
	if siteID != nil {
		q = q.Where("site_id = ?", *siteID)
	} else {
		q = q.Where("site_id IS NULL")
	}

	err := q.First(&server).Error
	if err == nil {
		return server, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return server, err
	}

	// Adoção do registro que ainda não tem unidade: antes desta mudança o agente
	// que não mandava site_code gravava site_id nulo. Quando ele passa a mandar,
	// abrir linha nova em vez de classificar a antiga partiria o histórico da
	// mesma máquina em duas séries.
	if siteID != nil {
		err = database.DB.Where("name = ? AND site_id IS NULL", hostname).First(&server).Error
		if err == nil {
			return server, nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return server, err
		}
	}

	server = database.Server{
		Name: hostname, HostIP: hostIP, Kind: "agent", SiteID: siteID, MachineID: machineID,
	}
	return server, database.DB.Create(&server).Error
}

// hostFacts monta o que muda pouco e vale sobrescrever a cada push: IP atual,
// sistema, usuário logado e versão do agente.
//
// Campo vazio é ignorado — o agente pode não conseguir ler um sensor ou a
// sessão ativa, e isso não pode apagar o que já se sabia.
func hostFacts(p ingestPayload, hostIP string, siteID *uint) map[string]any {
	facts := map[string]any{"host_ip": hostIP, "kind": "agent"}

	// Intervalo só é gravado quando o agente informa: agente antigo manda 0 e
	// sobrescrever com zero apagaria o valor que uma versão nova já tinha
	// registrado, devolvendo o host à janela fixa.
	if p.ReportIntervalSec > 0 {
		facts["report_interval_sec"] = p.ReportIntervalSec
	}

	for column, value := range map[string]string{
		"os":            p.OS,
		"platform":      p.Platform,
		"arch":          p.Arch,
		"agent_version": p.AgentVersion,
		"last_user":     p.LoggedUser,
	} {
		if strings.TrimSpace(value) != "" {
			facts[column] = value
		}
	}

	// Resolvida antes do upsert, porque agora faz parte da chave do host. Segue
	// sendo gravada aqui para classificar o registro adotado que ainda estava
	// sem unidade.
	if siteID != nil {
		facts["site_id"] = *siteID
	}
	return facts
}

package ssh

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm/clause"

	"github.com/jvS0uzx/dock_keeper/internal/config"
	"github.com/jvS0uzx/dock_keeper/internal/database"
	"github.com/jvS0uzx/dock_keeper/scripts"
)

const (
	intervaloPadraoDaSonda = 15 * time.Minute
	intervaloDoPortao      = 30 * time.Second
)

type NginxUpstreamPayload struct {
	Bloco    string   `json:"bloco"`
	Destinos []string `json:"destinos"`
}

type NginxProbePayload struct {
	Instalado    bool                   `json:"instalado"`
	Ativo        bool                   `json:"ativo"`
	ConfigLida   bool                   `json:"config_lida"`
	ConfigMotivo string                 `json:"config_motivo"`
	LogLegivel   bool                   `json:"log_legivel"`
	LogMotivo    string                 `json:"log_motivo"`
	LogCaminho   string                 `json:"log_caminho"`
	Usuario      string                 `json:"usuario"`
	Upstreams    []NginxUpstreamPayload `json:"upstreams"`
}

func intervaloDaSonda() time.Duration {
	return config.Duracao("NGINX_PROBE_INTERVAL", intervaloPadraoDaSonda)
}

func usuarioDaSonda(p NginxProbePayload) string {
	if u := strings.TrimSpace(p.Usuario); u != "" {
		return u
	}
	return "desconhecido"
}

func motivoDoLog(p NginxProbePayload) string {
	if p.LogLegivel {
		return ""
	}
	caminho := strings.TrimSpace(p.LogCaminho)
	if caminho == "" {
		caminho = NginxLogPath()
	}
	if strings.Contains(p.LogMotivo, "nao encontrado") {
		return fmt.Sprintf("Nginx ativo, log de acesso não encontrado em %s", caminho)
	}
	return fmt.Sprintf("Nginx ativo, log sem permissão de leitura para o usuário %s: %s",
		usuarioDaSonda(p), caminho)
}

func motivoDaConfig(p NginxProbePayload) string {
	detalhe := strings.TrimSpace(p.ConfigMotivo)
	if detalhe == "" {
		detalhe = "nginx -T não devolveu a configuração"
	}
	return fmt.Sprintf("Nginx ativo, configuração não pôde ser lida pelo usuário %s: %s. "+
		"Libere sudo para nginx -T ou suba o painel com SSH_USE_SUDO=true",
		usuarioDaSonda(p), detalhe)
}

func juntarMotivos(partes ...string) string {
	vivos := make([]string, 0, len(partes))
	for _, p := range partes {
		if p != "" {
			vivos = append(vivos, p)
		}
	}
	return strings.Join(vivos, "; ")
}

func classificarNginx(p NginxProbePayload) (string, string) {
	switch {
	case !p.Instalado:
		return database.NginxAusente, ""
	case !p.Ativo:
		return database.NginxInativo, ""
	case !p.ConfigLida:
		return database.NginxDesconhecido, juntarMotivos(motivoDaConfig(p), motivoDoLog(p))
	case len(p.Upstreams) == 0:
		return database.NginxSemUpstream, ""
	default:
		return database.NginxCandidato, motivoDoLog(p)
	}
}

var (
	portaoMu    sync.RWMutex
	portaoNginx = map[string]bool{}
)

func registrarColeta(serverID string, liberada bool) {
	portaoMu.Lock()
	defer portaoMu.Unlock()
	portaoNginx[serverID] = liberada
}

func esquecerColeta(serverID string) {
	portaoMu.Lock()
	defer portaoMu.Unlock()
	delete(portaoNginx, serverID)
}

func coletaDeNginxLiberada(t Target) bool {
	if t.CollectNginx {
		return true
	}
	portaoMu.RLock()
	defer portaoMu.RUnlock()
	return portaoNginx[t.ID]
}

func gravarUpstreams(serverID string, upstreams []NginxUpstreamPayload, agora time.Time) error {
	linhas := make([]database.NginxUpstream, 0, len(upstreams))
	for _, up := range upstreams {
		bloco := strings.TrimSpace(up.Bloco)
		if bloco == "" {
			continue
		}
		for _, destino := range up.Destinos {
			destino = strings.TrimSpace(destino)
			if destino == "" {
				continue
			}
			linhas = append(linhas, database.NginxUpstream{
				ServerID: serverID, Bloco: bloco, Destino: destino, ObservadoEm: agora,
			})
		}
	}

	if len(linhas) > 0 {
		err := database.DB.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "server_id"}, {Name: "bloco"}, {Name: "destino"}},
			DoUpdates: clause.AssignmentColumns([]string{"observado_em", "updated_at"}),
		}).Create(&linhas).Error
		if err != nil {
			return err
		}
	}

	return database.DB.Where("server_id = ? AND observado_em < ?", serverID, agora).
		Delete(&database.NginxUpstream{}).Error
}

func gravarSondaNginx(serverID string, p NginxProbePayload, agora time.Time) error {
	if database.DB == nil {
		return errors.New("banco indisponível para gravar a sonda do nginx")
	}

	estado, motivo := classificarNginx(p)
	err := database.DB.Model(&database.Server{}).Where("id = ?", serverID).Updates(map[string]any{
		"nginx_estado":     estado,
		"nginx_motivo":     motivo,
		"nginx_checado_em": agora,
	}).Error
	if err != nil {
		return err
	}

	if !p.ConfigLida {
		return nil
	}
	return gravarUpstreams(serverID, p.Upstreams, agora)
}

func SondarNginx(ctx context.Context, t Target) (NginxProbePayload, error) {
	var vazio NginxProbePayload

	client, session, err := openSession(t)
	if err != nil {
		return vazio, err
	}
	defer client.Close()
	defer session.Close()

	stopOnCancel(ctx, client, session)

	stdout, err := session.StdoutPipe()
	if err != nil {
		return vazio, err
	}
	if err := runScript(session, t, scripts.ProbeNginx); err != nil {
		return vazio, err
	}

	payload, lido := vazio, false
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		linha := bytes.TrimSpace(scanner.Bytes())
		if len(linha) == 0 || linha[0] != '{' {
			continue
		}
		if err := json.Unmarshal(linha, &payload); err == nil {
			lido = true
		}
	}
	if err := scanner.Err(); err != nil {
		return vazio, err
	}
	if err := session.Wait(); err != nil && !lido {
		return vazio, err
	}
	if !lido {
		return vazio, errors.New("sonda do nginx não devolveu JSON")
	}
	return payload, nil
}

func executarSonda(ctx context.Context, t Target) {
	payload, err := SondarNginx(ctx, t)
	if err != nil {
		if ctx.Err() == nil {
			log.Printf("[Nginx] sonda de %s falhou: %v", t.Host, err)
		}
		return
	}

	registrarColeta(t.ID, payload.Ativo && payload.LogLegivel && len(payload.Upstreams) > 0)

	agora := time.Now().UTC()
	if err := gravarSondaNginx(t.ID, payload, agora); err != nil {
		log.Printf("[Nginx] erro ao gravar a sonda de %s: %v", t.Host, err)
		return
	}

	estado, motivo := classificarNginx(payload)
	if motivo != "" {
		log.Printf("[Nginx] %s classificado como %s: %s", t.Host, estado, motivo)
		return
	}
	log.Printf("[Nginx] %s classificado como %s (%d upstreams)", t.Host, estado, len(payload.Upstreams))
}

func SondarNginxPeriodicamente(ctx context.Context, t Target) {
	for {
		executarSonda(ctx, t)

		select {
		case <-ctx.Done():
			esquecerColeta(t.ID)
			return
		case <-time.After(intervaloDaSonda()):
		}
	}
}

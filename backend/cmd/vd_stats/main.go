package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joaov/vd_stats/internal/alert"
	"github.com/joaov/vd_stats/internal/api"
	"github.com/joaov/vd_stats/internal/auth"
	"github.com/joaov/vd_stats/internal/database"
	"github.com/joaov/vd_stats/internal/discovery"
	"github.com/joaov/vd_stats/internal/logstore"
	"github.com/joaov/vd_stats/internal/network"
	"github.com/joaov/vd_stats/internal/rules"
	"github.com/joaov/vd_stats/internal/ssh"
	"github.com/joho/godotenv"
)

const (
	defaultAPIAddr = ":8080"

	retentionSweep = time.Hour
	trendInterval  = 15 * time.Minute

	sslInterval   = 30 * time.Minute
	rulesInterval = 30 * time.Second
)

// apiAddr permite subir uma segunda instância sem conflito de porta
// (API_ADDR=":8090"), útil para testar um build ao lado do que já roda.
func apiAddr() string {
	if addr := os.Getenv("API_ADDR"); addr != "" {
		return addr
	}
	return defaultAPIAddr
}

func main() {
	log.Println("Iniciando motor DockKeeper...")

	_ = godotenv.Load("../.env", ".env")

	cfg, err := api.LoadConfig(apiAddr())
	if err != nil {
		log.Fatalf("Configuração inválida: %v", err)
	}

	// Host key não verificado entrega a sessão root a quem estiver no caminho.
	// A recusa vem aqui, no boot, e não no primeiro coletor a discar.
	if err := ssh.ValidateHostKeyPolicy(); err != nil {
		log.Fatalf("Configuração SSH inválida: %v", err)
	}

	// Valida as credenciais do Telegram antes de qualquer alerta poder disparar.
	alert.Init()

	// Lê as faixas do inventário de rede; sem DISCOVERY_CIDRS fica desligado.
	discovery.Configure()

	if err := database.Connect(); err != nil {
		log.Fatalf("Falha crítica ao conectar no banco: %v", err)
	}

	// Cria o primeiro administrador se a instalação ainda não tiver usuário.
	auth.Bootstrap()

	// Agrega o histórico bruto em médias horárias antes de ele ser podado.
	// Devolve o sinal que libera a poda: as duas rotinas sobem juntas, e podar
	// antes do primeiro rollup perderia o bruto de um período nunca agregado.
	trendsReady := database.StartTrendWorker(trendInterval)

	// Poda métricas e logs antigos; sem isso as tabelas crescem sem limite.
	// Prazos lidos do ambiente: são o primeiro parâmetro que um adotante muda,
	// e recompilar para trocar sete dias por trinta não é configuração.
	metricRetention := database.RetentionDays("METRIC_RETENTION_DAYS", database.DefaultMetricRetentionDays)
	database.StartRetentionWorker(metricRetention, retentionSweep, trendsReady)
	logRetention := database.RetentionDays("LOG_RETENTION_DAYS", database.DefaultLogRetentionDays)
	logstore.StartRetention(logRetention, retentionSweep)

	// Revalida os certificados periodicamente e alerta os que estão vencendo.
	network.StartSSLWorker(sslInterval)

	// Motor de regras de alerta sobre métricas de host.
	rules.StartEngine(rulesInterval)

	startCollectors(cfg.SSHKeyPath)

	// Ctrl+C / SIGTERM do systemd derrubam a API drenando as conexões e
	// encerram os streams SSH em vez de matar o processo no meio da gravação.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Inventário da rede local: varredura periódica das faixas configuradas.
	discovery.Default.Start(ctx)

	if err := api.StartServer(ctx, cfg); err != nil {
		log.Printf("API encerrou com erro: %v", err)
	}
	ssh.Manager.StopAll()
	log.Println("DockKeeper encerrado.")
}

// startCollectors abre um stream SSH por servidor cadastrado. Hosts do tipo
// "agent" reportam por push e não aceitam SSH — tentar conectar neles só
// geraria alerta de host inalcançável em loop.
func startCollectors(sshKeyPath string) {
	var servers []database.Server
	if err := database.DB.Find(&servers).Error; err != nil {
		log.Printf("Erro ao carregar servidores: %v", err)
		return
	}

	started := 0
	for _, s := range servers {
		if s.Kind == "agent" {
			continue
		}
		ssh.Manager.Start(ssh.Target{
			ID:           s.ID,
			Name:         s.Name,
			Host:         s.HostIP,
			User:         s.User,
			Port:         s.Port,
			KeyPath:      sshKeyPath,
			CollectNginx: s.CollectNginx,
		})
		started++
	}
	log.Printf("Carregados %d servidores (%d por SSH, %d por agente push).", len(servers), started, len(servers)-started)
}

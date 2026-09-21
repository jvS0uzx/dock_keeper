package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/jvS0uzx/dock_keeper/internal/alert"
	"github.com/jvS0uzx/dock_keeper/internal/api"
	"github.com/jvS0uzx/dock_keeper/internal/auth"
	"github.com/jvS0uzx/dock_keeper/internal/database"
	"github.com/jvS0uzx/dock_keeper/internal/discovery"
	"github.com/jvS0uzx/dock_keeper/internal/logstore"
	"github.com/jvS0uzx/dock_keeper/internal/malha"
	"github.com/jvS0uzx/dock_keeper/internal/network"
	"github.com/jvS0uzx/dock_keeper/internal/observabilidade"
	"github.com/jvS0uzx/dock_keeper/internal/rules"
	"github.com/jvS0uzx/dock_keeper/internal/ssh"
)

const (
	defaultAPIAddr = ":8080"

	retentionSweep = time.Hour
	trendInterval  = 15 * time.Minute

	sslInterval   = 30 * time.Minute
	rulesInterval = 30 * time.Second
)

func apiAddr() string {
	if addr := os.Getenv("API_ADDR"); addr != "" {
		return addr
	}
	return defaultAPIAddr
}

func main() {
	observabilidade.Configurar()
	log.Println("Iniciando motor DockKeeper...")

	for _, arquivo := range []string{"../.env", ".env"} {
		if err := godotenv.Load(arquivo); err != nil && !errors.Is(err, os.ErrNotExist) {
			log.Printf("Arquivo %s ignorado, não foi possível ler: %v", arquivo, err)
		}
	}

	cfg, err := api.LoadConfig(apiAddr())
	if err != nil {
		log.Fatalf("Configuração inválida: %v", err)
	}

	if err := ssh.ValidateHostKeyPolicy(); err != nil {
		log.Fatalf("Configuração SSH inválida: %v", err)
	}

	alert.Init()

	discovery.Configure()

	if err := database.Connect(); err != nil {
		log.Fatalf("Falha crítica ao conectar no banco: %v", err)
	}

	auth.Configure()
	auth.Bootstrap()

	trendsReady := database.StartTrendWorker(trendInterval)

	metricRetention := database.RetentionDays("METRIC_RETENTION_DAYS", database.DefaultMetricRetentionDays)
	database.StartRetentionWorker(metricRetention, retentionSweep, trendsReady)
	logRetention := database.RetentionDays("LOG_RETENTION_DAYS", database.DefaultLogRetentionDays)
	logstore.StartRetention(logRetention, retentionSweep)

	api.AvisarJanelaDaMalha()

	network.StartSSLWorker(sslInterval)

	rules.StartEngine(rulesInterval)
	rules.StartAbsenceWatch(rulesInterval)

	startCollectors(cfg.SSHKeyPath)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	alert.StartHealthWatch(ctx)
	alert.StartDispatcher(ctx)
	malha.StartWorker(ctx, 0)
	discovery.Default.Start(ctx)

	if err := api.StartServer(ctx, cfg); err != nil {
		log.Printf("API encerrou com erro: %v", err)
	}
	ssh.Manager.StopAll()
	logstore.Flush()
	log.Println("DockKeeper encerrado.")
}

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
			SiteID:       s.SiteID,
			CollectNginx: s.CollectNginx,
		})
		started++
	}
	log.Printf("Carregados %d servidores (%d por SSH, %d por agente push).", len(servers), started, len(servers)-started)
}

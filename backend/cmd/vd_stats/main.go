package main

import (
	"log"
	"os"
	"time"

	"github.com/joaov/vd_stats/internal/api"
	"github.com/joaov/vd_stats/internal/database"
	"github.com/joaov/vd_stats/internal/network"
	"github.com/joaov/vd_stats/internal/ssh"
	"github.com/joho/godotenv"
)

func main() {
	log.Println("Iniciando motor DockKeeper...")

	_ = godotenv.Load("../.env", ".env")

	err := database.Connect()
	if err != nil {
		log.Fatalf("Falha crítica ao conectar no banco: %v", err)
	}

	// Poda métricas > 7 dias a cada 1h. Sem isso as tabelas metric_* crescem sem limite.
	database.StartRetentionWorker(7*24*time.Hour, time.Hour)

	// Revalida os certificados a cada 30min e alerta os que estão vencendo.
	// O check imediato (ao adicionar) e o recheck manual dão o feedback ao vivo.
	network.StartSSLWorker(30 * time.Minute)

	go api.StartServer(":8080")

	sshKey := os.Getenv("SSH_KEY_PATH")

	// Iniciar Streams SSH de todos os servidores cadastrados no Banco
	var servers []database.Server
	database.DB.Find(&servers)

	log.Printf("Carregados %d servidores do banco de dados.", len(servers))
	for _, s := range servers {
		ssh.Manager.Start(s.ID, s.Name, s.HostIP, s.User, sshKey)
	}

	select {}
}

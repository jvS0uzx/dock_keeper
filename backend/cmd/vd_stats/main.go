package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/joho/godotenv"
	"github.com/joaov/vd_stats/internal/api"
	"github.com/joaov/vd_stats/internal/database"
	"github.com/joaov/vd_stats/internal/ssh"
)

func main() {
	log.Println("Iniciando motor DockKeeper...")

	_ = godotenv.Load("../.env", ".env")

	err := database.Connect()
	if err != nil {
		log.Fatalf("Falha crítica ao conectar no banco: %v", err)
	}

	go api.StartServer(":8080")

	sshKey := os.Getenv("SSH_KEY_PATH")

	// Iniciar Streams SSH de todos os servidores cadastrados no Banco
	var servers []database.Server
	database.DB.Find(&servers)

	log.Printf("Carregados %d servidores do banco de dados.", len(servers))
	for _, s := range servers {
		if s.Name == "Load Balancer" {
			go func(id, host, user, key string) {
				ctx := context.Background()
				for {
					err := ssh.StartNginxStream(ctx, id, host, user, key)
					if err != nil {
						log.Printf("Erro no NGINX Stream %s: %v. Tentando reconectar...", host, err)
						time.Sleep(5 * time.Second)
					}
				}
			}(s.ID, s.HostIP, s.User, sshKey)
		} else {
			ssh.Manager.Start(s.ID, s.HostIP, s.User, sshKey)
		}
	}

	select {}
}

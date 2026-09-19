package auth

import (
	"log"
	"os"
	"strings"

	"github.com/jvS0uzx/dock_keeper/internal/database"
)

func Bootstrap() {
	var count int64
	if err := database.DB.Model(&database.User{}).Count(&count).Error; err != nil {
		log.Printf("[Auth] erro ao verificar usuários: %v", err)
		return
	}
	if count > 0 {
		return
	}

	username := strings.ToLower(strings.TrimSpace(os.Getenv("ADMIN_USER")))
	password := os.Getenv("ADMIN_PASSWORD")
	if username == "" || password == "" {
		log.Println("[Auth] nenhum usuário cadastrado. Defina ADMIN_USER e ADMIN_PASSWORD no .env para criar o primeiro administrador")
		return
	}

	hash, err := HashPassword(password)
	if err != nil {
		log.Printf("[Auth] ADMIN_PASSWORD recusada: %v", err)
		return
	}

	user := database.User{Username: username, PasswordHash: hash, Role: RoleAdmin, Active: true}
	if err := database.DB.Create(&user).Error; err != nil {
		log.Printf("[Auth] erro ao criar o administrador inicial: %v", err)
		return
	}
	log.Printf("[Auth] administrador inicial %q criado. Remova ADMIN_PASSWORD do .env depois do primeiro acesso", username)
}

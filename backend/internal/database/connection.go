package database

import (
	"fmt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"log"
	"os"
)

var DB *gorm.DB

func Connect() error {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Println("DATABASE_URL is not set")
	}

	var err error
	DB, err = gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return err
	}

	err = DB.AutoMigrate(&Server{}, &Container{}, &MetricServer{}, &MetricContainer{}, &MetricLoadBalancer{}, &Domain{}, &AlertRule{}, &LogEntry{})
	if err != nil {
		return fmt.Errorf("erro ao migrar as tabelas: %w", err)
	}

	log.Println("[RealTime] Schemas do Banco de Dados criados/atualizados com sucesso!")

	return nil
}

package database

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/config"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

const (
	defaultStatementTimeout = 15 * time.Second

	defaultMaxOpenConns    = 20
	defaultMaxIdleConns    = 5
	defaultConnMaxLifetime = 30 * time.Minute
)

func From(ctx context.Context) *gorm.DB {
	if DB == nil {
		return nil
	}
	return DB.WithContext(ctx)
}

func statementTimeout() time.Duration {
	return EnvDuration("DB_STATEMENT_TIMEOUT", defaultStatementTimeout)
}

func comStatementTimeout(dsn string) string {
	limite := statementTimeout()
	if limite <= 0 || strings.Contains(dsn, "statement_timeout") {
		return dsn
	}
	ms := strconv.FormatInt(limite.Milliseconds(), 10)

	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		separador := "?"
		if strings.Contains(dsn, "?") {
			separador = "&"
		}
		return dsn + separador + "options=" + url.QueryEscape("-c statement_timeout="+ms)
	}
	if strings.TrimSpace(dsn) == "" {
		return dsn
	}
	return dsn + " statement_timeout=" + ms
}

func Connect() error {
	dsn := comStatementTimeout(os.Getenv("DATABASE_URL"))
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

	if err := configurePool(DB); err != nil {
		return err
	}

	return prepararEsquema(DB)
}

func prepararEsquema(db *gorm.DB) error {
	if autoMigrateLigado() {
		log.Println("[Banco] DB_AUTOMIGRATE foi removido: o AutoMigrate criava banco sem chaves estrangeiras, CHECKs e índices; o esquema sobe pelas migrações versionadas")
	}
	return Migrate(db)
}

func autoMigrateLigado() bool {
	raw := strings.TrimSpace(os.Getenv("DB_AUTOMIGRATE"))
	if raw == "" {
		return false
	}
	ligado, _ := strconv.ParseBool(raw)
	return ligado
}

func configurePool(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("erro ao obter o pool de conexões: %w", err)
	}

	maxOpen := config.Inteiro("DB_MAX_OPEN_CONNS", defaultMaxOpenConns)
	maxIdle := config.Inteiro("DB_MAX_IDLE_CONNS", defaultMaxIdleConns)

	if maxIdle > maxOpen {
		log.Printf("[Banco] DB_MAX_IDLE_CONNS (%d) é maior que DB_MAX_OPEN_CONNS (%d); usando %d",
			maxIdle, maxOpen, maxOpen)
		maxIdle = maxOpen
	}

	lifetime := config.Duracao("DB_CONN_MAX_LIFETIME", defaultConnMaxLifetime)

	sqlDB.SetMaxOpenConns(maxOpen)
	sqlDB.SetMaxIdleConns(maxIdle)
	sqlDB.SetConnMaxLifetime(lifetime)

	log.Printf("[Banco] pool: até %d conexões abertas, %d ociosas, vida útil de %s",
		maxOpen, maxIdle, lifetime)
	return nil
}

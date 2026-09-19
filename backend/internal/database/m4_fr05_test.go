package database

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestDSNGanhaStatementTimeout(t *testing.T) {
	t.Setenv("DB_STATEMENT_TIMEOUT", "15s")

	url := comStatementTimeout("postgres://postgres:ci@127.0.0.1:5432/dockkeeper?sslmode=disable")
	if !strings.Contains(url, "statement_timeout%3D15000") {
		t.Errorf("DSN de URL não recebeu o limite: %s", url)
	}

	chaveValor := comStatementTimeout("host=localhost user=postgres dbname=dockkeeper")
	if !strings.Contains(chaveValor, "statement_timeout=15000") {
		t.Errorf("DSN chave=valor não recebeu o limite: %s", chaveValor)
	}

	jaTem := comStatementTimeout("host=localhost statement_timeout=9000")
	if strings.Count(jaTem, "statement_timeout") != 1 {
		t.Errorf("limite duplicado num DSN que já trazia o seu: %s", jaTem)
	}
}

func TestConsultaTravadaMorreNoLimiteESoltaOPool(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL não definido; pulando teste de statement_timeout")
	}
	t.Setenv("DB_STATEMENT_TIMEOUT", "1s")

	db, err := gorm.Open(postgres.Open(comStatementTimeout(dsn)), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Skipf("banco indisponível: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})

	inicio := time.Now()
	err = db.Exec("SELECT pg_sleep(5)").Error
	gasto := time.Since(inicio)

	if err == nil {
		t.Fatal("a consulta travada terminou sem erro: o limite não está valendo")
	}
	if gasto > 3*time.Second {
		t.Errorf("a consulta segurou a conexão por %s, esperado morrer em cerca de 1 s", gasto)
	}
	if !strings.Contains(strings.ToLower(err.Error()), "statement timeout") {
		t.Errorf("erro = %v, esperado falar em statement timeout", err)
	}

	var um int
	if err := db.Raw("SELECT 1").Scan(&um).Error; err != nil {
		t.Errorf("a conexão não voltou a servir depois do limite: %v", err)
	}
}

func TestFromRespeitaOCancelamentoDoPedido(t *testing.T) {
	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("DATABASE_URL não definido; pulando teste de contexto")
	}
	if DB == nil {
		if err := Connect(); err != nil {
			t.Skipf("banco indisponível: %v", err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var um int
	if err := From(ctx).Raw("SELECT 1").Scan(&um).Error; err == nil {
		t.Error("consulta com contexto cancelado passou: o pedido do cliente não está cancelando a consulta")
	}
}

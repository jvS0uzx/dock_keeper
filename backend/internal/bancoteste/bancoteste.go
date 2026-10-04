package bancoteste

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const VariavelExige = "TEST_EXIGE_BANCO"

func Exigido() bool {
	valor := strings.TrimSpace(os.Getenv(VariavelExige))
	return valor != "" && valor != "0"
}

func Pular(t testing.TB, formato string, args ...any) {
	t.Helper()
	if Exigido() {
		t.Fatalf(formato+" ("+VariavelExige+" exige o banco)", args...)
		return
	}
	t.Skipf(formato, args...)
}

func Main(m *testing.M) int {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return m.Run()
	}

	nome := fmt.Sprintf("dk_teste_%d_%d", os.Getpid(), time.Now().UnixNano())
	admin, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Discard})
	if err == nil {
		err = admin.Exec("CREATE DATABASE " + nome).Error
	}
	if err != nil {
		if Exigido() {
			fmt.Fprintf(os.Stderr, "criar o banco descartável dos testes: %v (%s exige o banco)\n", err, VariavelExige)
			return 1
		}
		return m.Run()
	}
	defer func() {
		admin.Exec("DROP DATABASE IF EXISTS " + nome + " WITH (FORCE)")
		if sqlDB, err := admin.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}()

	_ = os.Setenv("DATABASE_URL", TrocarBanco(dsn, nome))
	defer func() { _ = os.Setenv("DATABASE_URL", dsn) }()
	return m.Run()
}

func TrocarBanco(dsn, nome string) string {
	if strings.Contains(dsn, "dbname=") {
		campos := strings.Fields(dsn)
		for i, campo := range campos {
			if strings.HasPrefix(campo, "dbname=") {
				campos[i] = "dbname=" + nome
				return strings.Join(campos, " ")
			}
		}
	}
	if i := strings.LastIndex(dsn, "/"); i >= 0 {
		resto := ""
		if j := strings.Index(dsn[i:], "?"); j >= 0 {
			resto = dsn[i+j:]
		}
		return dsn[:i+1] + nome + resto
	}
	return dsn
}

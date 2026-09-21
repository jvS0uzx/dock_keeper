package database

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var contadorDeBanco atomic.Int32

func bancoVazio(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL não definido; pulando teste de migração")
	}

	nome := fmt.Sprintf("dockkeeper_mig_%d", os.Getpid()+int(contadorDeBanco.Add(1)))
	admin, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Skipf("banco indisponível: %v", err)
	}
	if err := admin.Exec("DROP DATABASE IF EXISTS " + nome).Error; err != nil {
		t.Skipf("sem permissão para criar banco de teste: %v", err)
	}
	if err := admin.Exec("CREATE DATABASE " + nome).Error; err != nil {
		t.Skipf("sem permissão para criar banco de teste: %v", err)
	}
	t.Cleanup(func() {
		admin.Exec("DROP DATABASE IF EXISTS " + nome)
		if sqlDB, err := admin.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})

	novo, err := gorm.Open(postgres.Open(trocarBancoNoDSN(dsn, nome)), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatalf("abrir o banco de teste: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := novo.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return novo
}

func trocarBancoNoDSN(dsn, nome string) string {
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

func TestTrocarBancoNoDSNCobreOsDoisFormatos(t *testing.T) {
	casos := []struct {
		nome     string
		dsn      string
		esperado string
	}{
		{"url", "postgres://u:s@localhost:5433/dockkeeper?sslmode=disable", "postgres://u:s@localhost:5433/alvo?sslmode=disable"},
		{"chave-valor", "host=localhost user=u password=s dbname=dockkeeper port=5433 sslmode=disable", "host=localhost user=u password=s dbname=alvo port=5433 sslmode=disable"},
	}
	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			if got := trocarBancoNoDSN(caso.dsn, "alvo"); got != caso.esperado {
				t.Errorf("trocarBancoNoDSN = %q, esperado %q", got, caso.esperado)
			}
		})
	}
}

func versaoAplicada(t *testing.T, db *gorm.DB) int {
	t.Helper()

	var versao int
	if err := db.Raw("SELECT COALESCE(MAX(versao), 0) FROM schema_migrations").Scan(&versao).Error; err != nil {
		t.Fatalf("ler a versão aplicada: %v", err)
	}
	return versao
}

func TestBancoVazioSobeAteAUltimaVersao(t *testing.T) {
	db := bancoVazio(t)

	if err := Migrate(db); err != nil {
		t.Fatalf("migrar banco vazio: %v", err)
	}

	lista, err := migracoesEmbutidas()
	if err != nil {
		t.Fatalf("ler migrações: %v", err)
	}
	if v := versaoAplicada(t, db); v != lista[len(lista)-1].versao {
		t.Errorf("versão aplicada = %d, esperado %d", v, lista[len(lista)-1].versao)
	}

	var tabelas int64
	db.Raw("SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public'").Scan(&tabelas)
	if tabelas < 24 {
		t.Errorf("%d tabelas criadas, esperado ao menos 24", tabelas)
	}

	var fks int64
	db.Raw("SELECT count(*) FROM information_schema.table_constraints WHERE constraint_type = 'FOREIGN KEY' AND table_schema = 'public'").Scan(&fks)
	if fks < 9 {
		t.Errorf("%d chaves estrangeiras, esperado ao menos 9", fks)
	}

	var checks int64
	db.Raw("SELECT count(*) FROM pg_constraint WHERE contype = 'c' AND conname LIKE 'ck_%'").Scan(&checks)
	if checks < 11 {
		t.Errorf("%d CHECKs de enum, esperado ao menos 11", checks)
	}

	var trigrama int64
	db.Raw("SELECT count(*) FROM pg_indexes WHERE indexname = 'idx_log_entries_line_trgm'").Scan(&trigrama)
	if trigrama != 1 {
		t.Errorf("índice de trigrama em log_entries não foi criado (%d)", trigrama)
	}
}

func TestMigrarDeNovoNaoFazNada(t *testing.T) {
	db := bancoVazio(t)

	if err := Migrate(db); err != nil {
		t.Fatalf("primeira migração: %v", err)
	}
	var antes string
	db.Raw("SELECT max(aplicada_em)::text FROM schema_migrations").Scan(&antes)

	if err := Migrate(db); err != nil {
		t.Fatalf("segunda migração: %v", err)
	}
	var depois string
	db.Raw("SELECT max(aplicada_em)::text FROM schema_migrations").Scan(&depois)

	if antes != depois {
		t.Errorf("a segunda execução reaplicou migração (%s -> %s)", antes, depois)
	}
}

func TestBaselineComHashAntigoConvergeEmVezDeRecusar(t *testing.T) {
	db := bancoVazio(t)

	if err := Migrate(db); err != nil {
		t.Fatalf("migrar: %v", err)
	}
	if err := db.Exec("UPDATE schema_migrations SET hash = 'hash-do-snapshot-antigo' WHERE versao = 1").Error; err != nil {
		t.Fatalf("alterar o hash do baseline: %v", err)
	}

	if err := Migrate(db); err != nil {
		t.Fatalf("o baseline com hash antigo derrubou a subida: %v", err)
	}
	var hash string
	db.Raw("SELECT hash FROM schema_migrations WHERE versao = 1").Scan(&hash)
	if hash == "hash-do-snapshot-antigo" {
		t.Error("o hash do baseline não foi atualizado")
	}
}

func TestMigracaoEditadaDepoisDeAplicadaFalha(t *testing.T) {
	db := bancoVazio(t)

	if err := Migrate(db); err != nil {
		t.Fatalf("migrar: %v", err)
	}
	if err := db.Exec("UPDATE schema_migrations SET hash = 'hash-de-outro-arquivo' WHERE versao = 2").Error; err != nil {
		t.Fatalf("alterar o hash: %v", err)
	}

	err := Migrate(db)
	if err == nil {
		t.Fatal("migração editada depois de aplicada passou sem erro")
	}
	for _, trecho := range []string{"002_integridade_referencial", "mudou depois de aplicada", "crie uma migração nova"} {
		if !strings.Contains(err.Error(), trecho) {
			t.Errorf("mensagem de erro não explica o problema (%q não contém %q)", err.Error(), trecho)
		}
	}
}

func TestDuasInstanciasMigramUmaVezSo(t *testing.T) {
	db := bancoVazio(t)
	outra, err := gorm.Open(postgres.Open(trocarBancoNoDSN(os.Getenv("DATABASE_URL"), bancoDe(t, db))), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatalf("segunda conexão: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := outra.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})

	var wg sync.WaitGroup
	erros := make([]error, 2)
	for i, conexao := range []*gorm.DB{db, outra} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			erros[i] = Migrate(conexao)
		}()
	}
	wg.Wait()

	for i, err := range erros {
		if err != nil {
			t.Fatalf("instância %d falhou: %v", i+1, err)
		}
	}

	var linhas int64
	db.Raw("SELECT count(*) FROM schema_migrations").Scan(&linhas)
	lista, _ := migracoesEmbutidas()
	if linhas != int64(len(lista)) {
		t.Errorf("%d linhas em schema_migrations, esperado %d: alguma migração rodou duas vezes", linhas, len(lista))
	}
}

func bancoDe(t *testing.T, db *gorm.DB) string {
	t.Helper()

	var nome string
	if err := db.Raw("SELECT current_database()").Scan(&nome).Error; err != nil {
		t.Fatalf("ler o nome do banco: %v", err)
	}
	return nome
}

func TestBancoLegadoAdotaBaselineSemRecriar(t *testing.T) {
	db := bancoVazio(t)

	err := db.AutoMigrate(&Server{}, &Container{}, &MetricServer{}, &MetricContainer{}, &MetricLoadBalancer{},
		&Domain{}, &AlertRule{}, &LogEntry{}, &Site{}, &NetworkHost{}, &FloorPlan{}, &FloorPlanPin{},
		&MetricServerTrend{}, &User{}, &UserSiteAccess{}, &AuditLog{}, &EnrollmentToken{}, &DeviceCredential{},
		&UserSession{}, &AlertState{}, &Alert{}, &Dashboard{}, &DashboardPanel{}, &Annotation{})
	if err != nil {
		t.Fatalf("montar o esquema legado: %v", err)
	}

	if err := Migrate(db); err != nil {
		t.Fatalf("migrar banco que já tinha as tabelas: %v", err)
	}

	lista, _ := migracoesEmbutidas()
	if v := versaoAplicada(t, db); v != lista[len(lista)-1].versao {
		t.Errorf("versão aplicada = %d, esperado %d", v, lista[len(lista)-1].versao)
	}
	var fks int64
	db.Raw("SELECT count(*) FROM information_schema.table_constraints WHERE constraint_type = 'FOREIGN KEY' AND table_schema = 'public'").Scan(&fks)
	if fks < 9 {
		t.Errorf("%d chaves estrangeiras no banco legado, esperado ao menos 9", fks)
	}
}

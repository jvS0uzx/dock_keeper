package database

import (
	"slices"
	"testing"

	"gorm.io/gorm"
)

func retratoDoEsquema(t *testing.T, db *gorm.DB) []string {
	t.Helper()

	var linhas []string
	err := db.Raw(`
		SELECT 'coluna ' || table_name || '.' || column_name || ' ' || data_type || ' nulo=' || is_nullable
		  FROM information_schema.columns WHERE table_schema = 'public'
		UNION ALL
		SELECT 'restricao ' || conrelid::regclass::text || ' ' || conname || ' ' || contype::text
		  FROM pg_constraint WHERE connamespace = 'public'::regnamespace
		UNION ALL
		SELECT 'indice ' || tablename || ' ' || indexname FROM pg_indexes WHERE schemaname = 'public'
		UNION ALL
		SELECT 'extensao ' || extname FROM pg_extension
	`).Scan(&linhas).Error
	if err != nil {
		t.Fatalf("ler o esquema: %v", err)
	}
	slices.Sort(linhas)
	return linhas
}

func TestAutoMigrateLigadoNaoCriaBancoMaisFraco(t *testing.T) {
	versionado := bancoVazio(t)
	if err := prepararEsquema(versionado); err != nil {
		t.Fatalf("esquema versionado: %v", err)
	}
	quer := retratoDoEsquema(t, versionado)

	t.Setenv("DB_AUTOMIGRATE", "true")
	outro := bancoVazio(t)
	if err := prepararEsquema(outro); err != nil {
		t.Fatalf("esquema com DB_AUTOMIGRATE=true: %v", err)
	}
	tem := retratoDoEsquema(t, outro)

	faltam := 0
	for _, linha := range quer {
		if !slices.Contains(tem, linha) {
			faltam++
			if faltam <= 5 {
				t.Errorf("com DB_AUTOMIGRATE=true falta: %s", linha)
			}
		}
	}
	sobram := 0
	for _, linha := range tem {
		if !slices.Contains(quer, linha) {
			sobram++
			if sobram <= 5 {
				t.Errorf("com DB_AUTOMIGRATE=true sobra: %s", linha)
			}
		}
	}
	if faltam+sobram > 0 {
		t.Errorf("os dois caminhos divergem: %d objeto(s) faltando e %d sobrando", faltam, sobram)
	}
}

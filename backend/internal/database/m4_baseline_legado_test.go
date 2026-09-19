package database

import (
	"strings"
	"testing"

	"gorm.io/gorm"
)

const esquemaDeAgosto = `
CREATE TABLE sites (
	id bigserial PRIMARY KEY,
	name character varying(255) NOT NULL,
	code character varying(64),
	created_at timestamp with time zone
);
CREATE TABLE users (
	id bigserial PRIMARY KEY,
	username character varying(64) NOT NULL,
	password_hash character varying(255) NOT NULL,
	role character varying(16) NOT NULL,
	active boolean,
	created_at timestamp with time zone
);
CREATE TABLE user_site_accesses (
	id bigserial PRIMARY KEY,
	user_id bigint NOT NULL,
	site_id bigint,
	role character varying(16) NOT NULL
);
CREATE TABLE servers (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	name character varying(255),
	host_ip character varying(45),
	site_id bigint,
	kind character varying(20),
	machine_id character varying(128),
	deleted_at timestamp with time zone
);
CREATE TABLE metric_servers (
	id bigserial PRIMARY KEY,
	server_id uuid NOT NULL,
	cpu_usage_percent double precision,
	"timestamp" timestamp with time zone NOT NULL
);
CREATE TABLE alert_rules (
	id bigserial PRIMARY KEY,
	name character varying(255) NOT NULL,
	target character varying(64) NOT NULL DEFAULT '*',
	metric character varying(32) NOT NULL,
	operator character varying(4) NOT NULL,
	threshold numeric NOT NULL,
	enabled boolean,
	severity character varying(16) NOT NULL DEFAULT 'warning',
	target_site_id bigint
);
CREATE TABLE alert_states (
	key character varying(128) PRIMARY KEY,
	rule_id bigint NOT NULL,
	server_id character varying(64),
	severity character varying(16),
	active boolean
);
CREATE TABLE floor_plans (
	id bigserial PRIMARY KEY,
	site_id bigint,
	name character varying(255)
);
CREATE TABLE floor_plan_pins (
	id bigserial PRIMARY KEY,
	plan_id bigint NOT NULL,
	host_ip character varying(45)
);
CREATE TABLE device_credentials (
	device_id character varying(64) PRIMARY KEY,
	site_id bigint NOT NULL,
	kind character varying(16) NOT NULL
);
CREATE TABLE enrollment_tokens (
	id bigserial PRIMARY KEY,
	site_id bigint NOT NULL,
	kind character varying(16) NOT NULL
);
CREATE TABLE log_entries (
	id bigserial PRIMARY KEY,
	server_id character varying(64),
	line text,
	"timestamp" timestamp with time zone NOT NULL
);
`

func bancoDeAgosto(t *testing.T) *gorm.DB {
	t.Helper()

	db := bancoVazio(t)
	for _, cmd := range strings.Split(esquemaDeAgosto, ";") {
		if strings.TrimSpace(cmd) == "" {
			continue
		}
		if err := db.Exec(cmd).Error; err != nil {
			t.Fatalf("montar o esquema de agosto: %v", err)
		}
	}

	sede := "INSERT INTO sites (name, code, created_at) VALUES ('Matriz', 'matriz', now())"
	if err := db.Exec(sede).Error; err != nil {
		t.Fatalf("semear unidade: %v", err)
	}
	return db
}

func conferirConvergencia(t *testing.T, db *gorm.DB) {
	t.Helper()

	lista, _ := migracoesEmbutidas()
	if v := versaoAplicada(t, db); v != lista[len(lista)-1].versao {
		t.Errorf("versão aplicada = %d, esperado %d", v, lista[len(lista)-1].versao)
	}

	for _, tabela := range []string{"alerts", "dashboards", "dashboard_panels", "annotations"} {
		var existe *string
		db.Raw("SELECT to_regclass('public." + tabela + "')::text").Scan(&existe)
		if existe == nil {
			t.Errorf("a tabela %q não foi criada pela convergência", tabela)
		}
	}

	var fks int64
	db.Raw("SELECT count(*) FROM information_schema.table_constraints WHERE constraint_type = 'FOREIGN KEY' AND table_schema = 'public'").Scan(&fks)
	if fks < 9 {
		t.Errorf("%d chaves estrangeiras depois de convergir, esperado ao menos 9", fks)
	}

	var unidades int64
	db.Raw("SELECT count(*) FROM sites").Scan(&unidades)
	if unidades != 1 {
		t.Errorf("%d unidades depois de migrar, esperado 1: a convergência não pode apagar dado", unidades)
	}

	var colunas int64
	db.Raw(`SELECT count(*) FROM information_schema.columns
	         WHERE table_name = 'metric_servers' AND column_name IN ('rtt_ms','net_rx_bps','net_tx_bps')`).Scan(&colunas)
	if colunas != 3 {
		t.Errorf("%d colunas novas em metric_servers, esperado 3: a convergência precisa completar tabela antiga", colunas)
	}
}

func TestBancoDeAgostoSemControleConverge(t *testing.T) {
	db := bancoDeAgosto(t)

	if err := Migrate(db); err != nil {
		t.Fatalf("migrar banco legado sem schema_migrations: %v", err)
	}
	conferirConvergencia(t, db)
}

func TestBancoJaAdotadoComBaselineIncompletoConverge(t *testing.T) {
	db := bancoDeAgosto(t)

	criar := `CREATE TABLE schema_migrations (
		versao integer PRIMARY KEY,
		nome text NOT NULL,
		hash text NOT NULL,
		aplicada_em timestamp with time zone NOT NULL DEFAULT now())`
	if err := db.Exec(criar).Error; err != nil {
		t.Fatalf("criar schema_migrations: %v", err)
	}
	if err := db.Exec("INSERT INTO schema_migrations (versao, nome, hash) VALUES (1, 'baseline', 'hash-antigo-do-snapshot')").Error; err != nil {
		t.Fatalf("gravar a adoção anterior: %v", err)
	}

	if err := Migrate(db); err != nil {
		t.Fatalf("migrar banco já adotado com baseline incompleto: %v", err)
	}
	conferirConvergencia(t, db)

	var hash string
	db.Raw("SELECT hash FROM schema_migrations WHERE versao = 1").Scan(&hash)
	if hash == "hash-antigo-do-snapshot" {
		t.Error("o hash do baseline não foi atualizado depois de convergir")
	}

	if err := Migrate(db); err != nil {
		t.Fatalf("segunda subida depois de convergir: %v", err)
	}
	conferirConvergencia(t, db)
}

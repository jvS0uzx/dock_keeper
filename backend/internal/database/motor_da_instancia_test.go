package database

import (
	"strings"
	"testing"
	"time"
)

func TestMotorDaInstanciaSobeNaVersao017(t *testing.T) {
	db := bancoComInventario(t)

	if v := versaoAplicada(t, db); v < 17 {
		t.Fatalf("versão aplicada = %d, esperado ao menos 17", v)
	}

	coluna, ok := colunasDe(t, db, "postgres_instancias")["motor"]
	if !ok {
		t.Fatal("postgres_instancias.motor não existe depois da 017")
	}
	if coluna.DataType != "character varying" {
		t.Errorf("motor é %s, esperado character varying", coluna.DataType)
	}
	if coluna.IsNullable != "NO" {
		t.Errorf("motor tem is_nullable = %s, esperado NO", coluna.IsNullable)
	}

	var tamanho int
	err := db.Raw(`
		SELECT character_maximum_length
		  FROM information_schema.columns
		 WHERE table_schema = 'public' AND table_name = 'postgres_instancias' AND column_name = 'motor'`).
		Scan(&tamanho).Error
	if err != nil {
		t.Fatalf("ler o tamanho de motor: %v", err)
	}
	if tamanho != 24 {
		t.Errorf("motor aceita %d caracteres, esperado 24", tamanho)
	}

	var indices int64
	db.Raw("SELECT count(*) FROM pg_indexes WHERE indexname = 'idx_instancia_motor'").Scan(&indices)
	if indices != 1 {
		t.Errorf("idx_instancia_motor não foi criado (%d)", indices)
	}
}

func TestMotorForaDoConjuntoNaoEntra(t *testing.T) {
	db := bancoComInventario(t)
	servidor := servidorDoInventario(t, db, "vps-motor", "203.0.113.247")

	for _, motor := range []string{"mysql", "mariadb", "POSTGRES", ""} {
		t.Run("motor="+motor, func(t *testing.T) {
			err := db.Exec(
				"INSERT INTO postgres_instancias (server_id, porta, motor) VALUES (?, ?, ?)",
				servidor, 6000+len(motor), motor).Error
			if err == nil {
				t.Fatalf("motor = %q entrou", motor)
			}
			if !strings.Contains(err.Error(), "chk_instancia_motor") {
				t.Errorf("a recusa não veio de chk_instancia_motor: %v", err)
			}
		})
	}

	err := db.Exec(
		"INSERT INTO postgres_instancias (server_id, porta, motor) VALUES (?, ?, ?)",
		servidor, 5432, "postgres").Error
	if err != nil {
		t.Errorf("motor do conjunto foi recusado: %v", err)
	}
}

func TestInstanciaGravadaSemMotorNasceComPostgres(t *testing.T) {
	db := bancoComInventario(t)
	servidor := servidorDoInventario(t, db, "vps-motor-padrao", "203.0.113.248")

	inst := PostgresInstancia{
		ServerID:    servidor,
		Porta:       5432,
		Papel:       "primario",
		Estado:      "ativo",
		ObservadoEm: time.Now().UTC(),
	}
	if err := db.Create(&inst).Error; err != nil {
		t.Fatalf("criar instância sem motor: %v", err)
	}

	var lida PostgresInstancia
	if err := db.Where("id = ?", inst.ID).First(&lida).Error; err != nil {
		t.Fatalf("reler a instância: %v", err)
	}
	if lida.Motor != "postgres" {
		t.Errorf("motor = %q, esperado postgres: o struct tem de casar com o DEFAULT do SQL", lida.Motor)
	}
}

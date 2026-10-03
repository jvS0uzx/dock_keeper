package database

import "testing"

func TestInterruptorDaSondaDeBancosSobeNaVersao018(t *testing.T) {
	db := bancoComInventario(t)

	if v := versaoAplicada(t, db); v < 18 {
		t.Fatalf("versão aplicada = %d, esperado ao menos 18", v)
	}

	colunas := colunasDe(t, db, "servers")
	if _, ok := colunas["collect_postgres"]; ok {
		t.Error("servers.collect_postgres continua existindo depois da 018")
	}
	coluna, ok := colunas["collect_bancos"]
	if !ok {
		t.Fatal("servers.collect_bancos não existe depois da 018")
	}
	if coluna.DataType != "boolean" || coluna.IsNullable != "NO" {
		t.Errorf("collect_bancos é %s/%s, esperado boolean/NO", coluna.DataType, coluna.IsNullable)
	}

	var padrao string
	err := db.Raw(`
		SELECT column_default
		  FROM information_schema.columns
		 WHERE table_schema = 'public' AND table_name = 'servers' AND column_name = 'collect_bancos'`).
		Scan(&padrao).Error
	if err != nil {
		t.Fatalf("ler o default de collect_bancos: %v", err)
	}
	if padrao != "true" {
		t.Errorf("default de collect_bancos = %q, esperado true", padrao)
	}
}

func TestServidorNovoNasceComASondaDeBancosLigada(t *testing.T) {
	db := bancoComInventario(t)
	id := servidorDoInventario(t, db, "vps-interruptor", "203.0.113.240")

	var ligada bool
	if err := db.Raw("SELECT collect_bancos FROM servers WHERE id = ?", id).Row().Scan(&ligada); err != nil {
		t.Fatalf("ler collect_bancos: %v", err)
	}
	if !ligada {
		t.Error("servidor novo nasceu com a sonda de bancos desligada, esperado ligada por padrão")
	}

	var lido Server
	if err := db.Where("id = ?", id).First(&lido).Error; err != nil {
		t.Fatalf("reler servidor: %v", err)
	}
	if !lido.CollectBancos {
		t.Error("Server.CollectBancos = false, esperado o struct casando com o DEFAULT do SQL")
	}
}

func TestMigracao018LigaASondaNosServidoresQueJaExistiam(t *testing.T) {
	db := bancoComInventario(t)

	for _, stmt := range []string{
		"ALTER TABLE servers RENAME COLUMN collect_bancos TO collect_postgres",
		"ALTER TABLE servers ALTER COLUMN collect_postgres SET DEFAULT false",
		"DELETE FROM schema_migrations WHERE versao >= 18",
	} {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("recriar o estado da 017 (%s): %v", stmt, err)
		}
	}

	var id string
	err := db.Raw(`INSERT INTO servers (name, host_ip, collect_postgres)
		VALUES ('vps-antiga', '203.0.113.241', false) RETURNING id`).Row().Scan(&id)
	if err != nil {
		t.Fatalf("criar servidor no estado antigo: %v", err)
	}

	if err := Migrate(db); err != nil {
		t.Fatalf("reaplicar a 018: %v", err)
	}

	var ligada bool
	if err := db.Raw("SELECT collect_bancos FROM servers WHERE id = ?", id).Row().Scan(&ligada); err != nil {
		t.Fatalf("ler collect_bancos: %v", err)
	}
	if !ligada {
		t.Error("servidor que existia antes da 018 ficou com a sonda desligada, esperado ligada")
	}
}

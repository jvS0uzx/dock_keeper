package database

import "testing"

func TestAutoMigrateSobeComColunaAntigaDefaultTrue(t *testing.T) {
	setupInventoryDB(t)

	for _, stmt := range []string{
		`ALTER TABLE alert_rules ALTER COLUMN enabled SET DEFAULT true`,
		`ALTER TABLE users ALTER COLUMN active SET DEFAULT true`,
	} {
		if err := DB.Exec(stmt).Error; err != nil {
			t.Fatalf("recriar o estado antigo (%s): %v", stmt, err)
		}
	}

	if err := DB.AutoMigrate(&AlertRule{}, &User{}); err != nil {
		t.Fatalf("AutoMigrate sobre coluna com DEFAULT true: %v", err)
	}

	regra := AlertRule{Name: "r1-migracao", Target: "*", Metric: "cpu", Operator: ">", Threshold: 90, Enabled: false}
	if err := DB.Create(&regra).Error; err != nil {
		t.Fatalf("criar regra: %v", err)
	}
	t.Cleanup(func() { DB.Where("id = ?", regra.ID).Delete(&AlertRule{}) })

	var gravado bool
	if err := DB.Raw("SELECT enabled FROM alert_rules WHERE id = ?", regra.ID).Row().Scan(&gravado); err != nil {
		t.Fatalf("ler regra: %v", err)
	}
	if gravado {
		t.Error("depois do AutoMigrate, regra criada com Enabled=false foi gravada como true")
	}

	usuario := User{Username: "r1-migracao", PasswordHash: "x", Role: "viewer", Active: false}
	if err := DB.Create(&usuario).Error; err != nil {
		t.Fatalf("criar usuário: %v", err)
	}
	t.Cleanup(func() { DB.Unscoped().Where("id = ?", usuario.ID).Delete(&User{}) })

	if err := DB.Raw("SELECT active FROM users WHERE id = ?", usuario.ID).Row().Scan(&gravado); err != nil {
		t.Fatalf("ler usuário: %v", err)
	}
	if gravado {
		t.Error("depois do AutoMigrate, usuário criado com Active=false foi gravado como true")
	}
}

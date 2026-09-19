package api

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/auth"
	"github.com/jvS0uzx/dock_keeper/internal/database"
)

func comTabelaRenomeada(t *testing.T, tabela string, fn func()) {
	t.Helper()

	desviada := tabela + "_m4_fora"
	if err := database.DB.Exec("ALTER TABLE " + tabela + " RENAME TO " + desviada).Error; err != nil {
		t.Fatalf("renomear %s: %v", tabela, err)
	}
	defer func() {
		if err := database.DB.Exec("ALTER TABLE " + desviada + " RENAME TO " + tabela).Error; err != nil {
			t.Fatalf("restaurar %s: %v", tabela, err)
		}
	}()

	fn()
}

func TestCriacaoDeUsuarioEAtomica(t *testing.T) {
	setupAuditAPI(t)

	const nome = "usuario-m4-atomico"
	database.DB.Unscoped().Where("username = ?", nome).Delete(&database.User{})
	t.Cleanup(func() { database.DB.Unscoped().Where("username = ?", nome).Delete(&database.User{}) })

	user := database.User{Username: nome, PasswordHash: "hash-de-teste", Role: auth.RoleViewer, Active: true}
	acessos := []database.UserSiteAccess{{Role: strings.Repeat("papel-comprido", 4)}}

	if err := criarUsuarioComAcessos(&user, acessos); err == nil {
		t.Fatal("gravar um acesso inválido deveria falhar")
	}

	var n int64
	database.DB.Model(&database.User{}).Where("username = ?", nome).Count(&n)
	if n != 0 {
		t.Errorf("o usuário %q ficou no banco sem acessos: a criação não é atômica", nome)
	}
}

func TestRemocaoDeUsuarioEAtomica(t *testing.T) {
	setupAuditAPI(t)

	const nome = "usuario-m4-remocao"
	database.DB.Unscoped().Where("username = ?", nome).Delete(&database.User{})
	t.Cleanup(func() {
		var u database.User
		if database.DB.Where("username = ?", nome).Take(&u).Error == nil {
			database.DB.Where("user_id = ?", u.ID).Delete(&database.UserSiteAccess{})
		}
		database.DB.Unscoped().Where("username = ?", nome).Delete(&database.User{})
	})

	user := database.User{Username: nome, PasswordHash: "hash-de-teste", Role: auth.RoleOperator, Active: true}
	if err := database.DB.Create(&user).Error; err != nil {
		t.Fatalf("criar usuário: %v", err)
	}
	if err := database.DB.Create(&database.UserSiteAccess{UserID: user.ID, Role: auth.RoleOperator}).Error; err != nil {
		t.Fatalf("criar acesso: %v", err)
	}

	comTabelaRenomeada(t, "dashboards", func() {
		if err := removerUsuario(user); err == nil {
			t.Fatal("remover dashboards deveria falhar com a tabela fora do lugar")
		}
	})

	var usuarios, acessos int64
	database.DB.Model(&database.User{}).Where("username = ?", nome).Count(&usuarios)
	database.DB.Model(&database.UserSiteAccess{}).Where("user_id = ?", user.ID).Count(&acessos)
	if usuarios != 1 || acessos != 1 {
		t.Errorf("usuário = %d e acessos = %d depois da falha, esperados 1 e 1: a remoção não é atômica", usuarios, acessos)
	}
}

func TestRemocaoDeRegraEAtomica(t *testing.T) {
	setupAuditAPI(t)

	const nome = "regra-m4-atomica"
	database.DB.Where("name = ?", nome).Delete(&database.AlertRule{})
	regra := database.AlertRule{Name: nome, Target: "*", Metric: "cpu", Operator: ">", Threshold: 90, Enabled: true}
	if err := database.DB.Create(&regra).Error; err != nil {
		t.Fatalf("criar regra: %v", err)
	}
	id := strconv.FormatUint(uint64(regra.ID), 10)
	t.Cleanup(func() {
		database.DB.Where("rule_id = ?", regra.ID).Delete(&database.AlertState{})
		database.DB.Where("id = ?", regra.ID).Delete(&database.AlertRule{})
	})

	estado := database.AlertState{
		Key:           "m4:" + id,
		RuleID:        regra.ID,
		ServerID:      "servidor-m4",
		Severity:      "warning",
		FirstBreachAt: time.Now().UTC(),
		LastBreachAt:  time.Now().UTC(),
	}
	if err := database.DB.Create(&estado).Error; err != nil {
		t.Fatalf("criar estado da regra: %v", err)
	}

	comTabelaRenomeada(t, "alert_states", func() {
		if err := removerRegra(id); err == nil {
			t.Fatal("limpar alert_states deveria falhar com a tabela fora do lugar")
		}
	})

	var regras int64
	database.DB.Model(&database.AlertRule{}).Where("id = ?", regra.ID).Count(&regras)
	if regras != 1 {
		t.Errorf("a regra sumiu mesmo com a limpeza do estado falhando: a remoção não é atômica")
	}
}

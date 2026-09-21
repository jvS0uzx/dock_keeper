package api

import (
	"net/http"
	"testing"

	"github.com/jvS0uzx/dock_keeper/internal/auth"
	"github.com/jvS0uzx/dock_keeper/internal/database"
)

func usuarioM1(t *testing.T, username, papel string, acessos ...database.UserSiteAccess) database.User {
	t.Helper()
	limparUsuario(t, username)

	u := database.User{Username: username, PasswordHash: "sem-login-neste-teste", Role: papel, Active: true}
	if err := database.DB.Create(&u).Error; err != nil {
		t.Fatalf("criar %s: %v", username, err)
	}
	for _, a := range acessos {
		a.UserID = u.ID
		if err := database.DB.Create(&a).Error; err != nil {
			t.Fatalf("conceder acesso a %s: %v", username, err)
		}
	}
	return u
}

func soEstesAtivos(t *testing.T, ids ...uint) {
	t.Helper()

	var desligados []uint
	database.DB.Model(&database.User{}).Where("active = ? AND id NOT IN ?", true, ids).Pluck("id", &desligados)
	if len(desligados) == 0 {
		return
	}
	database.DB.Model(&database.User{}).Where("id IN ?", desligados).Update("active", false)
	t.Cleanup(func() {
		database.DB.Model(&database.User{}).Where("id IN ?", desligados).Update("active", true)
	})
}

func TestUltimoAdminGlobalNaoSaiPorNenhumaDasQuatroPortas(t *testing.T) {
	c := setupC3(t)
	ultimo := usuarioM1(t, "m1-ultimo-global", auth.RoleAdmin)
	filial := usuarioM1(t, "m1-admin-de-filial", auth.RoleAdmin, database.UserSiteAccess{SiteID: &c.siteA, Role: auth.RoleAdmin})
	soEstesAtivos(t, ultimo.ID, filial.ID)

	rota := "/api/users?id=" + uintStr(ultimo.ID)
	for nome, corpo := range map[string]string{
		"rebaixar o papel":         `{"role":"viewer"}`,
		"restringir a uma unidade": `{"accesses":[{"site_id":` + uintStr(c.siteA) + `,"role":"admin"}]}`,
		"desativar":                `{"active":false}`,
	} {
		rec := chamar(t, usersHandler, c.adminGlobal, http.MethodPatch, rota, corpo)
		if rec.Code != http.StatusConflict {
			t.Errorf("%s: status %d, esperado 409; admin só de filial não é outro administrador (%s)", nome, rec.Code, rec.Body.String())
		}
	}
	if rec := chamar(t, usersHandler, c.adminGlobal, http.MethodDelete, rota, ""); rec.Code != http.StatusConflict {
		t.Errorf("apagar: status %d, esperado 409", rec.Code)
	}

	var depois database.User
	database.DB.First(&depois, ultimo.ID)
	var concessoes int64
	database.DB.Model(&database.UserSiteAccess{}).Where("user_id = ?", ultimo.ID).Count(&concessoes)
	if depois.Role != auth.RoleAdmin || !depois.Active || concessoes != 0 {
		t.Errorf("o último admin global ficou role=%s active=%v concessões=%d", depois.Role, depois.Active, concessoes)
	}
}

func TestAdminGlobalPorConcessaoContaComoOutroAdmin(t *testing.T) {
	c := setupC3(t)
	alvo := usuarioM1(t, "m1-alvo", auth.RoleAdmin)
	outro := usuarioM1(t, "m1-global-por-concessao", auth.RoleViewer, database.UserSiteAccess{Role: auth.RoleAdmin})
	soEstesAtivos(t, alvo.ID, outro.ID)

	rec := chamar(t, usersHandler, c.adminGlobal, http.MethodPatch, "/api/users?id="+uintStr(alvo.ID), `{"role":"viewer"}`)
	if rec.Code != http.StatusOK {
		t.Errorf("rebaixar com outro admin global efetivo: status %d, esperado 200 (%s)", rec.Code, rec.Body.String())
	}
}

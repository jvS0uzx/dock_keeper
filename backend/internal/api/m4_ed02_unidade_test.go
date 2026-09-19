package api

import (
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/auth"
	"github.com/jvS0uzx/dock_keeper/internal/database"
)

func unidadeDeTeste(t *testing.T, codigo string) database.Site {
	t.Helper()

	setupAuditAPI(t)
	database.DB.Where("code = ?", codigo).Delete(&database.Site{})

	site := database.Site{Name: codigo, Code: codigo}
	if err := database.DB.Create(&site).Error; err != nil {
		t.Fatalf("criar unidade: %v", err)
	}
	t.Cleanup(func() {
		database.DB.Where("site_id = ?", site.ID).Delete(&database.DeviceCredential{})
		database.DB.Where("site_id = ?", site.ID).Delete(&database.EnrollmentToken{})
		database.DB.Where("site_id = ?", site.ID).Delete(&database.UserSiteAccess{})
		database.DB.Model(&database.Server{}).Where("site_id = ?", site.ID).Update("site_id", nil)
		database.DB.Model(&database.AlertRule{}).Where("target_site_id = ?", site.ID).Update("target_site_id", nil)
		database.DB.Where("code = ?", codigo).Delete(&database.Site{})
	})
	return site
}

func credencialNaUnidade(t *testing.T, siteID uint, revogada bool) database.DeviceCredential {
	t.Helper()

	cred := database.DeviceCredential{
		DeviceID:   "dev-" + strconv.FormatUint(uint64(siteID), 10) + map[bool]string{true: "-rev", false: "-ativa"}[revogada],
		SiteID:     siteID,
		Kind:       kindCollector,
		SecretHash: "hash",
		MachineID:  "maquina",
	}
	if revogada {
		agora := time.Now().UTC()
		cred.RevokedAt = &agora
	}
	if err := database.DB.Create(&cred).Error; err != nil {
		t.Fatalf("criar credencial: %v", err)
	}
	return cred
}

func removerUnidade(t *testing.T, siteID uint) int {
	t.Helper()

	sess := sessaoReal(t, "admin-ed02-"+strconv.FormatUint(uint64(siteID), 10), auth.RoleAdmin)
	rec := pedirComSessao(t, http.MethodDelete, "/api/sites?id="+strconv.FormatUint(uint64(siteID), 10), "", sess)
	return rec.Code
}

func TestUnidadeComColetorAtivoNaoERemovida(t *testing.T) {
	site := unidadeDeTeste(t, "qa-ed02-ativa")
	credencialNaUnidade(t, site.ID, false)

	if código := removerUnidade(t, site.ID); código != http.StatusConflict {
		t.Fatalf("status = %d, esperado 409 — a unidade tem coletor ativo", código)
	}

	var unidades, creds int64
	database.DB.Model(&database.Site{}).Where("id = ?", site.ID).Count(&unidades)
	database.DB.Model(&database.DeviceCredential{}).Where("site_id = ?", site.ID).Count(&creds)
	if unidades != 1 || creds != 1 {
		t.Errorf("unidade = %d e credenciais = %d depois da recusa, esperados 1 e 1", unidades, creds)
	}
}

func TestConviteValidoTambemSeguraARemocao(t *testing.T) {
	site := unidadeDeTeste(t, "qa-ed02-convite")
	convite := database.EnrollmentToken{
		TokenHash: "hash-ed02",
		SiteID:    site.ID,
		Kind:      kindAgent,
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}
	if err := database.DB.Create(&convite).Error; err != nil {
		t.Fatalf("criar convite: %v", err)
	}

	if código := removerUnidade(t, site.ID); código != http.StatusConflict {
		t.Errorf("status = %d, esperado 409 — a unidade tem convite válido", código)
	}
}

func TestUnidadeSemDispositivoSaiSemDeixarOrfao(t *testing.T) {
	site := unidadeDeTeste(t, "qa-ed02-limpa")
	credencialNaUnidade(t, site.ID, true)

	servidor := database.Server{Name: "srv-ed02", HostIP: "203.0.113.90", User: "root", Port: 22, SiteID: &site.ID}
	if err := database.DB.Create(&servidor).Error; err != nil {
		t.Fatalf("criar servidor: %v", err)
	}
	t.Cleanup(func() { database.DB.Unscoped().Where("id = ?", servidor.ID).Delete(&database.Server{}) })

	regra := database.AlertRule{Name: "regra-ed02", Target: "*", Metric: "cpu", Operator: ">", Threshold: 90, TargetSiteID: &site.ID}
	if err := database.DB.Create(&regra).Error; err != nil {
		t.Fatalf("criar regra: %v", err)
	}
	t.Cleanup(func() { database.DB.Where("id = ?", regra.ID).Delete(&database.AlertRule{}) })

	usuario := sessaoReal(t, "operador-ed02", auth.RoleOperator)
	acesso := database.UserSiteAccess{UserID: usuario.UserID, SiteID: &site.ID, Role: auth.RoleOperator}
	if err := database.DB.Create(&acesso).Error; err != nil {
		t.Fatalf("criar acesso: %v", err)
	}

	host := database.NetworkHost{IP: "192.168.90.10", SiteID: &site.ID}
	if err := database.DB.Create(&host).Error; err != nil {
		t.Fatalf("criar host: %v", err)
	}
	t.Cleanup(func() { database.DB.Where("ip = ?", host.IP).Delete(&database.NetworkHost{}) })

	if código := removerUnidade(t, site.ID); código != http.StatusOK {
		t.Fatalf("status = %d, esperado 200", código)
	}

	var unidades, servidoresOrfaos, regrasOrfas, acessosOrfaos, credsOrfas, hostsOrfaos int64
	database.DB.Model(&database.Site{}).Where("id = ?", site.ID).Count(&unidades)
	database.DB.Model(&database.Server{}).Where("site_id = ?", site.ID).Count(&servidoresOrfaos)
	database.DB.Model(&database.AlertRule{}).Where("target_site_id = ?", site.ID).Count(&regrasOrfas)
	database.DB.Model(&database.UserSiteAccess{}).Where("site_id = ?", site.ID).Count(&acessosOrfaos)
	database.DB.Model(&database.DeviceCredential{}).Where("site_id = ?", site.ID).Count(&credsOrfas)
	database.DB.Model(&database.NetworkHost{}).Where("site_id = ?", site.ID).Count(&hostsOrfaos)

	if unidades != 0 {
		t.Errorf("a unidade continuou no banco")
	}
	if servidoresOrfaos+regrasOrfas+acessosOrfaos+credsOrfas+hostsOrfaos != 0 {
		t.Errorf("sobrou referência órfã: servidores=%d regras=%d acessos=%d credenciais=%d hosts=%d",
			servidoresOrfaos, regrasOrfas, acessosOrfaos, credsOrfas, hostsOrfaos)
	}
}

func TestFalhaNoMeioDaRemocaoNaoAplicaNada(t *testing.T) {
	site := unidadeDeTeste(t, "qa-ed02-falha")
	servidor := database.Server{Name: "srv-ed02-falha", HostIP: "203.0.113.91", User: "root", Port: 22, SiteID: &site.ID}
	if err := database.DB.Create(&servidor).Error; err != nil {
		t.Fatalf("criar servidor: %v", err)
	}
	t.Cleanup(func() { database.DB.Unscoped().Where("id = ?", servidor.ID).Delete(&database.Server{}) })

	comTabelaRenomeada(t, "floor_plans", func() {
		if código := removerUnidade(t, site.ID); código != http.StatusInternalServerError {
			t.Fatalf("status = %d, esperado 500 com a tabela fora do lugar", código)
		}
	})

	var unidades int64
	database.DB.Model(&database.Site{}).Where("id = ?", site.ID).Count(&unidades)
	if unidades != 1 {
		t.Errorf("a unidade foi removida mesmo com falha no meio: a remoção não é atômica")
	}

	var vinculado int64
	database.DB.Model(&database.Server{}).Where("id = ? AND site_id = ?", servidor.ID, site.ID).Count(&vinculado)
	if vinculado != 1 {
		t.Errorf("o servidor perdeu a unidade mesmo com a remoção falhando")
	}
}

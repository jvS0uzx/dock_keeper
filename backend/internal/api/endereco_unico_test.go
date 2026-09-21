package api

import (
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/jvS0uzx/dock_keeper/internal/auth"
	"github.com/jvS0uzx/dock_keeper/internal/database"
)

func cadastrar(t *testing.T, sess auth.Session, corpo string) int {
	t.Helper()

	rec := pedirComSessao(t, http.MethodPost, "/api/servers", corpo, sess)
	return rec.Code
}

func limparServidorPorNome(t *testing.T, nome string) {
	t.Helper()

	apagar := func() {
		var s database.Server
		if err := database.DB.Where("name = ?", nome).Take(&s).Error; err == nil {
			database.DB.Where("server_id = ?", s.ID).Delete(&database.ServerAddress{})
			database.DB.Unscoped().Where("id = ?", s.ID).Delete(&database.Server{})
		}
	}
	apagar()
	t.Cleanup(apagar)
}

func TestCadastroComIPDeOutroServidorRecusa(t *testing.T) {
	setupAuditAPI(t)
	sess := sessaoReal(t, "admin-ip-unico", auth.RoleAdmin)
	servidorDeRename(t, "VPS-1", "203.0.113.25", nil)
	limparServidorPorNome(t, "VPS-1-de-novo")

	rec := pedirComSessao(t, http.MethodPost, "/api/servers",
		`{"host_ip":"203.0.113.25","name":"VPS-1-de-novo","user":"root","port":22}`, sess)
	if rec.Code != http.StatusConflict {
		t.Fatalf("mesmo IP com outro nome: status %d, esperado 409 (%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "VPS-1") {
		t.Errorf("a recusa não diz qual servidor já usa o endereço: %s", rec.Body.String())
	}

	var quantos int64
	database.DB.Model(&database.Server{}).Where("host_ip = ?", "203.0.113.25").Count(&quantos)
	if quantos != 1 {
		t.Errorf("o cadastro recusado mexeu no banco: %d servidores com o IP", quantos)
	}
	var original database.Server
	database.DB.Where("host_ip = ?", "203.0.113.25").Take(&original)
	if original.Name != "VPS-1" {
		t.Errorf("o servidor existente foi renomeado para %q pelo cadastro recusado", original.Name)
	}
}

func TestCadastroComIPQueEAliasDeOutroRecusa(t *testing.T) {
	setupAuditAPI(t)
	sess := sessaoReal(t, "admin-ip-alias", auth.RoleAdmin)
	dono := servidorDeRename(t, "VPS-com-overlay", "203.0.113.26", nil)
	database.RegistrarAliases(dono.ID, []string{"100.100.0.11"})
	limparServidorPorNome(t, "VPS-clonada")

	rec := pedirComSessao(t, http.MethodPost, "/api/servers",
		`{"host_ip":"100.100.0.11","name":"VPS-clonada","user":"root","port":22}`, sess)
	if rec.Code != http.StatusConflict {
		t.Fatalf("IP que é alias de outro: status %d, esperado 409 (%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "VPS-com-overlay") {
		t.Errorf("a recusa não cita o dono do alias: %s", rec.Body.String())
	}
}

func TestFaixaPrivadaRepeteEntreUnidades(t *testing.T) {
	setupAuditAPI(t)
	sess := sessaoReal(t, "admin-privado", auth.RoleAdmin)
	matriz := unidadeDeRename(t, "priv-matriz")
	filial := unidadeDeRename(t, "priv-filial")
	servidorDeRename(t, "servidor-matriz", "192.168.0.10", &matriz)
	limparServidorPorNome(t, "servidor-filial")

	corpo := `{"host_ip":"192.168.0.10","name":"servidor-filial","user":"root","port":22,"site_id":` +
		itoa(filial) + `}`
	if code := cadastrar(t, sess, corpo); code != http.StatusCreated {
		t.Fatalf("mesma faixa privada em outra unidade: status %d, esperado 201", code)
	}
}

func TestFaixaPrivadaRepetidaNaMesmaUnidadeRecusa(t *testing.T) {
	setupAuditAPI(t)
	sess := sessaoReal(t, "admin-privado-mesma", auth.RoleAdmin)
	unidade := unidadeDeRename(t, "priv-mesma")
	servidorDeRename(t, "servidor-um", "192.168.5.10", &unidade)
	limparServidorPorNome(t, "servidor-dois")

	corpo := `{"host_ip":"192.168.5.10","name":"servidor-dois","user":"root","port":22,"site_id":` +
		itoa(unidade) + `}`
	if code := cadastrar(t, sess, corpo); code != http.StatusConflict {
		t.Fatalf("mesmo IP privado na mesma unidade: status %d, esperado 409", code)
	}
}

func TestOverlayEUnicaNaFrota(t *testing.T) {
	setupAuditAPI(t)
	sess := sessaoReal(t, "admin-overlay", auth.RoleAdmin)
	matriz := unidadeDeRename(t, "overlay-matriz")
	filial := unidadeDeRename(t, "overlay-filial")
	servidorDeRename(t, "servidor-overlay", "100.100.0.10", &matriz)
	limparServidorPorNome(t, "servidor-overlay-2")

	corpo := `{"host_ip":"100.100.0.10","name":"servidor-overlay-2","user":"root","port":22,"site_id":` +
		itoa(filial) + `}`
	if code := cadastrar(t, sess, corpo); code != http.StatusConflict {
		t.Fatalf("overlay 100.64/10 em outra unidade: status %d, esperado 409", code)
	}
}

func TestPatchComAliasDeOutroServidorRecusa(t *testing.T) {
	setupAuditAPI(t)
	sess := sessaoReal(t, "admin-alias-conflito", auth.RoleAdmin)
	dono := servidorDeRename(t, "dono-do-endereco", "203.0.113.27", nil)
	database.RegistrarAliases(dono.ID, []string{"100.100.0.9"})
	outro := servidorDeRename(t, "quer-o-endereco", "203.0.113.28", nil)

	rec := pedirComSessao(t, http.MethodPatch, "/api/servers?id="+outro.ID,
		`{"aliases":["100.100.0.9"]}`, sess)
	if rec.Code != http.StatusConflict {
		t.Fatalf("alias de outro servidor: status %d, esperado 409 (%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "dono-do-endereco") {
		t.Errorf("a recusa não cita o dono: %s", rec.Body.String())
	}
}

func TestCadastroRecusadoGeraAuditoria(t *testing.T) {
	setupAuditAPI(t)
	sess := sessaoReal(t, "admin-auditoria-ip", auth.RoleAdmin)
	servidorDeRename(t, "servidor-auditado", "203.0.113.29", nil)
	limparServidorPorNome(t, "servidor-recusado")

	cadastrar(t, sess, `{"host_ip":"203.0.113.29","name":"servidor-recusado","user":"root","port":22}`)

	var linha database.AuditLog
	err := database.DB.Where("action = ?", "server.create").Order("id desc").Take(&linha).Error
	if err != nil {
		t.Fatalf("a recusa por endereço duplicado não gerou linha de auditoria: %v", err)
	}
	if linha.Result == "ok" {
		t.Errorf("a linha da recusa ficou com resultado %q", linha.Result)
	}
}

func itoa(v uint) string {
	return strconv.FormatUint(uint64(v), 10)
}

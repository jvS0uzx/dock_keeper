package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/jvS0uzx/dock_keeper/internal/auth"
	"github.com/jvS0uzx/dock_keeper/internal/database"
	"github.com/jvS0uzx/dock_keeper/internal/ssh"
)

func limparServidorCadastrado(t *testing.T, nome string) {
	t.Helper()

	apagar := func() {
		var achados []database.Server
		database.DB.Unscoped().Where("name = ?", nome).Find(&achados)
		for _, s := range achados {
			ssh.Manager.Stop(s.ID)
			database.DB.Where("server_id = ?", s.ID).Delete(&database.ServerAddress{})
			database.DB.Unscoped().Where("id = ?", s.ID).Delete(&database.Server{})
		}
	}
	apagar()
	t.Cleanup(apagar)
}

func servidorPorNome(t *testing.T, nome string) database.Server {
	t.Helper()

	var s database.Server
	if err := database.DB.Where("name = ?", nome).Take(&s).Error; err != nil {
		t.Fatalf("servidor %q não está no banco: %v", nome, err)
	}
	return s
}

func comBancoFechado(t *testing.T, corpo func()) {
	t.Helper()

	morto, err := gorm.Open(postgres.Open(os.Getenv("DATABASE_URL")), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("abrir a conexão que vai ser fechada: %v", err)
	}
	sqlDB, err := morto.DB()
	if err != nil {
		t.Fatalf("pegar o pool: %v", err)
	}
	sqlDB.Close()

	vivo := database.DB
	database.DB = morto
	defer func() { database.DB = vivo }()
	corpo()
}

func TestServidorApagadoLiberaOIPEOsEnderecos(t *testing.T) {
	setupAuditAPI(t)
	sess := sessaoReal(t, "admin-m5-apagado", auth.RoleAdmin)
	antigo := servidorDeRename(t, "m5-vps-apagada", "198.51.100.10", nil)
	database.RegistrarAliases(antigo.ID, []string{"100.100.0.90"})
	database.RegistrarEnderecos(antigo.ID, []string{"10.50.0.1"})
	limparServidorCadastrado(t, "m5-vps-de-volta")
	limparServidorCadastrado(t, "m5-vps-no-alias")

	rec := pedirComSessao(t, http.MethodDelete, "/api/servers?id="+antigo.ID, "", sess)
	if rec.Code != http.StatusOK {
		t.Fatalf("DELETE do servidor: status %d (%s)", rec.Code, rec.Body.String())
	}

	var sobras int64
	database.DB.Model(&database.ServerAddress{}).Where("server_id = ?", antigo.ID).Count(&sobras)
	if sobras != 0 {
		t.Errorf("o servidor apagado deixou %d endereço(s) em server_addresses", sobras)
	}

	rec = pedirComSessao(t, http.MethodPost, "/api/servers",
		`{"host_ip":"198.51.100.10","name":"m5-vps-de-volta","user":"root","port":22}`, sess)
	if rec.Code != http.StatusCreated {
		t.Fatalf("recadastrar o IP de um servidor apagado: status %d, esperado 201 (%s)", rec.Code, rec.Body.String())
	}
	if novo := servidorPorNome(t, "m5-vps-de-volta"); novo.ID == antigo.ID {
		t.Errorf("o recadastro reviveu o registro apagado (%s) em vez de criar um novo", antigo.ID)
	}

	rec = pedirComSessao(t, http.MethodPost, "/api/servers",
		`{"host_ip":"100.100.0.90","name":"m5-vps-no-alias","user":"root","port":22}`, sess)
	if rec.Code != http.StatusCreated {
		t.Fatalf("cadastrar o antigo alias de um servidor apagado: status %d, esperado 201 (%s)", rec.Code, rec.Body.String())
	}
}

func TestIPPrivadoEmOutraUnidadeNaoMexeNoServidorExistente(t *testing.T) {
	setupAuditAPI(t)
	sess := sessaoReal(t, "admin-m5-privado", auth.RoleAdmin)
	matriz := unidadeDeRename(t, "m5-priv-matriz")
	filial := unidadeDeRename(t, "m5-priv-filial")
	original := servidorDeRename(t, "m5-servidor-matriz", "192.168.0.10", &matriz)
	limparServidorCadastrado(t, "m5-servidor-filial")

	corpo := `{"host_ip":"192.168.0.10","name":"m5-servidor-filial","user":"deploy","port":2222,"site_id":` + itoa(filial) + `}`
	if code := cadastrar(t, sess, corpo); code != http.StatusCreated {
		t.Fatalf("mesma faixa privada em outra unidade: status %d, esperado 201", code)
	}

	var comOIP int64
	database.DB.Model(&database.Server{}).Where("host_ip = ?", "192.168.0.10").Count(&comOIP)
	if comOIP != 2 {
		t.Errorf("há %d servidor(es) com 192.168.0.10, esperado 2 (um por unidade)", comOIP)
	}

	var depois database.Server
	if err := database.DB.Where("id = ?", original.ID).Take(&depois).Error; err != nil {
		t.Fatalf("o servidor da matriz sumiu: %v", err)
	}
	if depois.Name != "m5-servidor-matriz" || depois.SiteID == nil || *depois.SiteID != matriz ||
		depois.User != "root" || depois.Port != 22 {
		t.Errorf("o cadastro na filial sobrescreveu o servidor da matriz: nome=%q unidade=%v user=%q porta=%d",
			depois.Name, depois.SiteID, depois.User, depois.Port)
	}
}

func enviarMetricaDeAgente(t *testing.T, corpo string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/api/ingest/metrics", strings.NewReader(corpo))
	req.Header.Set(headerLegacyToken, "token-m5-enderecos")
	rec := httptest.NewRecorder()
	Routes(testConfig()).ServeHTTP(rec, req)
	return rec
}

func TestIngestaoComEnderecoAlheioRecusaEAuditaUmaVez(t *testing.T) {
	setupAuditAPI(t)
	t.Setenv("AGENT_INGEST_TOKEN", "token-m5-enderecos")
	t.Setenv("ALLOW_LEGACY_INGEST_TOKEN", "true")
	zerarLimiteDeIngestao()
	alvo := servidorDeRename(t, "m5-vps-alvo", "198.51.100.20", nil)
	limparServidorCadastrado(t, "m5-estacao-mentirosa")

	corpo := `{"hostname":"m5-estacao-mentirosa","machine_id":"m5-mentirosa","cpu":3.5,` +
		`"addresses":["198.51.100.20","10.60.0.1"]}`
	for i := 0; i < 3; i++ {
		if rec := enviarMetricaDeAgente(t, corpo); rec.Code != http.StatusOK {
			t.Fatalf("envio %d: status %d (%s)", i+1, rec.Code, rec.Body.String())
		}
	}

	estacao := servidorPorNome(t, "m5-estacao-mentirosa")
	t.Cleanup(func() { database.DB.Where("server_id = ?", estacao.ID).Delete(&database.MetricServer{}) })

	porServidor, err := database.EnderecosPorServidor([]string{estacao.ID})
	if err != nil {
		t.Fatalf("ler endereços: %v", err)
	}
	guardados := map[string]bool{}
	for _, a := range porServidor[estacao.ID] {
		guardados[a] = true
	}
	if guardados["198.51.100.20"] {
		t.Errorf("a estação ficou com o host_ip de %s: %v", alvo.Name, porServidor[estacao.ID])
	}
	if !guardados["10.60.0.1"] {
		t.Errorf("o endereço legítimo do envio se perdeu: %v", porServidor[estacao.ID])
	}

	var linhas []database.AuditLog
	database.DB.Where("action = ? AND target_id = ?", "server.address_refused", estacao.ID).Find(&linhas)
	if len(linhas) != 1 {
		t.Fatalf("três envios com o mesmo endereço alheio geraram %d linha(s) de auditoria, esperado 1", len(linhas))
	}
	t.Cleanup(func() {
		database.DB.Where("action = ? AND target_id = ?", "server.address_refused", estacao.ID).Delete(&database.AuditLog{})
	})
	if linhas[0].Result != "denied" {
		t.Errorf("resultado da linha = %q, esperado denied", linhas[0].Result)
	}
	var detalhe map[string]any
	if err := json.Unmarshal([]byte(linhas[0].Detail), &detalhe); err != nil {
		t.Fatalf("detalhe ilegível: %v (%s)", err, linhas[0].Detail)
	}
	if !strings.Contains(linhas[0].Detail, "198.51.100.20") || !strings.Contains(linhas[0].Detail, "m5-vps-alvo") {
		t.Errorf("o detalhe não diz o endereço nem o dono: %s", linhas[0].Detail)
	}
}

func TestIPObservadoRepetidoNaoViraAuditoria(t *testing.T) {
	setupAuditAPI(t)
	t.Setenv("AGENT_INGEST_TOKEN", "token-m5-enderecos")
	t.Setenv("ALLOW_LEGACY_INGEST_TOKEN", "true")
	zerarLimiteDeIngestao()
	limparServidorCadastrado(t, "m5-estacao-nat-um")
	limparServidorCadastrado(t, "m5-estacao-nat-dois")

	for _, nome := range []string{"m5-estacao-nat-um", "m5-estacao-nat-dois"} {
		corpo := `{"hostname":"` + nome + `","machine_id":"` + nome + `","cpu":1}`
		if rec := enviarMetricaDeAgente(t, corpo); rec.Code != http.StatusOK {
			t.Fatalf("envio de %s: status %d (%s)", nome, rec.Code, rec.Body.String())
		}
	}

	segunda := servidorPorNome(t, "m5-estacao-nat-dois")
	primeira := servidorPorNome(t, "m5-estacao-nat-um")
	t.Cleanup(func() {
		database.DB.Where("server_id IN ?", []string{primeira.ID, segunda.ID}).Delete(&database.MetricServer{})
	})

	var linhas int64
	database.DB.Model(&database.AuditLog{}).
		Where("action = ? AND target_id = ?", "server.address_refused", segunda.ID).Count(&linhas)
	if linhas != 0 {
		t.Errorf("duas estações atrás do mesmo NAT geraram %d linha(s) de auditoria; o IP observado não é declaração do dispositivo", linhas)
	}
}

func TestRecusaNaColetaSSHViraAuditoria(t *testing.T) {
	setupAuditAPI(t)
	dona := servidorDeRename(t, "m5-vps-dona-ssh", "198.51.100.30", nil)
	outra := servidorDeRename(t, "m5-vps-comprometida", "198.51.100.31", nil)
	t.Cleanup(func() {
		database.DB.Where("action = ? AND target_id = ?", "server.address_refused", outra.ID).Delete(&database.AuditLog{})
	})

	database.RegistrarEnderecos(outra.ID, []string{dona.HostIP, outra.HostIP})

	var linha database.AuditLog
	err := database.DB.Where("action = ? AND target_id = ?", "server.address_refused", outra.ID).Take(&linha).Error
	if err != nil {
		t.Fatalf("a coleta SSH recusou o endereço sem deixar linha de auditoria: %v", err)
	}
	if linha.TargetLabel != "m5-vps-comprometida" || !strings.Contains(linha.Detail, "m5-vps-dona-ssh") {
		t.Errorf("a linha não identifica quem declarou nem o dono: rótulo=%q detalhe=%s", linha.TargetLabel, linha.Detail)
	}
}

func TestCadastroComErroAoConferirDonoNaoCria(t *testing.T) {
	setupAuditAPI(t)
	sess := sessaoReal(t, "admin-m5-falha-fechada", auth.RoleAdmin)
	limparServidorCadastrado(t, "m5-nao-deveria-nascer")

	if err := database.DB.Exec("ALTER TABLE server_addresses RENAME TO server_addresses_m5_fora").Error; err != nil {
		t.Fatalf("tirar a tabela do lugar: %v", err)
	}
	t.Cleanup(func() {
		database.DB.Exec("ALTER TABLE IF EXISTS server_addresses_m5_fora RENAME TO server_addresses")
	})

	rec := pedirComSessao(t, http.MethodPost, "/api/servers",
		`{"host_ip":"198.51.100.40","name":"m5-nao-deveria-nascer","user":"root","port":22}`, sess)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("conferência de dono com erro de banco: status %d, esperado 500 (%s)", rec.Code, rec.Body.String())
	}

	var nascidos int64
	database.DB.Unscoped().Model(&database.Server{}).Where("name = ?", "m5-nao-deveria-nascer").Count(&nascidos)
	if nascidos != 0 {
		t.Errorf("o servidor foi criado mesmo sem conseguir conferir o dono do endereço (%d linha)", nascidos)
	}
}

func TestPatchDeAliasComErroAoConferirDonoNaoGrava(t *testing.T) {
	setupAuditAPI(t)
	sess := sessaoReal(t, "admin-m5-falha-alias", auth.RoleAdmin)
	s := servidorDeRename(t, "m5-alias-sem-conferencia", "198.51.100.41", nil)

	if err := database.DB.Exec("ALTER TABLE sites RENAME TO sites_m5_fora").Error; err != nil {
		t.Fatalf("tirar a tabela do lugar: %v", err)
	}
	t.Cleanup(func() { database.DB.Exec("ALTER TABLE IF EXISTS sites_m5_fora RENAME TO sites") })

	rec := pedirComSessao(t, http.MethodPatch, "/api/servers?id="+s.ID, `{"aliases":["100.100.0.91"]}`, sess)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("alias com erro de banco na conferência: status %d, esperado 500 (%s)", rec.Code, rec.Body.String())
	}
	database.DB.Exec("ALTER TABLE IF EXISTS sites_m5_fora RENAME TO sites")

	var gravados int64
	database.DB.Model(&database.ServerAddress{}).Where("server_id = ? AND address = ?", s.ID, "100.100.0.91").Count(&gravados)
	if gravados != 0 {
		t.Error("o alias foi gravado mesmo sem conseguir conferir o dono")
	}
}

func TestEmailEmUsoComBancoForaDevolveErro(t *testing.T) {
	setupAuditAPI(t)

	comBancoFechado(t, func() {
		emUso, err := emailEmUso("alguem@exemplo.com", 0)
		if err == nil {
			t.Errorf("emailEmUso com o banco fora devolveu (%t, nil); sem erro o cadastro segue como se o e-mail estivesse livre", emUso)
		}
	})
}

func TestContagemDeDashboardsComBancoForaDevolveErro(t *testing.T) {
	setupAuditAPI(t)

	comBancoFechado(t, func() {
		total, err := dashboardsDoDono(1)
		if err == nil {
			t.Errorf("dashboardsDoDono com o banco fora devolveu (%d, nil); sem erro o teto de 20 não é aplicado", total)
		}
	})
}

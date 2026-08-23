package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/joaov/vd_stats/internal/audit"
	"github.com/joaov/vd_stats/internal/auth"
	"github.com/joaov/vd_stats/internal/database"
)

// Item S8: o painel executa docker start/stop/restart como root na VPS e não
// registrava quem mandou. Estes testes cobrem os três desfechos da rota —
// executada, recusada por alcance e recusada por argumento.

const (
	srvAuditA = "00000000-0000-0000-0000-00000000e001"
	srvAuditB = "00000000-0000-0000-0000-00000000e002"
)

// setupAuditRouteDB planta duas filiais com um servidor cada. Sem DATABASE_URL o
// teste é pulado, no mesmo modelo de scope_read_test.go.
func setupAuditRouteDB(t *testing.T) (filialA, filialB uint) {
	t.Helper()

	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("DATABASE_URL não definido; pulando teste de auditoria da rota")
	}
	if database.DB == nil {
		if err := database.Connect(); err != nil {
			t.Skipf("banco indisponível: %v", err)
		}
	}

	limpaAuditRoute(t)
	t.Cleanup(func() { limpaAuditRoute(t) })

	sedeA := database.Site{Name: "Filial A da auditoria", Code: "audit-a"}
	sedeB := database.Site{Name: "Filial B da auditoria", Code: "audit-b"}
	for _, s := range []*database.Site{&sedeA, &sedeB} {
		if err := database.DB.Create(s).Error; err != nil {
			t.Fatalf("criar unidade %s: %v", s.Code, err)
		}
	}

	servidores := []database.Server{
		{ID: srvAuditA, Name: "host-audit-a", HostIP: "10.91.0.1", SiteID: &sedeA.ID},
		{ID: srvAuditB, Name: "host-audit-b", HostIP: "10.91.0.2", SiteID: &sedeB.ID},
	}
	for _, s := range servidores {
		if err := database.DB.Create(&s).Error; err != nil {
			t.Fatalf("criar servidor %s: %v", s.Name, err)
		}
	}

	return sedeA.ID, sedeB.ID
}

// Unscoped no servidor porque Server tem exclusão lógica: sem ele a linha
// sobrevive com o mesmo id e a execução seguinte esbarra na chave primária.
func limpaAuditRoute(t *testing.T) {
	t.Helper()

	database.DB.Where("action LIKE ?", "container.%").Delete(&database.AuditLog{})
	database.DB.Unscoped().Where("id IN ?", []string{srvAuditA, srvAuditB}).Delete(&database.Server{})
	database.DB.Where("code IN ?", []string{"audit-a", "audit-b"}).Delete(&database.Site{})
}

// sessaoDaFilial monta a sessão de um operador que só alcança uma unidade.
func sessaoDaFilial(id uint) auth.Session {
	return auth.Session{
		UserID:   77,
		Username: "operador-do-teste",
		Role:     auth.RoleOperator,
		Accesses: []auth.Access{{SiteID: &id, Role: auth.RoleOperator}},
	}
}

// acaoDeContainer dispara a rota com a sessão já no contexto, como o middleware
// a entrega.
func acaoDeContainer(sess auth.Session, corpo string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/containers/action", strings.NewReader(corpo))
	Config{}.containerActionHandler(rec, withSession(req, sess))
	return rec
}

// ultimaLinhaDeAuditoria devolve o registro mais recente de ação de container.
func ultimaLinhaDeAuditoria(t *testing.T) database.AuditLog {
	t.Helper()

	var row database.AuditLog
	err := database.DB.Where("action LIKE ?", "container.%").Order("id DESC").First(&row).Error
	if err != nil {
		t.Fatalf("nenhuma linha de auditoria foi gravada: %v", err)
	}
	return row
}

func detalheDe(t *testing.T, row database.AuditLog) map[string]any {
	t.Helper()

	var m map[string]any
	if err := json.Unmarshal([]byte(row.Detail), &m); err != nil {
		t.Fatalf("detalhe não é JSON: %q (%v)", row.Detail, err)
	}
	return m
}

// A execução falha porque não há chave SSH no ambiente de teste, e é exatamente
// o desfecho que interessa: a linha precisa existir mesmo assim. Se o registro
// fosse feito só depois de um comando bem-sucedido, este caso não deixaria
// rastro nenhum — e comando que falhou é o que mais se quer auditar.
//
// A presença simultânea do detalhe gravado ANTES (container, host) e do gravado
// DEPOIS (erro) é a prova de que as duas fases correram.
func TestAcaoDeContainerFalhaMasDeixaRastro(t *testing.T) {
	filialA, _ := setupAuditRouteDB(t)

	rec := acaoDeContainer(sessaoDaFilial(filialA),
		`{"server_id":"`+srvAuditA+`","container_name":"nginx_proxy","action":"restart"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, esperado 400 (sem chave SSH o comando não sai)", rec.Code)
	}

	row := ultimaLinhaDeAuditoria(t)
	if row.Action != "container.restart" {
		t.Errorf("ação = %q, esperada container.restart", row.Action)
	}
	if row.Result != audit.ResultError {
		t.Errorf("resultado = %q, esperado %q", row.Result, audit.ResultError)
	}
	if row.ActorUsername != "operador-do-teste" {
		t.Errorf("ator = %q, esperado operador-do-teste", row.ActorUsername)
	}
	if row.TargetID != srvAuditA || row.TargetLabel != "host-audit-a" {
		t.Errorf("alvo = %q/%q, esperado o servidor da filial A", row.TargetID, row.TargetLabel)
	}
	if row.SiteID == nil || *row.SiteID != filialA {
		t.Errorf("unidade = %v, esperada %d", row.SiteID, filialA)
	}

	detalhe := detalheDe(t, row)
	if detalhe["container"] != "nginx_proxy" {
		t.Errorf("o detalhe gravado antes da execução se perdeu: %v", detalhe)
	}
	if detalhe["host"] != "10.91.0.1" {
		t.Errorf("host = %v, esperado 10.91.0.1", detalhe["host"])
	}
	if detalhe["erro"] == nil {
		t.Errorf("o detalhe do resultado não foi gravado: %v", detalhe)
	}
}

// Tentar operar servidor de outra unidade é o sinal que mais justifica a
// auditoria existir. lookupServer responde 404 sem confirmar a existência, o
// que deixaria o episódio inteiramente invisível sem esta linha.
func TestAcaoEmUnidadeAlheiaRegistraRecusa(t *testing.T) {
	filialA, _ := setupAuditRouteDB(t)

	rec := acaoDeContainer(sessaoDaFilial(filialA),
		`{"server_id":"`+srvAuditB+`","container_name":"nginx","action":"stop"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, esperado 404", rec.Code)
	}

	row := ultimaLinhaDeAuditoria(t)
	if row.Result != audit.ResultDenied {
		t.Errorf("resultado = %q, esperado %q", row.Result, audit.ResultDenied)
	}
	if row.Action != "container.stop" {
		t.Errorf("ação = %q, esperada container.stop", row.Action)
	}
	if row.TargetID != srvAuditB {
		t.Errorf("alvo = %q, esperado o servidor da filial B", row.TargetID)
	}
	if row.ActorUsername != "operador-do-teste" {
		t.Errorf("ator = %q, esperado operador-do-teste", row.ActorUsername)
	}
}

// A coluna de ação não pode receber texto escolhido pelo cliente: seria mais uma
// superfície, e uma tabela de auditoria poluída deixa de ser consultável.
func TestAcaoInvalidaNaoInterpolaOCorpoNaColuna(t *testing.T) {
	filialA, _ := setupAuditRouteDB(t)

	rec := acaoDeContainer(sessaoDaFilial(filialA),
		`{"server_id":"`+srvAuditA+`","container_name":"nginx","action":"rm -rf /"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, esperado 400", rec.Code)
	}

	row := ultimaLinhaDeAuditoria(t)
	if row.Action != "container.invalid" {
		t.Errorf("ação = %q, esperada container.invalid — o corpo vazou para a coluna", row.Action)
	}
	if row.Result != audit.ResultDenied {
		t.Errorf("resultado = %q, esperado %q", row.Result, audit.ResultDenied)
	}
}

// O nome do container entra no detalhe da linha, gravado antes de o pacote ssh
// ter chance de recusá-lo — por isso a conferência precisa acontecer no handler.
func TestNomeDeContainerInvalidoRecusadoAntesDoRegistro(t *testing.T) {
	filialA, _ := setupAuditRouteDB(t)

	rec := acaoDeContainer(sessaoDaFilial(filialA),
		`{"server_id":"`+srvAuditA+`","container_name":"-f","action":"stop"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, esperado 400", rec.Code)
	}

	row := ultimaLinhaDeAuditoria(t)
	if row.Action != "container.invalid" {
		t.Errorf("ação = %q, esperada container.invalid", row.Action)
	}
	if strings.Contains(row.Detail, "-f") {
		t.Errorf("o nome recusado foi gravado no detalhe: %q", row.Detail)
	}
}

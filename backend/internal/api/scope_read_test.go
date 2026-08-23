package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/joaov/vd_stats/internal/auth"
	"github.com/joaov/vd_stats/internal/database"
)

// Regressão do furo S1. As três rotas de leitura corrigidas aqui — busca de
// log, descoberta de SSL e lista de regras — não liam a sessão e devolviam o
// parque inteiro para qualquer visualizador, inclusive um restrito a uma
// filial.

const (
	srvDaFilialA = "00000000-0000-0000-0000-00000000a001"
	srvDaFilialB = "00000000-0000-0000-0000-0000000b0001"
	srvSemSite   = "00000000-0000-0000-0000-00000000c001"
	srvApagado   = "00000000-0000-0000-0000-00000000d001"

	vhostDeTeste   = "painel-do-teste.exemplo.com"
	vhostDaFilialB = "painel-da-filial-b.exemplo.com"
)

func ptrSite(id uint) *uint { return &id }

// escopoDaFilial monta o recorte de quem só alcança uma unidade, do mesmo jeito
// que resolveScope o monta a partir da sessão.
func escopoDaFilial(id uint) siteScope {
	return siteScope{filter: true, ids: []uint{id}}
}

func nomesDasRegras(rules []database.AlertRule) []string {
	out := make([]string, 0, len(rules))
	for _, r := range rules {
		out = append(out, r.Name)
	}
	return out
}

// TestVisibilidadeDasRegrasPorUnidade cobre as quatro formas de uma regra se
// amarrar (ou não) a uma unidade. É teste puro de propósito: a decisão de
// visibilidade não pode depender de banco para ser verificável.
func TestVisibilidadeDasRegrasPorUnidade(t *testing.T) {
	const filialA, filialB = uint(1), uint(2)

	sites := map[string]*uint{
		srvDaFilialA: ptrSite(filialA),
		srvDaFilialB: ptrSite(filialB),
		srvSemSite:   nil,
	}

	regras := []database.AlertRule{
		{Name: "parque", Target: "*"},
		{Name: "unidade-A", Target: "*", TargetSiteID: ptrSite(filialA)},
		{Name: "unidade-B", Target: "*", TargetSiteID: ptrSite(filialB)},
		{Name: "host-A", Target: srvDaFilialA},
		{Name: "host-B", Target: srvDaFilialB},
		{Name: "host-sem-unidade", Target: srvSemSite},
		{Name: "host-apagado", Target: srvApagado},
	}

	casos := []struct {
		nome      string
		scope     siteScope
		hasGlobal bool
		esperado  []string
	}{
		{
			nome:      "visualizador restrito à filial A",
			scope:     escopoDaFilial(filialA),
			hasGlobal: false,
			esperado:  []string{"unidade-A", "host-A"},
		},
		{
			nome:      "visualizador restrito à filial B",
			scope:     escopoDaFilial(filialB),
			hasGlobal: false,
			esperado:  []string{"unidade-B", "host-B"},
		},
		{
			nome:      "acesso global sem filtro vê tudo",
			scope:     siteScope{},
			hasGlobal: true,
			esperado: []string{
				"parque", "unidade-A", "unidade-B",
				"host-A", "host-B", "host-sem-unidade", "host-apagado",
			},
		},
		{
			// A regra de parque vale também para os hosts da filial escolhida:
			// escondê-la de quem tem acesso global só porque filtrou a tela
			// faria sumir justamente a regra que está disparando ali.
			nome:      "acesso global filtrando uma unidade mantém a regra de parque",
			scope:     escopoDaFilial(filialA),
			hasGlobal: true,
			esperado:  []string{"parque", "unidade-A", "host-A", "host-apagado"},
		},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			got := nomesDasRegras(visibleRules(regras, c.scope, sites, c.hasGlobal))
			if len(got) != len(c.esperado) {
				t.Fatalf("regras visíveis = %v, esperado %v", got, c.esperado)
			}
			for i := range got {
				if got[i] != c.esperado[i] {
					t.Fatalf("regras visíveis = %v, esperado %v", got, c.esperado)
				}
			}
		})
	}
}

// TestRegraSemUnidadeNaoVazaParaFilial isola o caso que o recorte por SQL
// deixaria passar: a regra de parque inteiro e a regra cujo servidor alvo já
// não existe não pertencem a filial nenhuma, então nenhum escopo restrito pode
// enxergá-las.
func TestRegraSemUnidadeNaoVazaParaFilial(t *testing.T) {
	sites := map[string]*uint{srvDaFilialA: ptrSite(1)}

	orfas := []database.AlertRule{
		{Name: "parque", Target: "*"},
		{Name: "host-apagado", Target: srvApagado},
		{Name: "host-sem-unidade", Target: srvSemSite},
	}

	for _, regra := range orfas {
		t.Run(regra.Name, func(t *testing.T) {
			visiveis := visibleRules([]database.AlertRule{regra}, escopoDaFilial(1), sites, false)
			if len(visiveis) != 0 {
				t.Errorf("regra %q apareceu para quem só alcança a filial 1", regra.Name)
			}
			if len(visibleRules([]database.AlertRule{regra}, siteScope{}, sites, true)) != 1 {
				t.Errorf("regra %q sumiu para quem tem concessão global", regra.Name)
			}
		})
	}
}

// TestRuleSiteIDResolveAUnidadeDoAlvo trava a resolução alvo -> unidade, que é
// de onde a visibilidade da regra por servidor sai.
func TestRuleSiteIDResolveAUnidadeDoAlvo(t *testing.T) {
	sites := map[string]*uint{
		srvDaFilialA: ptrSite(7),
		srvSemSite:   nil,
	}

	casos := []struct {
		nome       string
		regra      database.AlertRule
		querSite   *uint
		querAmarra bool
	}{
		{"unidade explícita ganha do alvo", database.AlertRule{Target: srvDaFilialA, TargetSiteID: ptrSite(9)}, ptrSite(9), true},
		{"alvo por servidor herda a unidade", database.AlertRule{Target: srvDaFilialA}, ptrSite(7), true},
		{"servidor sem unidade continua amarrado", database.AlertRule{Target: srvSemSite}, nil, true},
		{"servidor apagado desamarra", database.AlertRule{Target: srvApagado}, nil, false},
		{"curinga não amarra", database.AlertRule{Target: "*"}, nil, false},
		{"alvo vazio não amarra", database.AlertRule{Target: ""}, nil, false},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			site, amarrada := ruleSiteID(c.regra, sites)
			if amarrada != c.querAmarra {
				t.Fatalf("amarrada = %v, esperado %v", amarrada, c.querAmarra)
			}
			switch {
			case c.querSite == nil && site != nil:
				t.Fatalf("unidade = %d, esperado nenhuma", *site)
			case c.querSite != nil && site == nil:
				t.Fatalf("unidade = nenhuma, esperado %d", *c.querSite)
			case c.querSite != nil && *site != *c.querSite:
				t.Fatalf("unidade = %d, esperado %d", *site, *c.querSite)
			}
		})
	}
}

// setupScopeDB liga no Postgres e planta duas filiais com um servidor, uma
// linha de log e uma regra cada. Sem DATABASE_URL o teste é pulado, no mesmo
// modelo de internal/database/retention_test.go: a suíte precisa passar numa
// máquina sem banco.
func setupScopeDB(t *testing.T) (filialA, filialB uint) {
	t.Helper()

	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("DATABASE_URL não definido; pulando teste de recorte por unidade")
	}
	if database.DB == nil {
		if err := database.Connect(); err != nil {
			t.Skipf("banco indisponível: %v", err)
		}
	}

	limpaEscopo(t)
	t.Cleanup(func() { limpaEscopo(t) })

	sedeA := database.Site{Name: "Filial A do teste", Code: "teste-a"}
	sedeB := database.Site{Name: "Filial B do teste", Code: "teste-b"}
	for _, s := range []*database.Site{&sedeA, &sedeB} {
		if err := database.DB.Create(s).Error; err != nil {
			t.Fatalf("criar unidade %s: %v", s.Code, err)
		}
	}

	servidores := []database.Server{
		{ID: srvDaFilialA, Name: "host-teste-a", HostIP: "10.90.0.1", SiteID: &sedeA.ID},
		{ID: srvDaFilialB, Name: "host-teste-b", HostIP: "10.90.0.2", SiteID: &sedeB.ID},
	}
	for _, s := range servidores {
		if err := database.DB.Create(&s).Error; err != nil {
			t.Fatalf("criar servidor %s: %v", s.Name, err)
		}
	}

	agora := time.Now().UTC()
	logs := []database.LogEntry{
		{ServerID: srvDaFilialA, Source: "auth", Line: "linha-da-filial-a", Timestamp: agora},
		{ServerID: srvDaFilialB, Source: "auth", Line: "linha-da-filial-b", Timestamp: agora},
	}
	for _, l := range logs {
		if err := database.DB.Create(&l).Error; err != nil {
			t.Fatalf("criar log de %s: %v", l.ServerID, err)
		}
	}

	regras := []database.AlertRule{
		{Name: "regra-teste-parque", Target: "*", Metric: "cpu", Operator: ">", Threshold: 90},
		{Name: "regra-teste-unidade-a", Target: "*", Metric: "cpu", Operator: ">", Threshold: 90, TargetSiteID: &sedeA.ID},
		{Name: "regra-teste-host-b", Target: srvDaFilialB, Metric: "cpu", Operator: ">", Threshold: 90},
	}
	for _, regra := range regras {
		if err := database.DB.Create(&regra).Error; err != nil {
			t.Fatalf("criar regra %s: %v", regra.Name, err)
		}
	}

	// Um vhost observado em cada filial. Com um só, a descoberta devolveria
	// lista vazia para o viewer restrito mesmo sem recorte nenhum, e o teste
	// passaria por acidente — foi o que aconteceu na primeira versão dele.
	vhosts := []database.MetricLoadBalancer{
		{
			UpstreamAddr: "10.90.0.9:443", ServerName: vhostDeTeste, Status: "200",
			ServerID: ptr(srvDaFilialA), SiteID: &sedeA.ID, RequestsCount: 5, Timestamp: agora,
		},
		{
			UpstreamAddr: "10.90.0.10:443", ServerName: vhostDaFilialB, Status: "200",
			ServerID: ptr(srvDaFilialB), SiteID: &sedeB.ID, RequestsCount: 7, Timestamp: agora,
		},
	}
	for _, v := range vhosts {
		if err := database.DB.Create(&v).Error; err != nil {
			t.Fatalf("criar vhost observado %s: %v", v.ServerName, err)
		}
	}

	return sedeA.ID, sedeB.ID
}

// Unscoped no servidor porque Server tem exclusão lógica: sem ele a linha
// sobrevive com o mesmo id e o teste seguinte esbarra na chave primária.
func limpaEscopo(t *testing.T) {
	t.Helper()

	database.DB.Where("server_id IN ?", []string{srvDaFilialA, srvDaFilialB}).Delete(&database.LogEntry{})
	database.DB.Where("name LIKE ?", "regra-teste-%").Delete(&database.AlertRule{})
	database.DB.Unscoped().Where("id IN ?", []string{srvDaFilialA, srvDaFilialB}).Delete(&database.Server{})
	database.DB.Where("code IN ?", []string{"teste-a", "teste-b"}).Delete(&database.Site{})
	database.DB.Where("server_name IN ?", []string{vhostDeTeste, vhostDaFilialB}).
		Delete(&database.MetricLoadBalancer{})
}

// pedeComoSessao chama o handler com a sessão já no contexto, como o middleware
// a entrega.
func pedeComoSessao(h http.HandlerFunc, sess auth.Session, url string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h(rec, withSession(httptest.NewRequest(http.MethodGet, url, nil), sess))
	return rec
}

// TestBuscaDeLogNaoVazaEntreUnidades cobre os dois caminhos que o furo abria:
// a busca sem server_id, que devolvia os 200 últimos do parque, e a busca com
// server_id, que entregava o log de qualquer host a quem soubesse o uuid.
func TestBuscaDeLogNaoVazaEntreUnidades(t *testing.T) {
	filialA, _ := setupScopeDB(t)

	viewer := sessaoDeTeste(t, "olheiro-da-filial-a", []auth.Access{
		{SiteID: &filialA, Role: auth.RoleViewer},
	})

	rec := pedeComoSessao(LogSearchHandler, viewer, "/api/logs/search?q=linha-da-filial")
	if rec.Code != http.StatusOK {
		t.Fatalf("busca de log: status = %d", rec.Code)
	}
	var entries []database.LogEntry
	if err := json.NewDecoder(rec.Body).Decode(&entries); err != nil {
		t.Fatalf("decode da busca: %v", err)
	}
	for _, e := range entries {
		if e.ServerID == srvDaFilialB {
			t.Errorf("log da filial B (%q) apareceu para quem só alcança a filial A", e.Line)
		}
	}
	if len(entries) != 1 {
		t.Errorf("linhas visíveis = %d, esperado 1 (só a da filial A)", len(entries))
	}

	rec = pedeComoSessao(LogSearchHandler, viewer, "/api/logs/search?server_id="+srvDaFilialB)
	if rec.Code != http.StatusNotFound {
		t.Errorf("log de host fora do alcance: status = %d, esperado 404", rec.Code)
	}

	rec = pedeComoSessao(LogSearchHandler, viewer, "/api/logs/search?server_id="+srvDaFilialA)
	if rec.Code != http.StatusOK {
		t.Errorf("log do próprio host recusado: status = %d", rec.Code)
	}
}

// TestListaDeRegrasNaoVazaEntreUnidades exercita AlertRulesHandler inteiro, e
// não só o filtro puro, para pegar um recorte que deixasse de ser chamado.
func TestListaDeRegrasNaoVazaEntreUnidades(t *testing.T) {
	filialA, _ := setupScopeDB(t)

	viewer := sessaoDeTeste(t, "olheiro-de-regras", []auth.Access{
		{SiteID: &filialA, Role: auth.RoleViewer},
	})

	rec := pedeComoSessao(AlertRulesHandler, viewer, "/api/alerts/rules")
	if rec.Code != http.StatusOK {
		t.Fatalf("lista de regras: status = %d", rec.Code)
	}
	var regras []database.AlertRule
	if err := json.NewDecoder(rec.Body).Decode(&regras); err != nil {
		t.Fatalf("decode das regras: %v", err)
	}

	for _, r := range regras {
		switch r.Name {
		case "regra-teste-parque":
			t.Error("regra de parque inteiro apareceu para quem só alcança uma filial")
		case "regra-teste-host-b":
			t.Error("regra de host da filial B apareceu para quem só alcança a filial A")
		}
	}

	var viuAUnidade bool
	for _, r := range regras {
		if r.Name == "regra-teste-unidade-a" {
			viuAUnidade = true
		}
	}
	if !viuAUnidade {
		t.Error("a regra da própria filial sumiu da lista")
	}
}

// TestDescobertaDeSslRecortaPorUnidade substitui o teste que exigia concessão
// global nesta rota.
//
// Enquanto metric_load_balancers não tinha unidade, a única saída era negar a
// lista inteira a quem é restrito — e o admin de filial via a tela de descoberta
// em branco. Com a coluna site_id, o recorte passou a ser possível, e negar
// virou custo sem contrapartida.
func TestDescobertaDeSslRecortaPorUnidade(t *testing.T) {
	filialA, _ := setupScopeDB(t)

	viewer := sessaoDeTeste(t, "olheiro-de-ssl", []auth.Access{
		{SiteID: &filialA, Role: auth.RoleViewer},
	})

	vistos := descobreDominios(t, viewer)
	if !vistos[vhostDeTeste] {
		t.Errorf("o vhost da própria unidade não apareceu para o viewer da filial A")
	}
	if vistos[vhostDaFilialB] {
		t.Errorf("o vhost %q, da filial B, vazou para o viewer da filial A", vhostDaFilialB)
	}
}

// O contraponto: quem tem concessão global continua vendo o parque inteiro.
// Sem ele, um recorte que negasse tudo passaria no teste acima.
func TestDescobertaDeSslMostraTudoParaGlobal(t *testing.T) {
	setupScopeDB(t)

	global := sessaoDeTeste(t, "olheiro-global-de-ssl", []auth.Access{
		{SiteID: nil, Role: auth.RoleViewer},
	})

	vistos := descobreDominios(t, global)
	for _, nome := range []string{vhostDeTeste, vhostDaFilialB} {
		if !vistos[nome] {
			t.Errorf("o vhost %q não apareceu para quem tem concessão global", nome)
		}
	}
}

func descobreDominios(t *testing.T, sess auth.Session) map[string]bool {
	t.Helper()

	rec := pedeComoSessao(sslDiscoverHandler, sess, "/api/ssl/discover")
	if rec.Code != http.StatusOK {
		t.Fatalf("descoberta de SSL: status = %d", rec.Code)
	}
	var domains []discoveredDomain
	if err := json.NewDecoder(rec.Body).Decode(&domains); err != nil {
		t.Fatalf("decode da descoberta: %v", err)
	}

	vistos := make(map[string]bool, len(domains))
	for _, d := range domains {
		vistos[d.Domain] = true
	}
	return vistos
}

func ptr(s string) *string { return &s }

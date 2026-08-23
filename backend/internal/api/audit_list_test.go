package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/joaov/vd_stats/internal/auth"
	"github.com/joaov/vd_stats/internal/database"
)

const acaoDeListagem = "listagem-teste.create"

func semearAuditoria(t *testing.T, quantas int) {
	t.Helper()

	agora := time.Now().UTC()
	linhas := make([]database.AuditLog, 0, quantas)
	for i := 0; i < quantas; i++ {
		linhas = append(linhas, database.AuditLog{
			// Todas no MESMO instante de propósito: é o caso que expõe
			// paginação sem desempate estável.
			At:            agora,
			Action:        acaoDeListagem,
			ActorUsername: "ator-de-teste",
			Result:        "ok",
			Detail:        "{}",
		})
	}
	if err := database.DB.Create(&linhas).Error; err != nil {
		t.Fatalf("semear auditoria: %v", err)
	}
	t.Cleanup(func() {
		database.DB.Where("action = ?", acaoDeListagem).Delete(&database.AuditLog{})
	})
}

func listarAuditoria(t *testing.T, query string) auditListPage {
	t.Helper()

	rec := httptest.NewRecorder()
	auditListHandler(rec, httptest.NewRequest(http.MethodGet, "/api/audit?"+query, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, corpo = %s", rec.Code, rec.Body.String())
	}

	var page auditListPage
	if err := json.NewDecoder(rec.Body).Decode(&page); err != nil {
		t.Fatalf("decodificar resposta: %v", err)
	}
	return page
}

// Sem teto de LIMIT, a primeira consulta numa base de um ano carrega a tabela
// inteira para a memória do processo.
func TestListagemLimitaOTamanhoDaPagina(t *testing.T) {
	setupAuditAPI(t)
	semearAuditoria(t, 5)

	page := listarAuditoria(t, "action="+acaoDeListagem+"&limit=99999")

	if page.Limit != maxAuditLimit {
		t.Errorf("limit = %d, esperado o teto de %d", page.Limit, maxAuditLimit)
	}
}

// Percorre as páginas e confere que cada linha aparece uma vez só.
//
// Não prova o desempate por id do ORDER BY: medido, o teste continua verde sem
// ele, porque neste tamanho o Postgres devolve a ordem de inserção por acaso do
// plano. O desempate fica assim mesmo — "ORDER BY at DESC" com empate é ordem
// indefinida pelo padrão, e o dia em que o plano virar index scan a paginação
// passaria a repetir e pular linha em silêncio.
func TestPaginacaoNaoRepeteNemPulaLinha(t *testing.T) {
	setupAuditAPI(t)
	semearAuditoria(t, 6)

	vistos := map[uint]bool{}
	for offset := 0; offset < 6; offset += 2 {
		page := listarAuditoria(t,
			"action="+acaoDeListagem+"&limit=2&offset="+strconv.Itoa(offset))
		for _, l := range page.Items {
			if vistos[l.ID] {
				t.Errorf("linha %d apareceu em duas páginas", l.ID)
			}
			vistos[l.ID] = true
		}
	}
	if len(vistos) != 6 {
		t.Errorf("linhas distintas vistas = %d, esperadas 6: a paginação pulou registro", len(vistos))
	}
}

func TestListagemContaOTotalIndependenteDaPagina(t *testing.T) {
	setupAuditAPI(t)
	semearAuditoria(t, 5)

	page := listarAuditoria(t, "action="+acaoDeListagem+"&limit=2")

	if len(page.Items) != 2 {
		t.Errorf("itens = %d, esperados 2", len(page.Items))
	}
	if page.Total != 5 {
		t.Errorf("total = %d, esperado 5: a tela não teria como saber que há próxima página", page.Total)
	}
}

// Filtro de tempo inválido não pode ser ignorado em silêncio: a tela mostraria
// o período errado e ninguém saberia.
func TestIntervaloInvalidoRecusaEmVezDeIgnorar(t *testing.T) {
	setupAuditAPI(t)

	rec := httptest.NewRecorder()
	auditListHandler(rec, httptest.NewRequest(http.MethodGet, "/api/audit?from=ontem", nil))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, esperado 400", rec.Code)
	}
}

// A auditoria mostra ação de todas as unidades. Um administrador de filial não
// pode ler a lista de quem o administra.
func TestListagemExigeAdminGlobal(t *testing.T) {
	cfg := testConfig()
	filial := uint(4)

	adminDeFilial := sessaoDeTeste(t, "admin-da-filial", []auth.Access{
		{SiteID: &filial, Role: auth.RoleAdmin},
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/audit", nil)
	req.Header.Set("Authorization", "Bearer "+adminDeFilial.Token)
	cfg.requireGlobalRole(auth.RoleAdmin)(okHandler)(rec, req)

	if rec.Code == http.StatusOK {
		t.Error("administrador de uma filial leu a auditoria do parque inteiro")
	}
}

package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/jvS0uzx/dock_keeper/internal/database"
)

const tokenLegadoDeTeste = "token-legado-de-teste-n3"

func corpoComHosts(n int, prefixo, sufixo string) string {
	var b strings.Builder
	b.WriteString(prefixo)
	b.WriteString(`"hosts":[`)
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(`{}`)
	}
	b.WriteString(`]`)
	b.WriteString(sufixo)
	return b.String()
}

func requisicaoDeInventario(t *testing.T, corpo string) *httptest.ResponseRecorder {
	t.Helper()
	t.Setenv("AGENT_INGEST_TOKEN", tokenLegadoDeTeste)
	t.Setenv("ALLOW_LEGACY_INGEST_TOKEN", "true")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/ingest/inventory", strings.NewReader(corpo))
	req.Header.Set("X-Agent-Token", tokenLegadoDeTeste)
	InventoryIngestHandler(rec, req)
	return rec
}

func TestInventarioEstouradoRecusadoDuranteODecode(t *testing.T) {
	corpo := corpoComHosts(maxInventoryHosts+1, `{"site_code":"qa-n3",`, `, lixo-que-nao-e-json`)
	rec := requisicaoDeInventario(t, corpo)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, esperado 413: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "inventário grande demais") {
		t.Errorf("mensagem mudou: %s", rec.Body.String())
	}
}

func TestInventarioNoTetoExatoPassaDoDecode(t *testing.T) {
	setupInventarioCap(t)

	corpo := corpoComHosts(maxInventoryHosts, `{"site_code":"qa-n3-cap",`, `}`)
	rec := requisicaoDeInventario(t, corpo)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, esperado 200: %s", rec.Code, rec.Body.String())
	}
}

func TestSiteCodeDepoisDeHostsContinuaAceito(t *testing.T) {
	setupInventarioCap(t)

	corpo := `{"hosts":[{"ip":"10.93.7.1","hostname":"qa-n3-h1"},{"ip":"10.93.7.2","hostname":"qa-n3-h2"}],` +
		`"campo_desconhecido":{"x":1},"collector_version":"qa-teste","site_code":"qa-n3-cap"}`
	rec := requisicaoDeInventario(t, corpo)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, esperado 200: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"stored":2`) {
		t.Errorf("esperava 2 hosts gravados: %s", rec.Body.String())
	}
}

func setupInventarioCap(t *testing.T) {
	t.Helper()

	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("DATABASE_URL não definido; pulando teste de inventário")
	}
	if database.DB == nil {
		if err := database.Connect(); err != nil {
			t.Skipf("banco indisponível: %v", err)
		}
	}
	limparInventarioCap(t)
	t.Cleanup(func() { limparInventarioCap(t) })

	site := database.Site{Name: "qa-n3-cap", Code: "qa-n3-cap"}
	if err := database.DB.Create(&site).Error; err != nil {
		t.Fatalf("criar unidade: %v", err)
	}
}

func limparInventarioCap(t *testing.T) {
	t.Helper()
	database.DB.Where("ip LIKE ?", "10.93.7.%").Delete(&database.NetworkHost{})
	database.DB.Where("code = ?", "qa-n3-cap").Delete(&database.Site{})
}

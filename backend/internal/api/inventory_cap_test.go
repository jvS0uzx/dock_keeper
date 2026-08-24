package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/joaov/vd_stats/internal/database"
)

const tokenLegadoDeTeste = "token-legado-de-teste-n3"

// corpoComHosts monta um envio com n hosts. Objeto vazio serve: o decode não
// valida campo por host, e é a contagem que está em teste.
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

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/ingest/inventory", strings.NewReader(corpo))
	req.Header.Set("X-Agent-Token", tokenLegadoDeTeste)
	InventoryIngestHandler(rec, req)
	return rec
}

// O discriminador do item N3: depois do host 5001 o corpo traz lixo que não é
// JSON. O decode antigo materializava a lista inteira antes de conferir o teto,
// então tropeçava no lixo e respondia 400; o decode incremental recusa no host
// 5001 sem ler o resto, e a resposta é 413. Este teste falha na implementação
// antiga.
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

// No teto exato o envio passa do decode e segue o fluxo normal — a recusa do
// teste acima não pode ter virado um off-by-one que rejeita envio legítimo.
func TestInventarioNoTetoExatoPassaDoDecode(t *testing.T) {
	setupInventarioCap(t)

	corpo := corpoComHosts(maxInventoryHosts, `{"site_code":"qa-n3-cap",`, `}`)
	rec := requisicaoDeInventario(t, corpo)

	// Hosts sem IP são pulados na gravação; o que importa é não ser 413.
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, esperado 200: %s", rec.Code, rec.Body.String())
	}
}

// site_code depois de hosts no JSON: o decode incremental percorre o objeto na
// ordem em que ele vem, e a ordem dos campos não é contrato do coletor.
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

package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/auth"
	"github.com/jvS0uzx/dock_keeper/internal/database"
)

type baseDoPainel struct {
	Nome         string `json:"nome"`
	Dono         string `json:"dono"`
	Encoding     string `json:"encoding"`
	TamanhoBytes *int64 `json:"tamanho_bytes"`
	Conexoes     *int   `json:"conexoes"`
	ObservadoEm  string `json:"observado_em"`
}

type instanciaDoPainel struct {
	ID                uint           `json:"id"`
	ServerID          string         `json:"server_id"`
	ServidorNome      string         `json:"servidor_nome"`
	SiteID            *uint          `json:"site_id"`
	Porta             int            `json:"porta"`
	Motor             string         `json:"motor"`
	Papel             string         `json:"papel"`
	Estado            string         `json:"estado"`
	Versao            string         `json:"versao"`
	TotalBases        int            `json:"total_bases"`
	TamanhoTotalBytes *int64         `json:"tamanho_total_bytes"`
	Bases             []baseDoPainel `json:"bases"`
}

func bytesDe(n int64) *int64 { return &n }

func instanciaDeBanco(t *testing.T, serverID string, porta int) database.PostgresInstancia {
	t.Helper()

	inst := database.PostgresInstancia{
		ServerID:    serverID,
		Porta:       porta,
		Papel:       "primario",
		Estado:      "ativo",
		Versao:      "18.0",
		WalLevel:    "replica",
		ArchiveMode: "off",
		ObservadoEm: time.Now().UTC().Truncate(time.Second),
	}
	if err := database.DB.Create(&inst).Error; err != nil {
		t.Fatalf("criar instância na porta %d: %v", porta, err)
	}
	t.Cleanup(func() {
		database.DB.Where("id = ?", inst.ID).Delete(&database.PostgresInstancia{})
	})
	return inst
}

func baseDeBanco(t *testing.T, instanciaID uint, nome string, tamanho *int64) {
	t.Helper()

	base := database.PostgresBase{
		InstanciaID:  instanciaID,
		Nome:         nome,
		Dono:         "postgres",
		Encoding:     "UTF8",
		TamanhoBytes: tamanho,
		ObservadoEm:  time.Now().UTC().Truncate(time.Second),
	}
	if err := database.DB.Create(&base).Error; err != nil {
		t.Fatalf("criar base %s: %v", nome, err)
	}
}

func lerBancos(t *testing.T, sess auth.Session, query string) []instanciaDoPainel {
	t.Helper()

	rec := pedirComSessao(t, http.MethodGet, "/api/bancos"+query, "", sess)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/bancos%s: status %d (%s)", query, rec.Code, rec.Body.String())
	}
	return decodificarBancos(t, rec)
}

func decodificarBancos(t *testing.T, rec *httptest.ResponseRecorder) []instanciaDoPainel {
	t.Helper()

	var out []instanciaDoPainel
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("resposta de /api/bancos não é a lista do contrato: %v (%s)", err, rec.Body.String())
	}
	return out
}

func somenteDosServidores(lista []instanciaDoPainel, ids ...string) []instanciaDoPainel {
	meus := make(map[string]bool, len(ids))
	for _, id := range ids {
		meus[id] = true
	}
	out := make([]instanciaDoPainel, 0, len(lista))
	for _, inst := range lista {
		if meus[inst.ServerID] {
			out = append(out, inst)
		}
	}
	return out
}

func acharInstancia(t *testing.T, lista []instanciaDoPainel, id uint) instanciaDoPainel {
	t.Helper()

	for _, inst := range lista {
		if inst.ID == id {
			return inst
		}
	}
	t.Fatalf("a listagem não trouxe a instância %d", id)
	return instanciaDoPainel{}
}

func TestBancosListaInstanciaComBasesAninhadasEOrdenadas(t *testing.T) {
	setupAuditAPI(t)
	sess := sessaoReal(t, "admin-bancos-lista", auth.RoleAdmin)
	srv := servidorDeRename(t, "qa-bancos-lista", "203.0.113.120", nil)

	inst := instanciaDeBanco(t, srv.ID, 5432)
	baseDeBanco(t, inst.ID, "zulu", nil)
	baseDeBanco(t, inst.ID, "alfa", bytesDe(1024))
	baseDeBanco(t, inst.ID, "meio", bytesDe(2048))

	achada := acharInstancia(t, lerBancos(t, sess, ""), inst.ID)

	if achada.ServidorNome != "qa-bancos-lista" {
		t.Errorf("servidor_nome = %q, esperado o nome do servidor dono da instância", achada.ServidorNome)
	}
	if achada.Porta != 5432 || achada.Papel != "primario" || achada.Estado != "ativo" {
		t.Errorf("instância = %+v, esperado porta 5432, papel primario e estado ativo", achada)
	}
	if achada.TotalBases != 3 {
		t.Errorf("total_bases = %d, esperado 3: a contagem não depende de ter tamanho medido", achada.TotalBases)
	}

	nomes := make([]string, 0, len(achada.Bases))
	for _, b := range achada.Bases {
		nomes = append(nomes, b.Nome)
	}
	if len(nomes) != 3 || nomes[0] != "alfa" || nomes[1] != "meio" || nomes[2] != "zulu" {
		t.Errorf("bases = %v, esperado [alfa meio zulu] em ordem de nome", nomes)
	}

	if achada.TamanhoTotalBytes == nil {
		t.Fatalf("tamanho_total_bytes veio null, esperado a soma das bases medidas")
	}
	if *achada.TamanhoTotalBytes != 3072 {
		t.Errorf("tamanho_total_bytes = %d, esperado 3072: a base sem tamanho não pode entrar como zero",
			*achada.TamanhoTotalBytes)
	}

	for _, b := range achada.Bases {
		if b.Nome == "zulu" && b.TamanhoBytes != nil {
			t.Errorf("tamanho_bytes da base zulu = %d, esperado null", *b.TamanhoBytes)
		}
		if b.Nome == "alfa" && (b.TamanhoBytes == nil || *b.TamanhoBytes != 1024) {
			t.Errorf("tamanho_bytes da base alfa = %v, esperado 1024", b.TamanhoBytes)
		}
		if b.Nome == "alfa" && (b.Dono != "postgres" || b.Encoding != "UTF8") {
			t.Errorf("base alfa = %+v, esperado dono e encoding gravados", b)
		}
	}
}

func TestBancosTrazOMotorDeCadaInstancia(t *testing.T) {
	setupAuditAPI(t)
	sess := sessaoReal(t, "admin-bancos-motor", auth.RoleAdmin)
	srv := servidorDeRename(t, "qa-bancos-motor", "203.0.113.127", nil)

	inst := instanciaDeBanco(t, srv.ID, 5435)
	achada := acharInstancia(t, lerBancos(t, sess, ""), inst.ID)

	if achada.Motor != "postgres" {
		t.Errorf("motor = %q, esperado postgres: a chave motor tem de vir em cada instância", achada.Motor)
	}
}

func TestBancosTamanhoTotalNuloQuandoNenhumaBaseFoiMedida(t *testing.T) {
	setupAuditAPI(t)
	sess := sessaoReal(t, "admin-bancos-sem-medida", auth.RoleAdmin)
	srv := servidorDeRename(t, "qa-bancos-sem-medida", "203.0.113.121", nil)

	inst := instanciaDeBanco(t, srv.ID, 5433)
	baseDeBanco(t, inst.ID, "uma", nil)
	baseDeBanco(t, inst.ID, "outra", nil)

	achada := acharInstancia(t, lerBancos(t, sess, ""), inst.ID)

	if achada.TamanhoTotalBytes != nil {
		t.Errorf("tamanho_total_bytes = %d, esperado null: nenhuma base foi medida",
			*achada.TamanhoTotalBytes)
	}
	if achada.TotalBases != 2 {
		t.Errorf("total_bases = %d, esperado 2 mesmo sem tamanho medido", achada.TotalBases)
	}
}

func TestBancosInstanciaSemBaseVemComListaVazia(t *testing.T) {
	setupAuditAPI(t)
	sess := sessaoReal(t, "admin-bancos-sem-base", auth.RoleAdmin)
	srv := servidorDeRename(t, "qa-bancos-sem-base", "203.0.113.122", nil)

	inst := instanciaDeBanco(t, srv.ID, 5434)
	achada := acharInstancia(t, lerBancos(t, sess, ""), inst.ID)

	if achada.Bases == nil {
		t.Errorf("bases veio null, esperado lista vazia")
	}
	if len(achada.Bases) != 0 {
		t.Errorf("bases = %v, esperado vazio", achada.Bases)
	}
	if achada.TotalBases != 0 {
		t.Errorf("total_bases = %d, esperado 0", achada.TotalBases)
	}
	if achada.TamanhoTotalBytes != nil {
		t.Errorf("tamanho_total_bytes = %d, esperado null", *achada.TamanhoTotalBytes)
	}
}

func TestBancosOrdenaPorServidorEDepoisPorPorta(t *testing.T) {
	setupAuditAPI(t)
	sess := sessaoReal(t, "admin-bancos-ordem", auth.RoleAdmin)
	primeiro := servidorDeRename(t, "qa-bancos-aa", "203.0.113.123", nil)
	segundo := servidorDeRename(t, "qa-bancos-zz", "203.0.113.124", nil)

	instanciaDeBanco(t, primeiro.ID, 5433)
	instanciaDeBanco(t, primeiro.ID, 5432)
	instanciaDeBanco(t, segundo.ID, 5432)

	lista := somenteDosServidores(lerBancos(t, sess, ""), primeiro.ID, segundo.ID)
	if len(lista) != 3 {
		t.Fatalf("a listagem trouxe %d instâncias dos servidores do teste, esperado 3", len(lista))
	}

	esperado := []struct {
		nome  string
		porta int
	}{
		{"qa-bancos-aa", 5432},
		{"qa-bancos-aa", 5433},
		{"qa-bancos-zz", 5432},
	}
	for i, quer := range esperado {
		if lista[i].ServidorNome != quer.nome || lista[i].Porta != quer.porta {
			t.Errorf("posição %d = %s:%d, esperado %s:%d",
				i, lista[i].ServidorNome, lista[i].Porta, quer.nome, quer.porta)
		}
	}
}

func TestBancosEscopoNaoVazaInstanciaDeOutraUnidade(t *testing.T) {
	setupAuditAPI(t)
	sedeA := unidadeDeRename(t, "qa-bancos-a")
	sedeB := unidadeDeRename(t, "qa-bancos-b")

	srvA := servidorDeRename(t, "qa-bancos-da-a", "203.0.113.125", &sedeA)
	srvB := servidorDeRename(t, "qa-bancos-da-b", "203.0.113.126", &sedeB)
	daA := instanciaDeBanco(t, srvA.ID, 5432)
	daB := instanciaDeBanco(t, srvB.ID, 5432)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/bancos", nil)
	bancosHandler(rec, comSessaoDaUnidade(req, sedeA))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, esperado 200: %s", rec.Code, rec.Body.String())
	}

	var viuA bool
	for _, inst := range decodificarBancos(t, rec) {
		if inst.ID == daB.ID {
			t.Errorf("a sessão da unidade A enxergou a instância %d, que é do servidor da unidade B", daB.ID)
		}
		if inst.ID != daA.ID {
			continue
		}
		viuA = true
		if inst.SiteID == nil || *inst.SiteID != sedeA {
			t.Errorf("site_id = %v, esperado %d vindo de servers.site_id", inst.SiteID, sedeA)
		}
	}
	if !viuA {
		t.Fatalf("a instância da própria unidade não apareceu; o teste não mediu nada")
	}
}

func TestBancosSiteIDInvalidoDevolveErroEmPortugues(t *testing.T) {
	setupAuditAPI(t)
	sess := sessaoReal(t, "admin-bancos-site-invalido", auth.RoleAdmin)

	rec := pedirComSessao(t, http.MethodGet, "/api/bancos?site_id=abacaxi", "", sess)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("site_id inválido: status %d, esperado 400 (%s)", rec.Code, rec.Body.String())
	}

	var corpo map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &corpo); err != nil {
		t.Fatalf("resposta de erro não é JSON: %v", err)
	}
	if corpo["error"] != "site_id inválido ou fora do seu alcance" {
		t.Errorf("error = %q, esperado a mensagem em pt-BR do contrato", corpo["error"])
	}
}

func TestBancosSiteIDForaDoAlcanceDevolve403(t *testing.T) {
	setupAuditAPI(t)
	sedeA := unidadeDeRename(t, "qa-bancos-alcance-a")
	sedeB := unidadeDeRename(t, "qa-bancos-alcance-b")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/bancos?site_id="+strconv.FormatUint(uint64(sedeB), 10), nil)
	bancosHandler(rec, comSessaoDaUnidade(req, sedeA))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("site_id de outra unidade: status %d, esperado 403 (%s)", rec.Code, rec.Body.String())
	}

	var corpo map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &corpo); err != nil {
		t.Fatalf("resposta de erro não é JSON: %v", err)
	}
	if corpo["error"] != "site_id inválido ou fora do seu alcance" {
		t.Errorf("error = %q, esperado a mensagem em pt-BR do contrato", corpo["error"])
	}
}

func TestBancosViewerConsegueLer(t *testing.T) {
	setupAuditAPI(t)
	sess := sessaoReal(t, "viewer-bancos", auth.RoleViewer)

	rec := pedirComSessao(t, http.MethodGet, "/api/bancos", "", sess)
	if rec.Code != http.StatusOK {
		t.Fatalf("viewer em GET /api/bancos: status %d, esperado 200 (%s)", rec.Code, rec.Body.String())
	}
	decodificarBancos(t, rec)
}

func TestBancosRecusaMetodoDiferenteDeGet(t *testing.T) {
	setupAuditAPI(t)
	sess := sessaoReal(t, "admin-bancos-metodo", auth.RoleAdmin)

	for _, metodo := range []string{http.MethodPost, http.MethodPatch, http.MethodDelete} {
		rec := pedirComSessao(t, metodo, "/api/bancos", "{}", sess)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s /api/bancos: status %d, esperado 405", metodo, rec.Code)
		}
	}
}

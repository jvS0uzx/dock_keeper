package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/database"
	"github.com/jvS0uzx/dock_keeper/internal/ssh"
)

const (
	srvDoEsquema      = "00000000-0000-0000-0000-0000000e5001"
	srvDoEsquemaOutro = "00000000-0000-0000-0000-0000000e5002"
)

type cenarioDeEsquema struct {
	sedeA          uint
	instancia      uint
	daOutraUnidade uint
}

func tabelaBruta(nome string) ssh.EsquemaTabelaBruta {
	return ssh.EsquemaTabelaBruta{Schema: "public", Nome: nome}
}

func relacaoBruta(nome, de, para string) ssh.EsquemaRelacaoBruta {
	return ssh.EsquemaRelacaoBruta{
		Nome:        nome,
		DeSchema:    "public",
		DeTabela:    de,
		DeColunas:   []string{para + "_id"},
		ParaSchema:  "public",
		ParaTabela:  para,
		ParaColunas: []string{"id"},
		AoApagar:    "CASCADE",
	}
}

func nomesDasTabelas(view EsquemaView) []string {
	nomes := make([]string, 0, len(view.Tabelas))
	for _, t := range view.Tabelas {
		nomes = append(nomes, t.Nome)
	}
	return nomes
}

func contem(nomes []string, alvo string) bool {
	for _, n := range nomes {
		if n == alvo {
			return true
		}
	}
	return false
}

func TestIdDaRotaDeEsquemaSoAceitaOCaminhoDoEsquema(t *testing.T) {
	validos := map[string]uint{
		"/api/bancos/1/esquema":   1,
		"/api/bancos/42/esquema":  42,
		"/api/bancos/42/esquema/": 42,
	}
	for caminho, esperado := range validos {
		id, ok := idDaRotaDeEsquema(caminho)
		if !ok || id != esperado {
			t.Errorf("idDaRotaDeEsquema(%q) = %d, %v; esperado %d, true", caminho, id, ok, esperado)
		}
	}

	invalidos := []string{
		"/api/bancos",
		"/api/bancos/",
		"/api/bancos/1",
		"/api/bancos/1/bases",
		"/api/bancos/1/esquema/extra",
		"/api/bancos/abc/esquema",
		"/api/bancos/0/esquema",
		"/api/bancos/-1/esquema",
	}
	for _, caminho := range invalidos {
		if _, ok := idDaRotaDeEsquema(caminho); ok {
			t.Errorf("idDaRotaDeEsquema(%q) aceitou um caminho que não é o do esquema", caminho)
		}
	}
}

func TestMontarEsquemaTruncaPelasMaisConectadas(t *testing.T) {
	p := ssh.EsquemaPayload{Ok: true}
	for i := 1; i <= 61; i++ {
		p.Tabelas = append(p.Tabelas, tabelaBruta(fmt.Sprintf("t%02d", i)))
	}
	for i := 2; i <= 40; i++ {
		p.Relacoes = append(p.Relacoes, relacaoBruta(fmt.Sprintf("fk%02d", i), fmt.Sprintf("t%02d", i), "t01"))
	}

	view := montarEsquema(p, tetoDeTabelasNoEsquema)

	if !view.Truncado {
		t.Fatal("truncado = false com 61 tabelas e teto de 60")
	}
	if view.TotalTabelas != 61 {
		t.Errorf("total_tabelas = %d, esperado 61: o número real não pode virar o número desenhado", view.TotalTabelas)
	}
	if len(view.Tabelas) != 60 {
		t.Fatalf("tabelas desenhadas = %d, esperado 60", len(view.Tabelas))
	}

	nomes := nomesDasTabelas(view)
	if !contem(nomes, "t01") {
		t.Error("a tabela mais conectada ficou de fora do corte")
	}
	if contem(nomes, "t61") {
		t.Error("a tabela sem nenhuma relação sobreviveu ao corte no lugar de uma conectada")
	}
}

func TestMontarEsquemaNaoDeixaArestaApontandoParaTabelaCortada(t *testing.T) {
	p := ssh.EsquemaPayload{
		Ok:      true,
		Tabelas: []ssh.EsquemaTabelaBruta{tabelaBruta("a"), tabelaBruta("b"), tabelaBruta("c")},
		Relacoes: []ssh.EsquemaRelacaoBruta{
			relacaoBruta("fk_a_b", "a", "b"),
			relacaoBruta("fk_c_a", "c", "a"),
		},
	}

	view := montarEsquema(p, 2)

	if !view.Truncado || view.TotalTabelas != 3 || len(view.Tabelas) != 2 {
		t.Fatalf("view = %+v, esperadas 2 de 3 tabelas com truncado", view)
	}
	if len(view.Relacoes) != 1 || view.Relacoes[0].Nome != "fk_a_b" {
		t.Fatalf("relações = %+v, esperada só a que liga duas tabelas desenhadas", view.Relacoes)
	}
}

func TestMontarEsquemaLimitaColunasAsChavesMaisOito(t *testing.T) {
	p := ssh.EsquemaPayload{Ok: true, Tabelas: []ssh.EsquemaTabelaBruta{tabelaBruta("pedido")}}
	p.Colunas = append(p.Colunas, ssh.EsquemaColunaBruta{
		Schema: "public", Tabela: "pedido", Nome: "id", Tipo: "bigint", ChavePrimaria: true,
	})
	for i := 1; i <= 12; i++ {
		p.Colunas = append(p.Colunas, ssh.EsquemaColunaBruta{
			Schema: "public", Tabela: "pedido", Nome: fmt.Sprintf("campo%02d", i), Tipo: "text", Nulo: true,
		})
	}
	p.Colunas = append(p.Colunas,
		ssh.EsquemaColunaBruta{Schema: "public", Tabela: "pedido", Nome: "cliente_id", Tipo: "bigint", ChaveEstrangeira: true},
		ssh.EsquemaColunaBruta{Schema: "public", Tabela: "pedido", Nome: "loja_id", Tipo: "bigint", ChaveEstrangeira: true},
	)

	view := montarEsquema(p, tetoDeTabelasNoEsquema)

	tabela := view.Tabelas[0]
	if len(tabela.Colunas) != 11 {
		t.Fatalf("colunas = %d, esperado 11: chave primária, duas estrangeiras e oito comuns", len(tabela.Colunas))
	}
	if len(tabela.ColunasChave) != 1 || tabela.ColunasChave[0] != "id" {
		t.Errorf("colunas_chave = %v, esperado [id]", tabela.ColunasChave)
	}
	if tabela.Colunas[0].Nome != "id" || tabela.Colunas[1].Nome != "campo01" {
		t.Errorf("colunas = %+v, esperada a ordem do catálogo", tabela.Colunas)
	}

	var chaves int
	for _, c := range tabela.Colunas {
		if c.Nome == "cliente_id" || c.Nome == "loja_id" {
			chaves++
		}
	}
	if chaves != 2 {
		t.Errorf("as chaves estrangeiras caíram no corte das oito comuns: %+v", tabela.Colunas)
	}
}

func TestMontarEsquemaPreservaNuloEAutoRelacionamento(t *testing.T) {
	linhas := int64(7)
	p := ssh.EsquemaPayload{
		Ok: true,
		Tabelas: []ssh.EsquemaTabelaBruta{
			{Schema: "public", Nome: "categorias", LinhasEstimadas: &linhas},
			{Schema: "extra", Nome: "solta"},
		},
		Relacoes: []ssh.EsquemaRelacaoBruta{
			relacaoBruta("fk_categorias_pai", "categorias", "categorias"),
		},
	}

	view := montarEsquema(p, tetoDeTabelasNoEsquema)

	if view.Truncado || view.TotalTabelas != 2 {
		t.Fatalf("view = %+v, esperado sem truncamento", view)
	}
	if len(view.Schemas) != 2 || view.Schemas[0] != "extra" || view.Schemas[1] != "public" {
		t.Errorf("schemas = %v, esperado [extra public] em ordem estável", view.Schemas)
	}
	if view.Tabelas[0].Schema != "extra" {
		t.Errorf("tabelas = %+v, esperada ordem estável por schema e nome", nomesDasTabelas(view))
	}

	var categorias EsquemaTabelaView
	for _, tab := range view.Tabelas {
		if tab.Nome == "categorias" {
			categorias = tab
		}
	}
	if categorias.LinhasEstimadas == nil || *categorias.LinhasEstimadas != 7 {
		t.Errorf("linhas_estimadas = %v, esperado 7", categorias.LinhasEstimadas)
	}
	if categorias.TamanhoBytes != nil {
		t.Errorf("tamanho_bytes = %v, esperado nulo: a coleta não mediu", *categorias.TamanhoBytes)
	}
	if len(categorias.Colunas) != 0 || len(categorias.ColunasChave) != 0 {
		t.Errorf("tabela sem colunas coletadas = %+v, esperadas listas vazias", categorias)
	}

	if len(view.Relacoes) != 1 || view.Relacoes[0].DeTabela != view.Relacoes[0].ParaTabela {
		t.Fatalf("relações = %+v, esperado o auto-relacionamento preservado", view.Relacoes)
	}
}

func comColetorDeEsquema(t *testing.T, fn func(context.Context, ssh.Target, ssh.AlvoDeEsquema) (ssh.EsquemaPayload, error)) *int {
	t.Helper()
	chamadas := 0
	anterior := coletarEsquema
	coletarEsquema = func(ctx context.Context, alvo ssh.Target, pedido ssh.AlvoDeEsquema) (ssh.EsquemaPayload, error) {
		chamadas++
		return fn(ctx, alvo, pedido)
	}
	t.Cleanup(func() { coletarEsquema = anterior })
	return &chamadas
}

func coletorQueNaoDeveriaRodar(t *testing.T) *int {
	t.Helper()
	return comColetorDeEsquema(t, func(context.Context, ssh.Target, ssh.AlvoDeEsquema) (ssh.EsquemaPayload, error) {
		t.Error("a coleta abriu sessão SSH quando não deveria")
		return ssh.EsquemaPayload{}, nil
	})
}

func esquemaDeExemplo() ssh.EsquemaPayload {
	return ssh.EsquemaPayload{
		Ok: true,
		Tabelas: []ssh.EsquemaTabelaBruta{
			tabelaBruta("servers"),
			tabelaBruta("server_addresses"),
		},
		Colunas: []ssh.EsquemaColunaBruta{
			{Schema: "public", Tabela: "servers", Nome: "id", Tipo: "uuid", ChavePrimaria: true},
			{Schema: "public", Tabela: "server_addresses", Nome: "server_id", Tipo: "uuid", ChaveEstrangeira: true},
		},
		Relacoes: []ssh.EsquemaRelacaoBruta{relacaoBruta("fk_enderecos", "server_addresses", "servers")},
	}
}

func montarCenarioDeEsquema(t *testing.T, estado string) cenarioDeEsquema {
	t.Helper()

	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("DATABASE_URL não definido; pulando teste de esquema com banco")
	}
	if database.DB == nil {
		if err := database.Connect(); err != nil {
			t.Skipf("banco indisponível: %v", err)
		}
	}

	limparCenarioDeEsquema(t)
	t.Cleanup(func() { limparCenarioDeEsquema(t) })

	sedes := []database.Site{
		{Name: "qa-esq-a", Code: "qa-esq-a"},
		{Name: "qa-esq-b", Code: "qa-esq-b"},
	}
	if err := database.DB.Create(&sedes).Error; err != nil {
		t.Fatalf("criar unidades: %v", err)
	}

	servidores := []database.Server{
		{ID: srvDoEsquema, Name: "qa-esq-servidor-a", HostIP: "203.0.113.41", Kind: "ssh", SiteID: &sedes[0].ID},
		{ID: srvDoEsquemaOutro, Name: "qa-esq-servidor-b", HostIP: "203.0.113.42", Kind: "ssh", SiteID: &sedes[1].ID},
	}
	if err := database.DB.Create(&servidores).Error; err != nil {
		t.Fatalf("criar servidores: %v", err)
	}

	agora := time.Now().UTC()
	instancias := []database.PostgresInstancia{
		{ServerID: srvDoEsquema, Porta: 5432, Estado: estado, Motivo: "senha recusada",
			Papel: "primario", ObservadoEm: agora},
		{ServerID: srvDoEsquemaOutro, Porta: 5432, Estado: "ativo", Papel: "primario", ObservadoEm: agora},
	}
	if err := database.DB.Create(&instancias).Error; err != nil {
		t.Fatalf("criar instâncias: %v", err)
	}

	bases := []database.PostgresBase{
		{InstanciaID: instancias[0].ID, Nome: "app", ObservadoEm: agora},
		{InstanciaID: instancias[0].ID, Nome: "dockkeeper", ObservadoEm: agora},
		{InstanciaID: instancias[1].ID, Nome: "app", ObservadoEm: agora},
	}
	if err := database.DB.Create(&bases).Error; err != nil {
		t.Fatalf("criar bases: %v", err)
	}

	return cenarioDeEsquema{
		sedeA:          sedes[0].ID,
		instancia:      instancias[0].ID,
		daOutraUnidade: instancias[1].ID,
	}
}

func limparCenarioDeEsquema(t *testing.T) {
	t.Helper()
	ids := []string{srvDoEsquema, srvDoEsquemaOutro}
	database.DB.Where("server_id IN ?", ids).Delete(&database.PostgresInstancia{})
	database.DB.Unscoped().Where("id IN ?", ids).Delete(&database.Server{})
	database.DB.Where("code IN ?", []string{"qa-esq-a", "qa-esq-b"}).Delete(&database.Site{})
}

func pedirEsquema(t *testing.T, cenario cenarioDeEsquema, id uint, consulta string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/api/bancos/%d/esquema?%s", id, consulta), nil)
	req = comSessaoDaUnidade(req, cenario.sedeA)

	Config{}.esquemaDaBaseHandler(rec, req)
	return rec
}

func TestEsquemaDaBaseEntregaOEsquemaMontado(t *testing.T) {
	cenario := montarCenarioDeEsquema(t, "ativo")
	var pedido ssh.AlvoDeEsquema
	comColetorDeEsquema(t, func(_ context.Context, _ ssh.Target, p ssh.AlvoDeEsquema) (ssh.EsquemaPayload, error) {
		pedido = p
		return esquemaDeExemplo(), nil
	})

	rec := pedirEsquema(t, cenario, cenario.instancia, "base=app")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, esperado 200: %s", rec.Code, rec.Body.String())
	}

	var view EsquemaView
	if err := json.NewDecoder(rec.Body).Decode(&view); err != nil {
		t.Fatalf("resposta não é JSON: %v", err)
	}
	if view.InstanciaID != cenario.instancia || view.Base != "app" {
		t.Errorf("view = %+v, esperada a instância e a base pedidas", view)
	}
	if view.Motor != "postgres" || !view.SuportaDiagrama {
		t.Errorf("motor = %q suporta_diagrama = %v, esperado postgres com diagrama", view.Motor, view.SuportaDiagrama)
	}
	if view.ColetadoEm.IsZero() {
		t.Error("coletado_em vazio: a coleta é sob demanda e precisa datar a leitura")
	}
	if len(view.Tabelas) != 2 || len(view.Relacoes) != 1 || view.TotalTabelas != 2 {
		t.Errorf("view = %+v, esperadas 2 tabelas e 1 relação", view)
	}
	if pedido.Base != "app" || pedido.Porta != 5432 {
		t.Errorf("pedido à sonda = %+v, esperada a base app na porta da instância", pedido)
	}
}

func TestEsquemaDaBaseRecusaBaseForaDoInventarioSemConectar(t *testing.T) {
	cenario := montarCenarioDeEsquema(t, "ativo")
	chamadas := coletorQueNaoDeveriaRodar(t)

	rec := pedirEsquema(t, cenario, cenario.instancia, "base=nao_existe")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, esperado 404: %s", rec.Code, rec.Body.String())
	}
	if *chamadas != 0 {
		t.Errorf("a sonda rodou %d vez(es) para uma base que não existe na instância", *chamadas)
	}
}

func TestEsquemaDaBaseCom409NaoTentaConectar(t *testing.T) {
	cenario := montarCenarioDeEsquema(t, "sem_acesso")
	chamadas := coletorQueNaoDeveriaRodar(t)

	rec := pedirEsquema(t, cenario, cenario.instancia, "base=app")
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, esperado 409: %s", rec.Code, rec.Body.String())
	}
	if *chamadas != 0 {
		t.Errorf("a sonda rodou %d vez(es) numa instância fora de ativo", *chamadas)
	}
	if corpo := rec.Body.String(); !strings.Contains(corpo, "senha recusada") {
		t.Errorf("corpo = %s, esperado repetir o motivo já conhecido", corpo)
	}
}

func TestEsquemaDaBaseCom502QuandoOCatalogoRecusa(t *testing.T) {
	cenario := montarCenarioDeEsquema(t, "ativo")
	comColetorDeEsquema(t, func(context.Context, ssh.Target, ssh.AlvoDeEsquema) (ssh.EsquemaPayload, error) {
		return ssh.EsquemaPayload{
			Ok:     false,
			Classe: ssh.FalhaSemPermissao,
			Erro:   "permission denied for table pg_class",
		}, nil
	})

	rec := pedirEsquema(t, cenario, cenario.instancia, "base=app")
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, esperado 502: falha de leitura não pode virar esquema vazio com 200: %s",
			rec.Code, rec.Body.String())
	}
	if corpo := rec.Body.String(); !strings.Contains(corpo, "permission denied") {
		t.Errorf("corpo = %s, esperado carregar o detalhe da recusa", corpo)
	}
}

func TestEsquemaDaBaseCom502QuandoASessaoFalha(t *testing.T) {
	cenario := montarCenarioDeEsquema(t, "ativo")
	comColetorDeEsquema(t, func(context.Context, ssh.Target, ssh.AlvoDeEsquema) (ssh.EsquemaPayload, error) {
		return ssh.EsquemaPayload{}, fmt.Errorf("conexão recusada")
	})

	rec := pedirEsquema(t, cenario, cenario.instancia, "base=app")
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, esperado 502: %s", rec.Code, rec.Body.String())
	}
}

func TestEsquemaDaBaseCom404ParaBaseInexistenteNaSonda(t *testing.T) {
	cenario := montarCenarioDeEsquema(t, "ativo")
	comColetorDeEsquema(t, func(context.Context, ssh.Target, ssh.AlvoDeEsquema) (ssh.EsquemaPayload, error) {
		return ssh.EsquemaPayload{
			Ok:     false,
			Classe: ssh.FalhaBaseInexistente,
			Erro:   "database app does not exist",
		}, nil
	})

	rec := pedirEsquema(t, cenario, cenario.instancia, "base=app")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, esperado 404: a base sumiu entre a coleta e o clique: %s",
			rec.Code, rec.Body.String())
	}
}

func TestEsquemaDaBaseNegaInstanciaDeOutraUnidade(t *testing.T) {
	cenario := montarCenarioDeEsquema(t, "ativo")
	chamadas := coletorQueNaoDeveriaRodar(t)

	rec := pedirEsquema(t, cenario, cenario.daOutraUnidade, "base=app")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, esperado 404 para instância de outra unidade: %s", rec.Code, rec.Body.String())
	}
	if *chamadas != 0 {
		t.Errorf("a sonda rodou %d vez(es) numa instância fora do escopo do usuário", *chamadas)
	}
}

func TestEsquemaDaBaseRecusaNomeDeBaseHostil(t *testing.T) {
	cenario := montarCenarioDeEsquema(t, "ativo")
	chamadas := coletorQueNaoDeveriaRodar(t)

	for _, consulta := range []string{"base=app;%20id", "base=app%20base", "base=../etc", "base="} {
		rec := pedirEsquema(t, cenario, cenario.instancia, consulta)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, esperado 400: %s", consulta, rec.Code, rec.Body.String())
		}
	}
	if *chamadas != 0 {
		t.Errorf("a sonda rodou %d vez(es) com nome de base recusado", *chamadas)
	}
}

func TestEsquemaDeMySQLRespondeSemDiagramaSemConectar(t *testing.T) {
	for _, caso := range []struct {
		motor, estado, nome string
	}{
		{"mysql", "ativo", "MySQL"},
		{"mariadb", "ativo", "MariaDB"},
		{"mysql", "sem_acesso", "MySQL"},
	} {
		t.Run(caso.motor+"/"+caso.estado, func(t *testing.T) {
			cenario := montarCenarioDeEsquema(t, caso.estado)
			chamadas := coletorQueNaoDeveriaRodar(t)
			if err := database.DB.Model(&database.PostgresInstancia{}).Where("id = ?", cenario.instancia).
				Update("motor", caso.motor).Error; err != nil {
				t.Fatalf("trocar o motor da instância: %v", err)
			}

			rec := pedirEsquema(t, cenario, cenario.instancia, "base=app")
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, esperado 200: %s", rec.Code, rec.Body.String())
			}
			if *chamadas != 0 {
				t.Errorf("o probe_schema.sh rodou %d vez(es) para %s", *chamadas, caso.motor)
			}

			var view EsquemaView
			if err := json.NewDecoder(rec.Body).Decode(&view); err != nil {
				t.Fatalf("resposta não é JSON: %v", err)
			}
			if view.SuportaDiagrama || view.Motor != caso.motor {
				t.Errorf("motor = %q suporta_diagrama = %v, esperado %s sem diagrama", view.Motor, view.SuportaDiagrama, caso.motor)
			}
			esperado := "O diagrama ainda não está disponível para " + caso.nome + "."
			if view.Motivo != esperado {
				t.Errorf("motivo = %q, esperado %q", view.Motivo, esperado)
			}
			if view.Tabelas == nil || len(view.Tabelas) != 0 || view.Relacoes == nil {
				t.Errorf("tabelas = %v relacoes = %v, esperado listas vazias, não nulas", view.Tabelas, view.Relacoes)
			}
		})
	}
}

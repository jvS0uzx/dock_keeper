package ssh

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
)

const jsonDoEsquema = `{"ok":true,"classe":"","erro":"","tabelas":[` +
	`{"schema":"public","nome":"servers","linhas_estimadas":4,"tamanho_bytes":81920},` +
	`{"schema":"public","nome":"solta"}],` +
	`"colunas":[{"schema":"public","tabela":"servers","nome":"id","tipo":"uuid","nulo":false,"chave_primaria":true,"chave_estrangeira":false}],` +
	`"relacoes":[{"nome":"fk_enderecos","de_schema":"public","de_tabela":"server_addresses","de_colunas":["server_id"],` +
	`"para_schema":"public","para_tabela":"servers","para_colunas":["id"],"ao_apagar":"c"}]}`

func alvoDeEsquemaPadrao() AlvoDeEsquema {
	return AlvoDeEsquema{Base: "dockkeeper", Porta: 5432}
}

func TestColetarEsquemaIgnoraRuidoAntesDoJSON(t *testing.T) {
	alvo := alvoComServidor(t, func(string) respostaExec {
		return respostaExec{
			stdout:       "sudo: unable to resolve host\nbash: linha ignorada\n" + jsonDoEsquema + "\n",
			consomeStdin: true,
		}
	})

	p, err := ColetarEsquema(context.Background(), alvo, alvoDeEsquemaPadrao())
	if err != nil {
		t.Fatalf("coleta falhou: %v", err)
	}
	if !p.Ok {
		t.Fatalf("payload = %+v, esperado ok", p)
	}
	if len(p.Tabelas) != 2 || p.Tabelas[0].Nome != "servers" {
		t.Fatalf("tabelas = %+v, esperadas servers e solta", p.Tabelas)
	}
	if len(p.Relacoes) != 1 || p.Relacoes[0].AoApagar != "CASCADE" {
		t.Fatalf("relações = %+v, esperado confdeltype c traduzido para CASCADE", p.Relacoes)
	}
	if len(p.Colunas) != 1 || !p.Colunas[0].ChavePrimaria {
		t.Fatalf("colunas = %+v, esperada a chave primária de servers", p.Colunas)
	}
}

func TestColetarEsquemaMantemNuloComoNulo(t *testing.T) {
	alvo := alvoComServidor(t, func(string) respostaExec {
		return respostaExec{stdout: jsonDoEsquema + "\n", consomeStdin: true}
	})

	p, err := ColetarEsquema(context.Background(), alvo, alvoDeEsquemaPadrao())
	if err != nil {
		t.Fatalf("coleta falhou: %v", err)
	}

	medida := p.Tabelas[0]
	if medida.LinhasEstimadas == nil || *medida.LinhasEstimadas != 4 {
		t.Errorf("linhas_estimadas = %v, esperado 4", medida.LinhasEstimadas)
	}
	if medida.TamanhoBytes == nil || *medida.TamanhoBytes != 81920 {
		t.Errorf("tamanho_bytes = %v, esperado 81920", medida.TamanhoBytes)
	}

	semMedida := p.Tabelas[1]
	if semMedida.LinhasEstimadas != nil || semMedida.TamanhoBytes != nil {
		t.Errorf("tabela sem medida = %+v, esperado ponteiro nulo, nunca 0", semMedida)
	}
}

func TestColetarEsquemaDevolveFalhaDaSondaSemInventarEsquema(t *testing.T) {
	alvo := alvoComServidor(t, func(string) respostaExec {
		return respostaExec{
			stdout:       `{"ok":false,"classe":"base_inexistente","erro":"database app does not exist","tabelas":[],"colunas":[],"relacoes":[]}` + "\n",
			consomeStdin: true,
		}
	})

	p, err := ColetarEsquema(context.Background(), alvo, alvoDeEsquemaPadrao())
	if err != nil {
		t.Fatalf("coleta falhou: %v", err)
	}
	if p.Ok {
		t.Fatal("ok = true num payload de falha")
	}
	if p.Classe != FalhaBaseInexistente {
		t.Errorf("classe = %q, esperado %q", p.Classe, FalhaBaseInexistente)
	}
	if len(p.Tabelas) != 0 {
		t.Errorf("tabelas = %+v, esperado vazio", p.Tabelas)
	}
}

func TestColetarEsquemaSemJSONExplicaEmPortugues(t *testing.T) {
	alvo := alvoComServidor(t, func(string) respostaExec {
		return respostaExec{stdout: "bash: psql: comando não encontrado\n", consomeStdin: true}
	})

	_, err := ColetarEsquema(context.Background(), alvo, alvoDeEsquemaPadrao())
	if err == nil {
		t.Fatal("saída sem JSON passou como coleta bem-sucedida")
	}
	if !strings.Contains(err.Error(), "não devolveu JSON") {
		t.Errorf("erro = %q, esperado explicar em pt-BR", err.Error())
	}
}

func TestColetarEsquemaRecusaBaseHostilAntesDeAbrirSessao(t *testing.T) {
	var mu sync.Mutex
	executou := false
	alvo := alvoComServidor(t, func(string) respostaExec {
		mu.Lock()
		executou = true
		mu.Unlock()
		return respostaExec{stdout: jsonDoEsquema + "\n", consomeStdin: true}
	})

	hostis := []string{
		"app; touch /tmp/dockkeeper-invadiu",
		"app`id`",
		"app'--",
		"app base",
		"",
		"../../etc/passwd",
		strings.Repeat("b", 64),
	}

	for _, base := range hostis {
		t.Run(base, func(t *testing.T) {
			_, err := ColetarEsquema(context.Background(), alvo, AlvoDeEsquema{Base: base, Porta: 5432})
			if !errors.Is(err, ErrBaseInvalida) {
				t.Fatalf("erro = %v, esperado ErrBaseInvalida", err)
			}
		})
	}

	mu.Lock()
	defer mu.Unlock()
	if executou {
		t.Fatal("o servidor recebeu comando com nome de base recusado; a validação precisa vir antes da sessão")
	}
}

func TestColetarEsquemaRecusaContainerInvalido(t *testing.T) {
	var mu sync.Mutex
	executou := false
	alvo := alvoComServidor(t, func(string) respostaExec {
		mu.Lock()
		executou = true
		mu.Unlock()
		return respostaExec{stdout: jsonDoEsquema + "\n", consomeStdin: true}
	})

	_, err := ColetarEsquema(context.Background(), alvo, AlvoDeEsquema{
		Base:          "app",
		Porta:         5432,
		EmContainer:   true,
		ContainerNome: "pg app; id",
	})
	if !errors.Is(err, ErrContainerInvalido) {
		t.Fatalf("erro = %v, esperado ErrContainerInvalido", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if executou {
		t.Fatal("o servidor recebeu comando com nome de container recusado")
	}
}

func TestNomeDeBaseValidoAceitaOQueOPostgresAceita(t *testing.T) {
	validos := []string{"app", "dock_keeper", "app-01", "app.qa", "A1", strings.Repeat("b", 63)}
	for _, nome := range validos {
		if !NomeDeBaseValido(nome) {
			t.Errorf("nome %q recusado, esperado aceito", nome)
		}
	}

	invalidos := []string{"", " app", "app ", "-app", ".app", "app;id", "app/x", "app\n", strings.Repeat("b", 64)}
	for _, nome := range invalidos {
		if NomeDeBaseValido(nome) {
			t.Errorf("nome %q aceito, esperado recusado", nome)
		}
	}
}

func TestTraduzirAoApagarCobreOCatalogo(t *testing.T) {
	casos := map[string]string{
		"c": "CASCADE",
		"r": "RESTRICT",
		"n": "SET NULL",
		"d": "SET DEFAULT",
		"a": "NO ACTION",
		"":  "",
		"x": "",
	}

	for bruto, esperado := range casos {
		if got := traduzirAoApagar(bruto); got != esperado {
			t.Errorf("traduzirAoApagar(%q) = %q, esperado %q", bruto, got, esperado)
		}
	}
}

func TestPreludioDeEsquemaNaoVazaContainerDeInstanciaDeHost(t *testing.T) {
	deHost := preludioDeEsquema(AlvoDeEsquema{Base: "app", Porta: 0, ContainerNome: "pg-app"})
	if !strings.Contains(deHost, "DOCKKEEPER_ESQUEMA_CONTAINER=\"\"\n") {
		t.Errorf("prelúdio = %q, esperado container vazio: a instância é de host", deHost)
	}
	if !strings.Contains(deHost, "DOCKKEEPER_ESQUEMA_PORTA=5432\n") {
		t.Errorf("prelúdio = %q, esperada a porta padrão quando não há porta", deHost)
	}

	emContainer := preludioDeEsquema(AlvoDeEsquema{
		Base: "app", Porta: 5433, EmContainer: true, ContainerNome: "pg-app",
	})
	if !strings.Contains(emContainer, "DOCKKEEPER_ESQUEMA_CONTAINER=\"pg-app\"\n") {
		t.Errorf("prelúdio = %q, esperado o container da instância", emContainer)
	}
	if !strings.Contains(emContainer, "DOCKKEEPER_ESQUEMA_BASE=\"app\"\n") {
		t.Errorf("prelúdio = %q, esperada a base", emContainer)
	}
}

func TestNormalizarEsquemaDescartaLinhaSemIdentidade(t *testing.T) {
	limpo := normalizarEsquema(EsquemaPayload{
		Ok: true,
		Tabelas: []EsquemaTabelaBruta{
			{Schema: "public", Nome: " servers "},
			{Schema: "public", Nome: "   "},
			{Schema: "", Nome: "sem_schema"},
		},
		Colunas: []EsquemaColunaBruta{
			{Schema: "public", Tabela: "servers", Nome: "id", Tipo: "uuid"},
			{Schema: "public", Tabela: "servers", Nome: ""},
		},
		Relacoes: []EsquemaRelacaoBruta{
			{Nome: "fk", DeSchema: "public", DeTabela: "a", DeColunas: []string{"b_id"},
				ParaSchema: "public", ParaTabela: "b", ParaColunas: []string{"id"}, AoApagar: "c"},
			{Nome: "fk_sem_coluna", DeSchema: "public", DeTabela: "a",
				ParaSchema: "public", ParaTabela: "b", ParaColunas: []string{"id"}},
		},
	})

	if len(limpo.Tabelas) != 1 || limpo.Tabelas[0].Nome != "servers" {
		t.Fatalf("tabelas = %+v, esperada só servers sem espaço em volta", limpo.Tabelas)
	}
	if len(limpo.Colunas) != 1 {
		t.Fatalf("colunas = %+v, esperada 1: coluna sem nome não desenha nada", limpo.Colunas)
	}
	if len(limpo.Relacoes) != 1 || limpo.Relacoes[0].AoApagar != "CASCADE" {
		t.Fatalf("relações = %+v, esperada 1 traduzida: aresta sem coluna não é aresta", limpo.Relacoes)
	}
}

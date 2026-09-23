package scripts

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type tabelaDoEsquema struct {
	Schema          string `json:"schema"`
	Nome            string `json:"nome"`
	LinhasEstimadas *int64 `json:"linhas_estimadas"`
	TamanhoBytes    *int64 `json:"tamanho_bytes"`
}

type colunaDoEsquema struct {
	Schema           string `json:"schema"`
	Tabela           string `json:"tabela"`
	Nome             string `json:"nome"`
	Tipo             string `json:"tipo"`
	Nulo             bool   `json:"nulo"`
	ChavePrimaria    bool   `json:"chave_primaria"`
	ChaveEstrangeira bool   `json:"chave_estrangeira"`
}

type relacaoDoEsquema struct {
	Nome        string   `json:"nome"`
	DeSchema    string   `json:"de_schema"`
	DeTabela    string   `json:"de_tabela"`
	DeColunas   []string `json:"de_colunas"`
	ParaSchema  string   `json:"para_schema"`
	ParaTabela  string   `json:"para_tabela"`
	ParaColunas []string `json:"para_colunas"`
	AoApagar    string   `json:"ao_apagar"`
}

type esquemaDaSonda struct {
	Ok       bool               `json:"ok"`
	Classe   string             `json:"classe"`
	Erro     string             `json:"erro"`
	Tabelas  []tabelaDoEsquema  `json:"tabelas"`
	Colunas  []colunaDoEsquema  `json:"colunas"`
	Relacoes []relacaoDoEsquema `json:"relacoes"`
}

type cenarioEsquema struct {
	base string
	psql string
}

const psqlComEsquemaSimples = `case "$sql" in
  *confdeltype*)
    printf 'server_addresses_server_id_fkey|public|server_addresses|server_id|public|servers|id|c\n'
    printf 'categorias_pai_id_fkey|public|categorias|pai_id|public|categorias|id|n\n'
    printf 'aponta_composta_x_y_fkey|public|aponta_composta|x,y|public|composta|a,b|r\n'
    ;;
  *pg_total_relation_size*)
    printf 'public|aponta_composta||8192\n'
    printf 'public|categorias|7|16384\n'
    printf 'public|composta|2|8192\n'
    printf 'public|server_addresses|12|16384\n'
    printf 'public|servers|4|81920\n'
    printf 'public|solta|0|16384\n'
    ;;
  *format_type*)
    printf 'public|aponta_composta|x|integer|f|f|t\n'
    printf 'public|aponta_composta|y|integer|f|f|t\n'
    printf 'public|categorias|id|bigint|t|t|f\n'
    printf 'public|categorias|pai_id|bigint|f|f|t\n'
    printf 'public|composta|a|integer|t|t|f\n'
    printf 'public|composta|b|integer|t|t|f\n'
    printf 'public|server_addresses|id|bigint|t|t|f\n'
    printf 'public|server_addresses|server_id|uuid|t|f|t\n'
    printf 'public|servers|id|uuid|t|t|f\n'
    printf 'public|servers|site_id|bigint|f|f|f\n'
    printf 'public|solta|id|bigint|t|t|f\n'
    printf 'public|solta|tags|integer[]|f|f|f\n'
    ;;
  *reltuples*) printf 'public|servers|4\n' ;;
  *) exit 1 ;;
esac
`

const psqlSemTamanho = `case "$sql" in
  *confdeltype*) : ;;
  *pg_total_relation_size*) >&2 echo 'ERROR:  permission denied for function pg_total_relation_size'; exit 1 ;;
  *format_type*) printf 'public|servers|id|uuid|t|t|f\n' ;;
  *reltuples*) printf 'public|servers|\n' ;;
  *) exit 1 ;;
esac
`

const psqlSemPermissao = `>&2 echo 'ERROR:  permission denied for table pg_class'
exit 1
`

const psqlCom61Tabelas = `case "$sql" in
  *confdeltype*)
    i=2
    while [ $i -le 61 ]; do
      printf 'fk_t%s|public|t%s|pai_id|public|t1|id|c\n' "$i" "$i"
      i=$((i + 1))
    done
    ;;
  *pg_total_relation_size*)
    i=1
    while [ $i -le 61 ]; do
      printf 'public|t%s|3|8192\n' "$i"
      i=$((i + 1))
    done
    ;;
  *format_type*)
    i=1
    while [ $i -le 61 ]; do
      printf 'public|t%s|id|bigint|t|t|f\n' "$i"
      i=$((i + 1))
    done
    ;;
  *) exit 1 ;;
esac
`

func rodarSondaEsquema(t *testing.T, c cenarioEsquema) (esquemaDaSonda, string) {
	t.Helper()

	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash indisponível")
	}

	dir := t.TempDir()
	chamadas := filepath.Join(dir, "chamadas")
	escrever := func(nome, corpo string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, nome), []byte(corpo), 0o755); err != nil {
			t.Fatalf("criar %s falso: %v", nome, err)
		}
	}

	logica := c.psql
	if logica == "" {
		logica = "exit 1\n"
	}
	escrever("pgfake", "#!/bin/sh\necho chamou >> "+chamadas+"\nsql=\"\"\nanterior=\"\"\n"+
		"for a in \"$@\"; do\n  if [ \"$anterior\" = \"-c\" ]; then sql=\"$a\"; fi\n  anterior=\"$a\"\ndone\n"+logica)
	escrever("psql", "#!/bin/sh\nexec "+filepath.Join(dir, "pgfake")+" \"$@\"\n")
	escrever("docker", "#!/bin/sh\necho chamou >> "+chamadas+"\nexit 1\n")

	cmd := exec.Command(bash, "-s")
	cmd.Env = append(os.Environ(),
		"PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"DOCKKEEPER_PSQL_CMD=",
		"DOCKKEEPER_ESQUEMA_BASE="+c.base,
		"DOCKKEEPER_ESQUEMA_PORTA=5432",
		"DOCKKEEPER_ESQUEMA_CONTAINER=",
	)
	cmd.Stdin = strings.NewReader(ProbeSchema)
	saida, err := cmd.Output()
	if err != nil {
		t.Fatalf("sonda de esquema falhou: %v", err)
	}

	var payload esquemaDaSonda
	if err := json.Unmarshal(saida, &payload); err != nil {
		t.Fatalf("JSON inválido da sonda (%q): %v", string(saida), err)
	}
	return payload, chamadas
}

func relacaoChamada(t *testing.T, p esquemaDaSonda, nome string) relacaoDoEsquema {
	t.Helper()
	for _, r := range p.Relacoes {
		if r.Nome == nome {
			return r
		}
	}
	t.Fatalf("relação %q ausente em %+v", nome, p.Relacoes)
	return relacaoDoEsquema{}
}

func colunaChamada(t *testing.T, p esquemaDaSonda, tabela, nome string) colunaDoEsquema {
	t.Helper()
	for _, c := range p.Colunas {
		if c.Tabela == tabela && c.Nome == nome {
			return c
		}
	}
	t.Fatalf("coluna %s.%s ausente", tabela, nome)
	return colunaDoEsquema{}
}

func TestSondaDeEsquemaLeTabelasColunasERelacoes(t *testing.T) {
	p, _ := rodarSondaEsquema(t, cenarioEsquema{base: "app", psql: psqlComEsquemaSimples})

	if !p.Ok || p.Classe != "" || p.Erro != "" {
		t.Fatalf("payload = %+v, esperado ok sem falha", p)
	}
	if len(p.Tabelas) != 6 {
		t.Fatalf("tabelas = %+v, esperado 6", p.Tabelas)
	}
	if len(p.Relacoes) != 3 {
		t.Fatalf("relações = %+v, esperado 3", p.Relacoes)
	}

	fk := relacaoChamada(t, p, "server_addresses_server_id_fkey")
	if fk.DeTabela != "server_addresses" || fk.ParaTabela != "servers" {
		t.Errorf("relação = %+v, esperada de server_addresses para servers", fk)
	}
	if len(fk.DeColunas) != 1 || fk.DeColunas[0] != "server_id" {
		t.Errorf("de_colunas = %v, esperado [server_id]", fk.DeColunas)
	}
	if fk.AoApagar != "c" {
		t.Errorf("ao_apagar = %q, esperado o confdeltype cru c; a tradução é do lado Go", fk.AoApagar)
	}

	composta := relacaoChamada(t, p, "aponta_composta_x_y_fkey")
	if len(composta.DeColunas) != 2 || composta.DeColunas[1] != "y" {
		t.Errorf("de_colunas = %v, esperado [x y]: chave composta não pode virar uma coluna só", composta.DeColunas)
	}
	if len(composta.ParaColunas) != 2 || composta.ParaColunas[0] != "a" {
		t.Errorf("para_colunas = %v, esperado [a b]", composta.ParaColunas)
	}
}

func TestSondaDeEsquemaMantemAutoRelacionamento(t *testing.T) {
	p, _ := rodarSondaEsquema(t, cenarioEsquema{base: "app", psql: psqlComEsquemaSimples})

	auto := relacaoChamada(t, p, "categorias_pai_id_fkey")
	if auto.DeTabela != "categorias" || auto.ParaTabela != "categorias" {
		t.Fatalf("relação = %+v, esperada categorias apontando para si mesma", auto)
	}
	if auto.AoApagar != "n" {
		t.Errorf("ao_apagar = %q, esperado n", auto.AoApagar)
	}
}

func TestSondaDeEsquemaListaTabelaSemNenhumaChaveEstrangeira(t *testing.T) {
	p, _ := rodarSondaEsquema(t, cenarioEsquema{base: "app", psql: psqlComEsquemaSimples})

	var vista bool
	for _, tab := range p.Tabelas {
		if tab.Nome == "solta" {
			vista = true
		}
	}
	if !vista {
		t.Fatalf("tabelas = %+v, esperada a tabela solta: sem FK não é motivo para sumir", p.Tabelas)
	}

	for _, r := range p.Relacoes {
		if r.DeTabela == "solta" || r.ParaTabela == "solta" {
			t.Errorf("relação inventada para a tabela solta: %+v", r)
		}
	}

	tags := colunaChamada(t, p, "solta", "tags")
	if tags.Tipo != "integer[]" {
		t.Errorf("tipo = %q, esperado integer[]: tipo de array não pode perder os colchetes", tags.Tipo)
	}
}

func TestSondaDeEsquemaNaoInventaLinhaNemTamanho(t *testing.T) {
	p, _ := rodarSondaEsquema(t, cenarioEsquema{base: "app", psql: psqlComEsquemaSimples})

	for _, tab := range p.Tabelas {
		if tab.Nome != "aponta_composta" {
			continue
		}
		if tab.LinhasEstimadas != nil {
			t.Errorf("linhas_estimadas = %v, esperado ausente: o catálogo não sabe", *tab.LinhasEstimadas)
		}
		if tab.TamanhoBytes == nil || *tab.TamanhoBytes != 8192 {
			t.Errorf("tamanho_bytes = %v, esperado 8192 medido", tab.TamanhoBytes)
		}
	}

	id := colunaChamada(t, p, "servers", "id")
	if id.Nulo || !id.ChavePrimaria {
		t.Errorf("coluna id = %+v, esperada NOT NULL e chave primária", id)
	}
	site := colunaChamada(t, p, "servers", "site_id")
	if !site.Nulo || site.ChavePrimaria || site.ChaveEstrangeira {
		t.Errorf("coluna site_id = %+v, esperada anulável e sem chave", site)
	}
}

func TestSondaDeEsquemaCaiParaConsultaSemTamanho(t *testing.T) {
	p, _ := rodarSondaEsquema(t, cenarioEsquema{base: "app", psql: psqlSemTamanho})

	if !p.Ok {
		t.Fatalf("payload = %+v, esperado ok: só o tamanho ficou fora de alcance", p)
	}
	if len(p.Tabelas) != 1 {
		t.Fatalf("tabelas = %+v, esperado 1", p.Tabelas)
	}
	if p.Tabelas[0].TamanhoBytes != nil {
		t.Errorf("tamanho_bytes = %v, esperado ausente, nunca 0", *p.Tabelas[0].TamanhoBytes)
	}
	if p.Tabelas[0].LinhasEstimadas != nil {
		t.Errorf("linhas_estimadas = %v, esperado ausente", *p.Tabelas[0].LinhasEstimadas)
	}
}

func TestSondaDeEsquemaComCatalogoRecusadoNaoDevolveEsquemaVazio(t *testing.T) {
	p, _ := rodarSondaEsquema(t, cenarioEsquema{base: "app", psql: psqlSemPermissao})

	if p.Ok {
		t.Fatal("ok = true com o catálogo recusado; esquema vazio e falha de leitura ficariam iguais")
	}
	if p.Classe != "sem_permissao" {
		t.Errorf("classe = %q, esperado sem_permissao", p.Classe)
	}
	if !strings.Contains(p.Erro, "permission denied") {
		t.Errorf("erro = %q, esperado carregar o detalhe do psql", p.Erro)
	}
	if len(p.Tabelas) != 0 || len(p.Relacoes) != 0 {
		t.Errorf("payload de falha trouxe conteúdo: %+v", p)
	}
}

func TestSondaDeEsquemaEntregaTodasAsTabelasParaOTetoSerHonesto(t *testing.T) {
	p, _ := rodarSondaEsquema(t, cenarioEsquema{base: "app", psql: psqlCom61Tabelas})

	if !p.Ok {
		t.Fatalf("payload = %+v, esperado ok", p)
	}
	if len(p.Tabelas) != 61 {
		t.Fatalf("tabelas = %d, esperado 61: o teto de 60 é aplicado na API, que precisa do total real",
			len(p.Tabelas))
	}
	if len(p.Relacoes) != 60 {
		t.Fatalf("relações = %d, esperado 60", len(p.Relacoes))
	}
}

func TestSondaDeEsquemaRecusaBaseHostilAntesDeChamarOPsql(t *testing.T) {
	hostis := []string{
		"app; touch /tmp/dockkeeper-invadiu",
		"app`id`",
		"app'--",
		"",
		"../../etc/passwd",
	}

	for _, base := range hostis {
		t.Run(base, func(t *testing.T) {
			p, chamadas := rodarSondaEsquema(t, cenarioEsquema{base: base, psql: psqlComEsquemaSimples})

			if p.Ok || p.Classe != "base_invalida" {
				t.Fatalf("payload = %+v, esperada recusa com classe base_invalida", p)
			}
			if _, err := os.Stat(chamadas); err == nil {
				t.Fatal("o psql foi executado com um nome de base recusado")
			}
		})
	}
}

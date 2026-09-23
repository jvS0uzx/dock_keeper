package ssh

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"regexp"
	"strconv"
	"strings"

	"github.com/jvS0uzx/dock_keeper/scripts"
)

const (
	FalhaBaseInvalida    = "base_invalida"
	FalhaBaseInexistente = "base_inexistente"
	FalhaSemPermissao    = "sem_permissao"
	FalhaIndisponivel    = "indisponivel"

	maxLinhaDoEsquema = 16 << 20
)

var ErrBaseInvalida = errors.New("nome de base recusado: use apenas letras, números, ponto, hífen e sublinhado")

var ErrContainerInvalido = errors.New("nome de container recusado pela coleta de esquema")

var nomeDeBase = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]{0,62}$`)

type EsquemaTabelaBruta struct {
	Schema          string `json:"schema"`
	Nome            string `json:"nome"`
	LinhasEstimadas *int64 `json:"linhas_estimadas"`
	TamanhoBytes    *int64 `json:"tamanho_bytes"`
}

type EsquemaColunaBruta struct {
	Schema           string `json:"schema"`
	Tabela           string `json:"tabela"`
	Nome             string `json:"nome"`
	Tipo             string `json:"tipo"`
	Nulo             bool   `json:"nulo"`
	ChavePrimaria    bool   `json:"chave_primaria"`
	ChaveEstrangeira bool   `json:"chave_estrangeira"`
}

type EsquemaRelacaoBruta struct {
	Nome        string   `json:"nome"`
	DeSchema    string   `json:"de_schema"`
	DeTabela    string   `json:"de_tabela"`
	DeColunas   []string `json:"de_colunas"`
	ParaSchema  string   `json:"para_schema"`
	ParaTabela  string   `json:"para_tabela"`
	ParaColunas []string `json:"para_colunas"`
	AoApagar    string   `json:"ao_apagar"`
}

type EsquemaPayload struct {
	Ok       bool                  `json:"ok"`
	Classe   string                `json:"classe"`
	Erro     string                `json:"erro"`
	Tabelas  []EsquemaTabelaBruta  `json:"tabelas"`
	Colunas  []EsquemaColunaBruta  `json:"colunas"`
	Relacoes []EsquemaRelacaoBruta `json:"relacoes"`
}

type AlvoDeEsquema struct {
	Base          string
	Porta         int
	EmContainer   bool
	ContainerNome string
}

func NomeDeBaseValido(nome string) bool {
	return nomeDeBase.MatchString(nome)
}

func traduzirAoApagar(bruto string) string {
	switch strings.ToLower(strings.TrimSpace(bruto)) {
	case "c":
		return "CASCADE"
	case "r":
		return "RESTRICT"
	case "n":
		return "SET NULL"
	case "d":
		return "SET DEFAULT"
	case "a":
		return "NO ACTION"
	default:
		return ""
	}
}

func normalizarColunasDaRelacao(colunas []string) []string {
	limpas := make([]string, 0, len(colunas))
	for _, c := range colunas {
		nome := truncarTexto(strings.TrimSpace(c), 128)
		if nome == "" {
			continue
		}
		limpas = append(limpas, nome)
	}
	return limpas
}

func normalizarEsquema(p EsquemaPayload) EsquemaPayload {
	limpo := EsquemaPayload{
		Ok:       p.Ok,
		Classe:   strings.TrimSpace(p.Classe),
		Erro:     strings.TrimSpace(p.Erro),
		Tabelas:  make([]EsquemaTabelaBruta, 0, len(p.Tabelas)),
		Colunas:  make([]EsquemaColunaBruta, 0, len(p.Colunas)),
		Relacoes: make([]EsquemaRelacaoBruta, 0, len(p.Relacoes)),
	}

	for _, t := range p.Tabelas {
		schema := truncarTexto(strings.TrimSpace(t.Schema), 128)
		nome := truncarTexto(strings.TrimSpace(t.Nome), 128)
		if schema == "" || nome == "" {
			continue
		}
		limpo.Tabelas = append(limpo.Tabelas, EsquemaTabelaBruta{
			Schema:          schema,
			Nome:            nome,
			LinhasEstimadas: t.LinhasEstimadas,
			TamanhoBytes:    t.TamanhoBytes,
		})
	}

	for _, c := range p.Colunas {
		schema := truncarTexto(strings.TrimSpace(c.Schema), 128)
		tabela := truncarTexto(strings.TrimSpace(c.Tabela), 128)
		nome := truncarTexto(strings.TrimSpace(c.Nome), 128)
		if schema == "" || tabela == "" || nome == "" {
			continue
		}
		limpo.Colunas = append(limpo.Colunas, EsquemaColunaBruta{
			Schema:           schema,
			Tabela:           tabela,
			Nome:             nome,
			Tipo:             truncarTexto(strings.TrimSpace(c.Tipo), 128),
			Nulo:             c.Nulo,
			ChavePrimaria:    c.ChavePrimaria,
			ChaveEstrangeira: c.ChaveEstrangeira,
		})
	}

	for _, r := range p.Relacoes {
		de := normalizarColunasDaRelacao(r.DeColunas)
		para := normalizarColunasDaRelacao(r.ParaColunas)
		deTabela := truncarTexto(strings.TrimSpace(r.DeTabela), 128)
		paraTabela := truncarTexto(strings.TrimSpace(r.ParaTabela), 128)
		if deTabela == "" || paraTabela == "" || len(de) == 0 || len(para) == 0 {
			continue
		}
		limpo.Relacoes = append(limpo.Relacoes, EsquemaRelacaoBruta{
			Nome:        truncarTexto(strings.TrimSpace(r.Nome), 128),
			DeSchema:    truncarTexto(strings.TrimSpace(r.DeSchema), 128),
			DeTabela:    deTabela,
			DeColunas:   de,
			ParaSchema:  truncarTexto(strings.TrimSpace(r.ParaSchema), 128),
			ParaTabela:  paraTabela,
			ParaColunas: para,
			AoApagar:    traduzirAoApagar(r.AoApagar),
		})
	}

	return limpo
}

func preludioDeEsquema(alvo AlvoDeEsquema) string {
	porta := alvo.Porta
	if porta <= 0 {
		porta = 5432
	}
	container := ""
	if alvo.EmContainer {
		container = alvo.ContainerNome
	}

	var b strings.Builder
	b.WriteString("DOCKKEEPER_ESQUEMA_BASE=\"" + alvo.Base + "\"\n")
	b.WriteString("DOCKKEEPER_ESQUEMA_PORTA=" + strconv.Itoa(porta) + "\n")
	b.WriteString("DOCKKEEPER_ESQUEMA_CONTAINER=\"" + container + "\"\n")
	return b.String()
}

func enviarSondaDeEsquema(session sessionWriter, t Target, alvo AlvoDeEsquema) error {
	stdin, err := session.StdinPipe()
	if err != nil {
		return err
	}
	go func() {
		defer stdin.Close()
		corpo := scriptPrelude(t) + preludioDeEsquema(alvo) + scripts.ProbeSchema
		if _, err := io.WriteString(stdin, corpo); err != nil {
			log.Printf("[SSH] erro ao enviar a sonda de esquema: %v", err)
		}
	}()
	return session.Start("bash -s")
}

func ColetarEsquema(ctx context.Context, t Target, alvo AlvoDeEsquema) (EsquemaPayload, error) {
	var vazio EsquemaPayload

	if !NomeDeBaseValido(alvo.Base) {
		return vazio, ErrBaseInvalida
	}
	if alvo.EmContainer && !IsValidContainerName(alvo.ContainerNome) {
		return vazio, ErrContainerInvalido
	}

	client, session, err := openSession(t)
	if err != nil {
		return vazio, err
	}
	defer client.Close()
	defer session.Close()

	stopOnCancel(ctx, client, session)

	stdout, err := session.StdoutPipe()
	if err != nil {
		return vazio, err
	}
	if err := enviarSondaDeEsquema(session, t, alvo); err != nil {
		return vazio, err
	}

	payload, lido := vazio, false
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), maxLinhaDoEsquema)
	for scanner.Scan() {
		linha := bytes.TrimSpace(scanner.Bytes())
		if len(linha) == 0 || linha[0] != '{' {
			continue
		}
		var candidata EsquemaPayload
		if err := json.Unmarshal(linha, &candidata); err == nil {
			payload, lido = candidata, true
		}
	}
	if err := scanner.Err(); err != nil {
		return vazio, err
	}
	if err := session.Wait(); err != nil && !lido {
		return vazio, err
	}
	if !lido {
		return vazio, errors.New("sonda de esquema não devolveu JSON")
	}
	return normalizarEsquema(payload), nil
}

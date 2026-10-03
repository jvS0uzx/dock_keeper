package api

import (
	"context"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/database"
	"github.com/jvS0uzx/dock_keeper/internal/ssh"
)

const (
	tetoDeTabelasNoEsquema = 60
	tetoDeColunasComuns    = 8

	motorPostgres          = "postgres"
	estadoDaInstanciaAtivo = "ativo"
)

var coletarEsquema = ssh.ColetarEsquema

type EsquemaColunaView struct {
	Nome          string `json:"nome"`
	Tipo          string `json:"tipo"`
	Nulo          bool   `json:"nulo"`
	ChavePrimaria bool   `json:"chave_primaria"`
}

type EsquemaTabelaView struct {
	Schema          string              `json:"schema"`
	Nome            string              `json:"nome"`
	LinhasEstimadas *int64              `json:"linhas_estimadas"`
	TamanhoBytes    *int64              `json:"tamanho_bytes"`
	ColunasChave    []string            `json:"colunas_chave"`
	Colunas         []EsquemaColunaView `json:"colunas"`
}

type EsquemaRelacaoView struct {
	Nome        string   `json:"nome"`
	DeSchema    string   `json:"de_schema"`
	DeTabela    string   `json:"de_tabela"`
	DeColunas   []string `json:"de_colunas"`
	ParaSchema  string   `json:"para_schema"`
	ParaTabela  string   `json:"para_tabela"`
	ParaColunas []string `json:"para_colunas"`
	AoApagar    string   `json:"ao_apagar"`
}

type EsquemaView struct {
	InstanciaID     uint                 `json:"instancia_id"`
	Base            string               `json:"base"`
	Motor           string               `json:"motor"`
	ColetadoEm      time.Time            `json:"coletado_em"`
	SuportaDiagrama bool                 `json:"suporta_diagrama"`
	Motivo          string               `json:"motivo,omitempty"`
	Schemas         []string             `json:"schemas"`
	Tabelas         []EsquemaTabelaView  `json:"tabelas"`
	Relacoes        []EsquemaRelacaoView `json:"relacoes"`
	Truncado        bool                 `json:"truncado"`
	TotalTabelas    int                  `json:"total_tabelas"`
}

type instanciaParaEsquema struct {
	ID            uint
	ServerID      string
	Porta         int
	EmContainer   bool
	ContainerNome string
	Motor         string
	Estado        string
	Motivo        string
}

const colunasDaInstanciaDeEsquema = `postgres_instancias.id,
	postgres_instancias.server_id,
	postgres_instancias.porta,
	postgres_instancias.em_container,
	postgres_instancias.container_nome,
	postgres_instancias.motor,
	postgres_instancias.estado,
	postgres_instancias.motivo`

func idDaRotaDeEsquema(caminho string) (uint, bool) {
	resto := strings.TrimPrefix(caminho, "/api/bancos/")
	if resto == caminho {
		return 0, false
	}

	partes := strings.Split(strings.TrimSuffix(resto, "/"), "/")
	if len(partes) != 2 || partes[1] != "esquema" {
		return 0, false
	}

	n, err := strconv.ParseUint(partes[0], 10, 64)
	if err != nil || n == 0 {
		return 0, false
	}
	return uint(n), true
}

func (c Config) esquemaDaBaseHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := idDaRotaDeEsquema(r.URL.Path)
	if !ok {
		writeError(w, http.StatusNotFound, "rota não encontrada")
		return
	}

	scope, status := resolveScope(sessionFrom(r), r)
	if status != 0 {
		writeError(w, status, "site_id inválido ou fora do seu alcance")
		return
	}

	base := strings.TrimSpace(r.URL.Query().Get("base"))
	if base == "" {
		writeError(w, http.StatusBadRequest, "informe a base em ?base=")
		return
	}
	if !ssh.NomeDeBaseValido(base) {
		writeError(w, http.StatusBadRequest,
			"nome de base inválido: use apenas letras, números, ponto, hífen e sublinhado")
		return
	}

	inst, err := instanciaDoEsquema(r.Context(), scope, id)
	if err != nil {
		writeError(w, http.StatusNotFound, "instância de banco não encontrada")
		return
	}

	if inst.Motor != "" && inst.Motor != motorPostgres {
		writeJSON(w, http.StatusOK, esquemaSemDiagrama(inst, base))
		return
	}

	if inst.Estado != estadoDaInstanciaAtivo {
		writeError(w, http.StatusConflict, motivoDaInstanciaParada(inst))
		return
	}

	conhecida, err := baseNoInventario(r.Context(), inst.ID, base)
	if err != nil {
		log.Printf("[API] erro ao conferir a base %q da instância %d: %v", base, inst.ID, err)
		writeError(w, http.StatusInternalServerError, "falha ao ler o inventário de bases")
		return
	}
	if !conhecida {
		writeError(w, http.StatusNotFound, "base "+base+" não existe nesta instância")
		return
	}

	var servidor database.Server
	if err := database.From(r.Context()).Where("id = ?", inst.ServerID).Take(&servidor).Error; err != nil {
		writeError(w, http.StatusNotFound, "servidor da instância não encontrado")
		return
	}

	alvo := c.sshTarget(servidor)
	release, ok := holdSSHSession(w, alvo)
	if !ok {
		return
	}
	defer release()

	payload, err := coletarEsquema(r.Context(), alvo, ssh.AlvoDeEsquema{
		Base:          base,
		Porta:         inst.Porta,
		EmContainer:   inst.EmContainer,
		ContainerNome: inst.ContainerNome,
	})
	if err != nil {
		log.Printf("[API] coleta de esquema da base %q em %s falhou: %v", base, servidor.HostIP, err)
		writeError(w, http.StatusBadGateway, "falha ao ler o esquema da base "+base)
		return
	}
	if !payload.Ok {
		responderFalhaDeEsquema(w, base, payload)
		return
	}

	view := montarEsquema(payload, tetoDeTabelasNoEsquema)
	view.InstanciaID = inst.ID
	view.Base = base
	view.Motor = motorDaInstancia(inst)
	view.SuportaDiagrama = true
	view.ColetadoEm = time.Now().UTC()
	writeJSON(w, http.StatusOK, view)
}

func motorDaInstancia(inst instanciaParaEsquema) string {
	if inst.Motor == "" {
		return motorPostgres
	}
	return inst.Motor
}

func nomeDoMotor(motor string) string {
	switch motor {
	case "mysql":
		return "MySQL"
	case "mariadb":
		return "MariaDB"
	case motorPostgres:
		return "PostgreSQL"
	default:
		return motor
	}
}

func motivoDaInstanciaParada(inst instanciaParaEsquema) string {
	texto := "a instância está em " + inst.Estado + " e não foi consultada"
	if strings.TrimSpace(inst.Motivo) == "" {
		return texto
	}
	return texto + ": " + inst.Motivo
}

func esquemaSemDiagrama(inst instanciaParaEsquema, base string) EsquemaView {
	return EsquemaView{
		InstanciaID:     inst.ID,
		Base:            base,
		Motor:           motorDaInstancia(inst),
		ColetadoEm:      time.Now().UTC(),
		SuportaDiagrama: false,
		Motivo:          "O diagrama ainda não está disponível para " + nomeDoMotor(motorDaInstancia(inst)) + ".",
		Schemas:         []string{},
		Tabelas:         []EsquemaTabelaView{},
		Relacoes:        []EsquemaRelacaoView{},
	}
}

func responderFalhaDeEsquema(w http.ResponseWriter, base string, payload ssh.EsquemaPayload) {
	detalhe := strings.TrimSpace(payload.Erro)
	if payload.Classe == ssh.FalhaBaseInexistente {
		writeError(w, http.StatusNotFound, "base "+base+" não existe nesta instância")
		return
	}

	mensagem := "falha ao ler o esquema da base " + base
	if detalhe != "" {
		mensagem += ": " + detalhe
	}
	writeError(w, http.StatusBadGateway, mensagem)
}

func instanciaDoEsquema(ctx context.Context, scope siteScope, id uint) (instanciaParaEsquema, error) {
	tx := database.From(ctx).
		Model(&database.PostgresInstancia{}).
		Joins("JOIN servers ON servers.id = postgres_instancias.server_id AND servers.deleted_at IS NULL").
		Where("postgres_instancias.id = ?", id)

	var linha instanciaParaEsquema
	err := scope.apply(tx).Select(colunasDaInstanciaDeEsquema).Take(&linha).Error
	return linha, err
}

func baseNoInventario(ctx context.Context, instanciaID uint, base string) (bool, error) {
	var nomes []string
	err := database.From(ctx).
		Model(&database.PostgresBase{}).
		Where("instancia_id = ?", instanciaID).
		Pluck("nome", &nomes).Error
	if err != nil {
		return false, err
	}
	if len(nomes) == 0 {
		return true, nil
	}

	for _, nome := range nomes {
		if nome == base {
			return true, nil
		}
	}
	return false, nil
}

func chaveDaTabela(schema, nome string) string {
	return schema + "." + nome
}

func grauDasTabelas(p ssh.EsquemaPayload, existe map[string]bool) map[string]int {
	grau := make(map[string]int, len(p.Tabelas))
	for _, r := range p.Relacoes {
		de := chaveDaTabela(r.DeSchema, r.DeTabela)
		para := chaveDaTabela(r.ParaSchema, r.ParaTabela)
		if !existe[de] || !existe[para] {
			continue
		}
		grau[de]++
		if de != para {
			grau[para]++
		}
	}
	return grau
}

func tabelasMaisConectadas(tabelas []ssh.EsquemaTabelaBruta, grau map[string]int, teto int) []ssh.EsquemaTabelaBruta {
	ordenadas := make([]ssh.EsquemaTabelaBruta, len(tabelas))
	copy(ordenadas, tabelas)

	sort.SliceStable(ordenadas, func(i, j int) bool {
		chaveI := chaveDaTabela(ordenadas[i].Schema, ordenadas[i].Nome)
		chaveJ := chaveDaTabela(ordenadas[j].Schema, ordenadas[j].Nome)
		if grau[chaveI] != grau[chaveJ] {
			return grau[chaveI] > grau[chaveJ]
		}
		return chaveI < chaveJ
	})

	return ordenadas[:teto]
}

func colunasDaTabela(colunas []ssh.EsquemaColunaBruta, limite int) ([]EsquemaColunaView, []string) {
	escolhidas := make([]EsquemaColunaView, 0, len(colunas))
	chaves := make([]string, 0, 4)
	comuns := 0

	for _, c := range colunas {
		if c.ChavePrimaria {
			chaves = append(chaves, c.Nome)
		}
		if !c.ChavePrimaria && !c.ChaveEstrangeira {
			if comuns >= limite {
				continue
			}
			comuns++
		}
		escolhidas = append(escolhidas, EsquemaColunaView{
			Nome:          c.Nome,
			Tipo:          c.Tipo,
			Nulo:          c.Nulo,
			ChavePrimaria: c.ChavePrimaria,
		})
	}

	return escolhidas, chaves
}

func montarEsquema(p ssh.EsquemaPayload, teto int) EsquemaView {
	existe := make(map[string]bool, len(p.Tabelas))
	for _, t := range p.Tabelas {
		existe[chaveDaTabela(t.Schema, t.Nome)] = true
	}

	escolhidas := p.Tabelas
	truncado := false
	if teto > 0 && len(p.Tabelas) > teto {
		escolhidas = tabelasMaisConectadas(p.Tabelas, grauDasTabelas(p, existe), teto)
		truncado = true
	}

	desenhadas := make(map[string]bool, len(escolhidas))
	for _, t := range escolhidas {
		desenhadas[chaveDaTabela(t.Schema, t.Nome)] = true
	}

	porTabela := make(map[string][]ssh.EsquemaColunaBruta, len(escolhidas))
	for _, c := range p.Colunas {
		chave := chaveDaTabela(c.Schema, c.Tabela)
		if !desenhadas[chave] {
			continue
		}
		porTabela[chave] = append(porTabela[chave], c)
	}

	ordenadas := make([]ssh.EsquemaTabelaBruta, len(escolhidas))
	copy(ordenadas, escolhidas)
	sort.SliceStable(ordenadas, func(i, j int) bool {
		return chaveDaTabela(ordenadas[i].Schema, ordenadas[i].Nome) <
			chaveDaTabela(ordenadas[j].Schema, ordenadas[j].Nome)
	})

	vistos := make(map[string]bool, len(ordenadas))
	schemas := make([]string, 0, 4)
	tabelas := make([]EsquemaTabelaView, 0, len(ordenadas))
	for _, t := range ordenadas {
		chave := chaveDaTabela(t.Schema, t.Nome)
		colunas, chavesPrimarias := colunasDaTabela(porTabela[chave], tetoDeColunasComuns)
		tabelas = append(tabelas, EsquemaTabelaView{
			Schema:          t.Schema,
			Nome:            t.Nome,
			LinhasEstimadas: t.LinhasEstimadas,
			TamanhoBytes:    t.TamanhoBytes,
			ColunasChave:    chavesPrimarias,
			Colunas:         colunas,
		})
		if !vistos[t.Schema] {
			vistos[t.Schema] = true
			schemas = append(schemas, t.Schema)
		}
	}
	sort.Strings(schemas)

	relacoes := make([]EsquemaRelacaoView, 0, len(p.Relacoes))
	for _, r := range p.Relacoes {
		de := chaveDaTabela(r.DeSchema, r.DeTabela)
		para := chaveDaTabela(r.ParaSchema, r.ParaTabela)
		if !desenhadas[de] || !desenhadas[para] {
			continue
		}
		relacoes = append(relacoes, EsquemaRelacaoView{
			Nome:        r.Nome,
			DeSchema:    r.DeSchema,
			DeTabela:    r.DeTabela,
			DeColunas:   r.DeColunas,
			ParaSchema:  r.ParaSchema,
			ParaTabela:  r.ParaTabela,
			ParaColunas: r.ParaColunas,
			AoApagar:    r.AoApagar,
		})
	}

	return EsquemaView{
		Schemas:      schemas,
		Tabelas:      tabelas,
		Relacoes:     relacoes,
		Truncado:     truncado,
		TotalTabelas: len(p.Tabelas),
	}
}

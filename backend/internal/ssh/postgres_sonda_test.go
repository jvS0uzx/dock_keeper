package ssh

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/database"
)

const jsonDaSondaPostgres = `{"descoberta_host_ok":true,"descoberta_container_ok":true,"instancias":[{"porta":5432,"em_container":true,"container_nome":"gestao-ativos-postgres","versao":"18.0","papel":"primario","wal_level":"replica","max_wal_senders":10,"archive_mode":"off","estado":"ativo","motivo":"","bases":[{"nome":"dockkeeper","dono":"postgres","encoding":"UTF8","tamanho_bytes":41943040,"conexoes":3},{"nome":"cega","dono":"","encoding":"UTF8"}]}]}`

func TestSondarPostgresIgnoraRuidoAntesDoJSON(t *testing.T) {
	alvo := alvoComServidor(t, func(string) respostaExec {
		return respostaExec{
			stdout:       "sudo: unable to resolve host\n" + jsonDaSondaPostgres + "\n",
			consomeStdin: true,
		}
	})

	p, err := SondarPostgres(context.Background(), alvo)
	if err != nil {
		t.Fatalf("sonda falhou: %v", err)
	}
	if len(p.Instancias) != 1 {
		t.Fatalf("instâncias = %+v, esperado 1", p.Instancias)
	}
	if !p.DescobertaHostOk || !p.DescobertaContainerOk {
		t.Errorf("host_ok=%v container_ok=%v, esperado true nos dois: vieram true no JSON",
			p.DescobertaHostOk, p.DescobertaContainerOk)
	}

	inst := p.Instancias[0]
	if inst.Porta != 5432 || !inst.EmContainer || inst.ContainerNome != "gestao-ativos-postgres" {
		t.Errorf("instância = %+v, esperada a do container na 5432", inst)
	}
	if inst.MaxWalSenders == nil || *inst.MaxWalSenders != 10 {
		t.Errorf("max_wal_senders = %v, esperado 10", inst.MaxWalSenders)
	}
	if len(inst.Bases) != 2 {
		t.Fatalf("bases = %+v, esperado 2", inst.Bases)
	}
	if inst.Bases[0].TamanhoBytes == nil || *inst.Bases[0].TamanhoBytes != 41943040 {
		t.Errorf("tamanho da primeira base = %v, esperado 41943040", inst.Bases[0].TamanhoBytes)
	}
	if inst.Bases[1].TamanhoBytes != nil || inst.Bases[1].Conexoes != nil {
		t.Errorf("base sem medida = %+v, esperado ponteiro nulo, nunca zero", inst.Bases[1])
	}
}

func TestSondarPostgresFicaComAUltimaLinhaValida(t *testing.T) {
	alvo := alvoComServidor(t, func(string) respostaExec {
		return respostaExec{
			stdout: `{"instancias":[{"porta":1111,"estado":"ativo"}]}` + "\n" +
				`{"instancias":[{"porta":2222,"estado":"ativo"}]}` + "\n",
			consomeStdin: true,
		}
	})

	p, err := SondarPostgres(context.Background(), alvo)
	if err != nil {
		t.Fatalf("sonda falhou: %v", err)
	}
	if len(p.Instancias) != 1 || p.Instancias[0].Porta != 2222 {
		t.Fatalf("instâncias = %+v, esperada a última linha", p.Instancias)
	}
}

func TestSondarPostgresSemJSONExplicaEmPortugues(t *testing.T) {
	alvo := alvoComServidor(t, func(string) respostaExec {
		return respostaExec{stdout: "bash: psql: comando não encontrado\n", consomeStdin: true}
	})

	_, err := SondarPostgres(context.Background(), alvo)
	if err == nil {
		t.Fatal("saída sem JSON passou como sonda bem-sucedida")
	}
	if !strings.Contains(err.Error(), "não devolveu JSON") {
		t.Errorf("erro = %q, esperado explicar em pt-BR", err.Error())
	}
}

func TestNormalizarInstanciaDerrubaValorForaDoConjunto(t *testing.T) {
	casos := []struct {
		papel     string
		estado    string
		esperadoP string
		esperadoE string
	}{
		{"primario", "ativo", papelPrimario, estadoPgAtivo},
		{"replica", "sem_acesso", papelReplica, estadoPgSemAcesso},
		{"PRIMARIO", " inativo ", papelPrimario, estadoPgInativo},
		{"master", "up", papelDesconhecido, estadoPgDesconhecido},
		{"", "", papelDesconhecido, estadoPgDesconhecido},
	}

	for _, caso := range casos {
		limpa := normalizarInstancia(PostgresInstanciaPayload{Papel: caso.papel, Estado: caso.estado})
		if limpa.Papel != caso.esperadoP {
			t.Errorf("papel %q virou %q, esperado %q", caso.papel, limpa.Papel, caso.esperadoP)
		}
		if limpa.Estado != caso.esperadoE {
			t.Errorf("estado %q virou %q, esperado %q", caso.estado, limpa.Estado, caso.esperadoE)
		}
	}
}

func TestNormalizarInstanciaPreservaNuloEApagaMotivoDeInstanciaViva(t *testing.T) {
	tamanho := int64(4096)
	inst := PostgresInstanciaPayload{
		Porta:  5432,
		Papel:  "primario",
		Estado: "ativo",
		Motivo: "sobra de uma coleta anterior",
		Bases: []PostgresBasePayload{
			{Nome: " app ", Dono: "postgres", Encoding: "UTF8", TamanhoBytes: &tamanho},
			{Nome: "restrito"},
			{Nome: "   "},
		},
	}

	limpa := normalizarInstancia(inst)
	if limpa.Motivo != "" {
		t.Errorf("motivo = %q, esperado vazio quando a instância respondeu", limpa.Motivo)
	}
	if len(limpa.Bases) != 2 {
		t.Fatalf("bases = %+v, esperado 2: base sem nome não entra", limpa.Bases)
	}
	if limpa.Bases[0].Nome != "app" {
		t.Errorf("nome = %q, esperado sem espaço em volta", limpa.Bases[0].Nome)
	}
	if limpa.Bases[0].TamanhoBytes == nil || *limpa.Bases[0].TamanhoBytes != 4096 {
		t.Errorf("tamanho medido = %v, esperado 4096", limpa.Bases[0].TamanhoBytes)
	}
	if limpa.Bases[1].TamanhoBytes != nil || limpa.Bases[1].Conexoes != nil {
		t.Errorf("base não medida = %+v, esperado ponteiro nulo", limpa.Bases[1])
	}
}

func TestNormalizarInstanciaCortaTextoNoLimiteDaColuna(t *testing.T) {
	limpa := normalizarInstancia(PostgresInstanciaPayload{
		Versao:        strings.Repeat("v", 80),
		WalLevel:      strings.Repeat("w", 40),
		ArchiveMode:   strings.Repeat("a", 40),
		ContainerNome: strings.Repeat("c", 300),
		Bases:         []PostgresBasePayload{{Nome: strings.Repeat("b", 300), Encoding: strings.Repeat("e", 60)}},
	})

	if len(limpa.Versao) != 32 || len(limpa.WalLevel) != 16 || len(limpa.ArchiveMode) != 16 {
		t.Errorf("versao=%d wal_level=%d archive_mode=%d, esperado 32/16/16",
			len(limpa.Versao), len(limpa.WalLevel), len(limpa.ArchiveMode))
	}
	if len(limpa.ContainerNome) != 128 {
		t.Errorf("container_nome = %d caracteres, esperado 128", len(limpa.ContainerNome))
	}
	if len(limpa.Bases[0].Nome) != 128 || len(limpa.Bases[0].Encoding) != 32 {
		t.Errorf("nome=%d encoding=%d, esperado 128/32", len(limpa.Bases[0].Nome), len(limpa.Bases[0].Encoding))
	}
}

func TestNormalizarSondaDescartaPortaInvalidaERepetida(t *testing.T) {
	p := PostgresProbePayload{Instancias: []PostgresInstanciaPayload{
		{Porta: 5432, Estado: "ativo"},
		{Porta: 5432, Estado: "sem_acesso"},
		{Porta: 0, Estado: "ativo"},
		{Porta: -1, Estado: "ativo"},
		{Porta: 5433, Estado: "ativo"},
	}}

	limpo := normalizarSondaPostgres(p)
	if len(limpo.Instancias) != 2 {
		t.Fatalf("instâncias = %+v, esperado 2: porta repetida quebraria o upsert", limpo.Instancias)
	}
	if limpo.Instancias[0].Porta != 5432 || limpo.Instancias[1].Porta != 5433 {
		t.Errorf("portas = %d e %d, esperado 5432 e 5433",
			limpo.Instancias[0].Porta, limpo.Instancias[1].Porta)
	}
}

func TestNormalizarBasesDescartaNomeRepetido(t *testing.T) {
	limpa := normalizarInstancia(PostgresInstanciaPayload{
		Bases: []PostgresBasePayload{{Nome: "app"}, {Nome: " app "}, {Nome: "outra"}},
	})

	if len(limpa.Bases) != 2 {
		t.Fatalf("bases = %+v, esperado 2: nome repetido derrubaria o ON CONFLICT", limpa.Bases)
	}
}

func TestPayloadSemChaveDeDescobertaCaiNoLadoSeguro(t *testing.T) {
	alvo := alvoComServidor(t, func(string) respostaExec {
		return respostaExec{stdout: `{"instancias":[]}` + "\n", consomeStdin: true}
	})

	p, err := SondarPostgres(context.Background(), alvo)
	if err != nil {
		t.Fatalf("sonda falhou: %v", err)
	}
	if p.DescobertaHostOk || p.DescobertaContainerOk {
		t.Fatalf("host_ok=%v container_ok=%v sem as chaves no JSON; o zero do bool precisa significar não podar",
			p.DescobertaHostOk, p.DescobertaContainerOk)
	}
}

func TestNormalizarSondaCarregaOsDoisFlagsDeDescoberta(t *testing.T) {
	for _, host := range []bool{true, false} {
		for _, container := range []bool{true, false} {
			limpo := normalizarSondaPostgres(PostgresProbePayload{
				DescobertaHostOk:      host,
				DescobertaContainerOk: container,
				Instancias:            []PostgresInstanciaPayload{{Porta: 5432, Estado: "ativo"}},
			})
			if limpo.DescobertaHostOk != host || limpo.DescobertaContainerOk != container {
				t.Errorf("host_ok=%v container_ok=%v, esperado %v e %v: a poda depende deles",
					limpo.DescobertaHostOk, limpo.DescobertaContainerOk, host, container)
			}
		}
	}
}

func instanciaDeTeste(porta int, emContainer bool) PostgresInstanciaPayload {
	nome := ""
	if emContainer {
		nome = "pg-teste"
	}
	return PostgresInstanciaPayload{
		Porta: porta, EmContainer: emContainer, ContainerNome: nome,
		Papel: "primario", Estado: "ativo", Versao: "16.2",
		Bases: []PostgresBasePayload{{Nome: "app", Dono: "postgres", Encoding: "UTF8"}},
	}
}

func sondaPostgresDeTeste(hostOk, containerOk bool, instancias ...PostgresInstanciaPayload) PostgresProbePayload {
	return PostgresProbePayload{
		DescobertaHostOk:      hostOk,
		DescobertaContainerOk: containerOk,
		Instancias:            instancias,
	}
}

func portasGravadas(t *testing.T, serverID string) []int {
	t.Helper()
	portas := make([]int, 0, 4)
	for _, linha := range instanciasGravadas(t, serverID) {
		portas = append(portas, linha.Porta)
	}
	return portas
}

func instanciasGravadas(t *testing.T, serverID string) []database.PostgresInstancia {
	t.Helper()
	var linhas []database.PostgresInstancia
	if err := database.DB.Where("server_id = ?", serverID).Order("porta ASC").Find(&linhas).Error; err != nil {
		t.Fatalf("reler instâncias: %v", err)
	}
	return linhas
}

func TestGravarSondaPostgresPodaInstanciaQueSumiu(t *testing.T) {
	srv := servidorDeTeste(t)

	agora := time.Now().UTC()
	cheia := sondaPostgresDeTeste(true, true, instanciaDeTeste(5432, false), instanciaDeTeste(5433, false))
	if err := gravarSondaPostgres(srv.ID, cheia, agora); err != nil {
		t.Fatalf("primeira sonda: %v", err)
	}
	if len(instanciasGravadas(t, srv.ID)) != 2 {
		t.Fatalf("primeira sonda não gravou as duas instâncias")
	}

	menor := sondaPostgresDeTeste(true, true, instanciaDeTeste(5432, false))
	if err := gravarSondaPostgres(srv.ID, menor, agora.Add(time.Second)); err != nil {
		t.Fatalf("segunda sonda: %v", err)
	}

	linhas := instanciasGravadas(t, srv.ID)
	if len(linhas) != 1 || linhas[0].Porta != 5432 {
		t.Fatalf("instâncias = %+v, esperada só a 5432: a descoberta rodou e não viu a outra", linhas)
	}
}

func TestGravarSondaPostgresCegaPreservaOInventario(t *testing.T) {
	srv := servidorDeTeste(t)

	agora := time.Now().UTC()
	cheia := sondaPostgresDeTeste(true, true, instanciaDeTeste(5432, false), instanciaDeTeste(5433, true))
	if err := gravarSondaPostgres(srv.ID, cheia, agora); err != nil {
		t.Fatalf("primeira sonda: %v", err)
	}

	cega := PostgresProbePayload{}
	if err := gravarSondaPostgres(srv.ID, cega, agora.Add(time.Second)); err != nil {
		t.Fatalf("segunda sonda: %v", err)
	}

	linhas := instanciasGravadas(t, srv.ID)
	if len(linhas) != 2 {
		t.Fatalf("instâncias = %d, esperado 2: sem mecanismo de descoberta não se apaga inventário", len(linhas))
	}

	var bases []database.PostgresBase
	database.DB.Where("instancia_id = ?", linhas[0].ID).Find(&bases)
	if len(bases) != 1 {
		t.Errorf("bases = %d, esperado 1: sonda cega também não apaga base", len(bases))
	}
}

func TestGravarSondaPostgresSoPodaOEscopoDoDockerQuandoAListagemFalha(t *testing.T) {
	srv := servidorDeTeste(t)

	agora := time.Now().UTC()
	cheia := sondaPostgresDeTeste(true, true, instanciaDeTeste(5432, false), instanciaDeTeste(5433, true))
	if err := gravarSondaPostgres(srv.ID, cheia, agora); err != nil {
		t.Fatalf("primeira sonda: %v", err)
	}

	soDocker := sondaPostgresDeTeste(false, true)
	if err := gravarSondaPostgres(srv.ID, soDocker, agora.Add(time.Second)); err != nil {
		t.Fatalf("segunda sonda: %v", err)
	}

	portas := portasGravadas(t, srv.ID)
	if len(portas) != 1 || portas[0] != 5432 {
		t.Fatalf("portas = %v, esperada só a 5432: o docker respondeu e não viu o container, "+
			"mas a listagem cega não autoriza apagar instância de host", portas)
	}
}

func TestGravarSondaPostgresSoPodaOEscopoDoHostQuandoODockerFalha(t *testing.T) {
	srv := servidorDeTeste(t)

	agora := time.Now().UTC()
	cheia := sondaPostgresDeTeste(true, true, instanciaDeTeste(5432, false), instanciaDeTeste(5433, true))
	if err := gravarSondaPostgres(srv.ID, cheia, agora); err != nil {
		t.Fatalf("primeira sonda: %v", err)
	}

	soListagem := sondaPostgresDeTeste(true, false)
	if err := gravarSondaPostgres(srv.ID, soListagem, agora.Add(time.Second)); err != nil {
		t.Fatalf("segunda sonda: %v", err)
	}

	portas := portasGravadas(t, srv.ID)
	if len(portas) != 1 || portas[0] != 5433 {
		t.Fatalf("portas = %v, esperada só a 5433: o docker cego não autoriza apagar instância em container", portas)
	}
}

func TestGravarSondaPostgresSemSudoNaListagemPreservaOPostgresDeHost(t *testing.T) {
	srv := servidorDeTeste(t)

	agora := time.Now().UTC()
	comSudo := sondaPostgresDeTeste(true, true, instanciaDeTeste(5432, false))
	if err := gravarSondaPostgres(srv.ID, comSudo, agora); err != nil {
		t.Fatalf("coleta com sudo: %v", err)
	}

	semSudo := sondaPostgresDeTeste(false, true)
	if err := gravarSondaPostgres(srv.ID, semSudo, agora.Add(time.Second)); err != nil {
		t.Fatalf("coleta sem sudo: %v", err)
	}

	linhas := instanciasGravadas(t, srv.ID)
	if len(linhas) != 1 || linhas[0].Porta != 5432 {
		t.Fatalf("instâncias = %+v, esperada a 5432 viva: desligar SSH_USE_SUDO cega o ss, não apaga o banco", linhas)
	}

	var bases []database.PostgresBase
	database.DB.Where("instancia_id = ?", linhas[0].ID).Find(&bases)
	if len(bases) != 1 || bases[0].Nome != "app" {
		t.Errorf("bases = %+v, esperada a base observada antes", bases)
	}
}

func TestGravarSondaPostgresSemAcessoPreservaAsBases(t *testing.T) {
	srv := servidorDeTeste(t)

	agora := time.Now().UTC()
	if err := gravarSondaPostgres(srv.ID, sondaPostgresDeTeste(true, true, instanciaDeTeste(5432, false)), agora); err != nil {
		t.Fatalf("primeira sonda: %v", err)
	}

	perdeuAcesso := sondaPostgresDeTeste(true, true, PostgresInstanciaPayload{
		Porta: 5432, Papel: "desconhecido", Estado: "sem_acesso", Motivo: "senha recusada",
	})
	if err := gravarSondaPostgres(srv.ID, perdeuAcesso, agora.Add(time.Second)); err != nil {
		t.Fatalf("segunda sonda: %v", err)
	}

	linhas := instanciasGravadas(t, srv.ID)
	if len(linhas) != 1 || linhas[0].Estado != estadoPgSemAcesso {
		t.Fatalf("instâncias = %+v, esperada a 5432 em sem_acesso", linhas)
	}

	var bases []database.PostgresBase
	database.DB.Where("instancia_id = ?", linhas[0].ID).Find(&bases)
	if len(bases) != 1 || bases[0].Nome != "app" {
		t.Errorf("bases = %+v, esperada a base observada antes: sonda cega não apaga inventário", bases)
	}
}

func TestGravarSondaPostgresAtualizaBaseEPodaAQueSumiu(t *testing.T) {
	srv := servidorDeTeste(t)

	tamanho := int64(1024)
	conexoes := 3
	agora := time.Now().UTC()
	cheia := sondaPostgresDeTeste(true, true, PostgresInstanciaPayload{
		Porta: 5432, Papel: "primario", Estado: "ativo",
		Bases: []PostgresBasePayload{
			{Nome: "app", Dono: "postgres", Encoding: "UTF8", TamanhoBytes: &tamanho, Conexoes: &conexoes},
			{Nome: "antiga", Dono: "postgres", Encoding: "UTF8"},
		},
	})
	if err := gravarSondaPostgres(srv.ID, cheia, agora); err != nil {
		t.Fatalf("primeira sonda: %v", err)
	}

	menor := sondaPostgresDeTeste(true, true, PostgresInstanciaPayload{
		Porta: 5432, Papel: "replica", Estado: "ativo",
		Bases: []PostgresBasePayload{{Nome: "app", Dono: "outro", Encoding: "UTF8"}},
	})
	if err := gravarSondaPostgres(srv.ID, menor, agora.Add(time.Second)); err != nil {
		t.Fatalf("segunda sonda: %v", err)
	}

	linhas := instanciasGravadas(t, srv.ID)
	if len(linhas) != 1 || linhas[0].Papel != papelReplica {
		t.Fatalf("instâncias = %+v, esperada a 5432 promovida a replica", linhas)
	}

	var bases []database.PostgresBase
	database.DB.Where("instancia_id = ?", linhas[0].ID).Find(&bases)
	if len(bases) != 1 || bases[0].Nome != "app" {
		t.Fatalf("bases = %+v, esperada só app", bases)
	}
	if bases[0].Dono != "outro" {
		t.Errorf("dono = %q, esperado outro: o upsert precisa atualizar", bases[0].Dono)
	}
	if bases[0].TamanhoBytes != nil {
		t.Errorf("tamanho = %v, esperado nulo: a coleta nova não mediu", *bases[0].TamanhoBytes)
	}
}

func TestGravarSondaPostgresNasceComMotorPostgres(t *testing.T) {
	srv := servidorDeTeste(t)

	agora := time.Now().UTC()
	sonda := sondaPostgresDeTeste(true, true, instanciaDeTeste(5432, false), instanciaDeTeste(5433, true))
	if err := gravarSondaPostgres(srv.ID, sonda, agora); err != nil {
		t.Fatalf("sonda: %v", err)
	}

	linhas := instanciasGravadas(t, srv.ID)
	if len(linhas) != 2 {
		t.Fatalf("instâncias = %d, esperado 2", len(linhas))
	}
	for _, linha := range linhas {
		if linha.Motor != motorPostgres {
			t.Errorf("motor da porta %d = %q, esperado %q", linha.Porta, linha.Motor, motorPostgres)
		}
	}
}

func soltarCheckDoMotor(t *testing.T) {
	t.Helper()

	if err := database.DB.Exec("ALTER TABLE postgres_instancias DROP CONSTRAINT chk_instancia_motor").Error; err != nil {
		t.Fatalf("soltar chk_instancia_motor: %v", err)
	}
	t.Cleanup(func() {
		database.DB.Exec("UPDATE postgres_instancias SET motor = ? WHERE motor NOT IN ?", motorPostgres, todosOsMotores)
		err := database.DB.Exec(
			"ALTER TABLE postgres_instancias ADD CONSTRAINT chk_instancia_motor CHECK (motor IN ('postgres', 'mysql', 'mariadb'))").Error
		if err != nil {
			t.Errorf("devolver chk_instancia_motor ao banco de teste: %v", err)
		}
	})
}

func TestGravarSondaPostgresAtualizaOMotorDaInstanciaPreexistente(t *testing.T) {
	srv := servidorDeTeste(t)

	agora := time.Now().UTC()
	if err := gravarSondaPostgres(srv.ID, sondaPostgresDeTeste(true, true, instanciaDeTeste(5432, false)), agora); err != nil {
		t.Fatalf("primeira sonda: %v", err)
	}

	antes := instanciasGravadas(t, srv.ID)
	if len(antes) != 1 {
		t.Fatalf("instâncias = %d, esperado 1", len(antes))
	}

	soltarCheckDoMotor(t)
	if err := database.DB.Exec("UPDATE postgres_instancias SET motor = ? WHERE id = ?", "legado", antes[0].ID).Error; err != nil {
		t.Fatalf("simular instância gravada antes da 017: %v", err)
	}

	segunda := sondaPostgresDeTeste(true, true, instanciaDeTeste(5432, false))
	if err := gravarSondaPostgres(srv.ID, segunda, agora.Add(time.Second)); err != nil {
		t.Fatalf("segunda sonda: %v", err)
	}

	depois := instanciasGravadas(t, srv.ID)
	if len(depois) != 1 || depois[0].ID != antes[0].ID {
		t.Fatalf("instâncias = %+v, esperada a mesma linha atualizada pelo upsert", depois)
	}
	if depois[0].Motor != motorPostgres {
		t.Errorf("motor = %q, esperado %q: motor precisa estar entre as colunas atualizadas no conflito",
			depois[0].Motor, motorPostgres)
	}
}

func TestIntervaloDaSondaPostgresRespeitaAVariavel(t *testing.T) {
	if got := intervaloDaSondaPostgres(); got != 15*time.Minute {
		t.Errorf("intervalo padrão = %s, esperado 15m", got)
	}

	t.Setenv("POSTGRES_PROBE_INTERVAL", "2m")
	if got := intervaloDaSondaPostgres(); got != 2*time.Minute {
		t.Errorf("intervalo = %s, esperado 2m", got)
	}

	t.Setenv("POSTGRES_PROBE_INTERVAL", "meia hora")
	if got := intervaloDaSondaPostgres(); got != 15*time.Minute {
		t.Errorf("intervalo com valor inválido = %s, esperado o padrão de 15m", got)
	}
}

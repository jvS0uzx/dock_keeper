package ssh

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"strings"
	"time"

	"gorm.io/gorm/clause"

	"github.com/jvS0uzx/dock_keeper/internal/config"
	"github.com/jvS0uzx/dock_keeper/internal/database"
	"github.com/jvS0uzx/dock_keeper/scripts"
)

const (
	intervaloPadraoDaSondaPostgres = 15 * time.Minute

	motorPostgres = "postgres"

	papelDesconhecido = "desconhecido"
	papelPrimario     = "primario"
	papelReplica      = "replica"

	estadoPgDesconhecido = "desconhecido"
	estadoPgAtivo        = "ativo"
	estadoPgInativo      = "inativo"
	estadoPgSemAcesso    = "sem_acesso"
)

type PostgresBasePayload struct {
	Nome         string `json:"nome"`
	Dono         string `json:"dono"`
	Encoding     string `json:"encoding"`
	TamanhoBytes *int64 `json:"tamanho_bytes"`
	Conexoes     *int   `json:"conexoes"`
}

type PostgresInstanciaPayload struct {
	Porta         int                   `json:"porta"`
	EmContainer   bool                  `json:"em_container"`
	ContainerNome string                `json:"container_nome"`
	Versao        string                `json:"versao"`
	Papel         string                `json:"papel"`
	WalLevel      string                `json:"wal_level"`
	MaxWalSenders *int                  `json:"max_wal_senders"`
	ArchiveMode   string                `json:"archive_mode"`
	Estado        string                `json:"estado"`
	Motivo        string                `json:"motivo"`
	Bases         []PostgresBasePayload `json:"bases"`
}

type PostgresProbePayload struct {
	DescobertaHostOk      bool                       `json:"descoberta_host_ok"`
	DescobertaContainerOk bool                       `json:"descoberta_container_ok"`
	Instancias            []PostgresInstanciaPayload `json:"instancias"`
}

func intervaloDaSondaPostgres() time.Duration {
	return config.Duracao("POSTGRES_PROBE_INTERVAL", intervaloPadraoDaSondaPostgres)
}

func truncarTexto(s string, limite int) string {
	runas := []rune(s)
	if len(runas) <= limite {
		return s
	}
	return string(runas[:limite])
}

func normalizarPapel(bruto string) string {
	switch strings.ToLower(strings.TrimSpace(bruto)) {
	case papelPrimario:
		return papelPrimario
	case papelReplica:
		return papelReplica
	default:
		return papelDesconhecido
	}
}

func normalizarEstadoPostgres(bruto string) string {
	switch strings.ToLower(strings.TrimSpace(bruto)) {
	case estadoPgAtivo:
		return estadoPgAtivo
	case estadoPgInativo:
		return estadoPgInativo
	case estadoPgSemAcesso:
		return estadoPgSemAcesso
	default:
		return estadoPgDesconhecido
	}
}

func normalizarBases(bases []PostgresBasePayload) []PostgresBasePayload {
	limpas := make([]PostgresBasePayload, 0, len(bases))
	vistas := make(map[string]bool, len(bases))
	for _, b := range bases {
		nome := truncarTexto(strings.TrimSpace(b.Nome), 128)
		if nome == "" || vistas[nome] {
			continue
		}
		vistas[nome] = true
		limpas = append(limpas, PostgresBasePayload{
			Nome:         nome,
			Dono:         truncarTexto(strings.TrimSpace(b.Dono), 128),
			Encoding:     truncarTexto(strings.TrimSpace(b.Encoding), 32),
			TamanhoBytes: b.TamanhoBytes,
			Conexoes:     b.Conexoes,
		})
	}
	return limpas
}

func normalizarInstancia(inst PostgresInstanciaPayload) PostgresInstanciaPayload {
	limpa := PostgresInstanciaPayload{
		Porta:         inst.Porta,
		EmContainer:   inst.EmContainer,
		ContainerNome: truncarTexto(strings.TrimSpace(inst.ContainerNome), 128),
		Versao:        truncarTexto(strings.TrimSpace(inst.Versao), 32),
		Papel:         normalizarPapel(inst.Papel),
		WalLevel:      truncarTexto(strings.TrimSpace(inst.WalLevel), 16),
		MaxWalSenders: inst.MaxWalSenders,
		ArchiveMode:   truncarTexto(strings.TrimSpace(inst.ArchiveMode), 16),
		Estado:        normalizarEstadoPostgres(inst.Estado),
		Motivo:        strings.TrimSpace(inst.Motivo),
		Bases:         normalizarBases(inst.Bases),
	}
	if limpa.Estado == estadoPgAtivo {
		limpa.Motivo = ""
	}
	return limpa
}

func normalizarSondaPostgres(p PostgresProbePayload) PostgresProbePayload {
	limpas := make([]PostgresInstanciaPayload, 0, len(p.Instancias))
	vistas := make(map[int]bool, len(p.Instancias))
	for _, inst := range p.Instancias {
		if inst.Porta <= 0 || vistas[inst.Porta] {
			continue
		}
		vistas[inst.Porta] = true
		limpas = append(limpas, normalizarInstancia(inst))
	}
	return PostgresProbePayload{
		DescobertaHostOk:      p.DescobertaHostOk,
		DescobertaContainerOk: p.DescobertaContainerOk,
		Instancias:            limpas,
	}
}

func gravarInstanciaPostgres(serverID string, inst PostgresInstanciaPayload, agora time.Time) (uint, error) {
	linha := database.PostgresInstancia{
		ServerID:      serverID,
		Porta:         inst.Porta,
		EmContainer:   inst.EmContainer,
		ContainerNome: inst.ContainerNome,
		Motor:         motorPostgres,
		Versao:        inst.Versao,
		Papel:         inst.Papel,
		WalLevel:      inst.WalLevel,
		MaxWalSenders: inst.MaxWalSenders,
		ArchiveMode:   inst.ArchiveMode,
		Estado:        inst.Estado,
		Motivo:        inst.Motivo,
		ObservadoEm:   agora,
	}

	err := database.DB.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "server_id"}, {Name: "porta"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"em_container", "container_nome", "motor", "versao", "papel", "wal_level",
			"max_wal_senders", "archive_mode", "estado", "motivo", "observado_em", "updated_at",
		}),
	}).Create(&linha).Error
	if err != nil {
		return 0, err
	}
	if linha.ID != 0 {
		return linha.ID, nil
	}

	var existente database.PostgresInstancia
	err = database.DB.Where("server_id = ? AND porta = ?", serverID, inst.Porta).Take(&existente).Error
	if err != nil {
		return 0, err
	}
	return existente.ID, nil
}

func gravarBasesPostgres(instanciaID uint, bases []PostgresBasePayload, agora time.Time) error {
	if len(bases) == 0 {
		return nil
	}

	linhas := make([]database.PostgresBase, 0, len(bases))
	for _, b := range bases {
		linhas = append(linhas, database.PostgresBase{
			InstanciaID:  instanciaID,
			Nome:         b.Nome,
			Dono:         b.Dono,
			Encoding:     b.Encoding,
			TamanhoBytes: b.TamanhoBytes,
			Conexoes:     b.Conexoes,
			ObservadoEm:  agora,
		})
	}

	return database.DB.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "instancia_id"}, {Name: "nome"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"dono", "encoding", "tamanho_bytes", "conexoes", "observado_em", "updated_at",
		}),
	}).Create(&linhas).Error
}

func gravarSondaPostgres(serverID string, p PostgresProbePayload, agora time.Time) error {
	if database.DB == nil {
		return errors.New("banco indisponível para gravar a sonda do postgres")
	}

	limpo := normalizarSondaPostgres(p)
	for _, inst := range limpo.Instancias {
		id, err := gravarInstanciaPostgres(serverID, inst, agora)
		if err != nil {
			return err
		}
		if err := gravarBasesPostgres(id, inst.Bases, agora); err != nil {
			return err
		}
		if inst.Estado != estadoPgAtivo {
			continue
		}
		err = database.DB.Where("instancia_id = ? AND observado_em < ?", id, agora).
			Delete(&database.PostgresBase{}).Error
		if err != nil {
			return err
		}
	}

	return podarInstanciasPostgres(serverID, limpo, agora)
}

func podarInstanciasPostgres(serverID string, p PostgresProbePayload, agora time.Time) error {
	if p.DescobertaHostOk {
		err := database.DB.
			Where("server_id = ? AND em_container = ? AND observado_em < ?", serverID, false, agora).
			Delete(&database.PostgresInstancia{}).Error
		if err != nil {
			return err
		}
	}

	if p.DescobertaContainerOk {
		err := database.DB.
			Where("server_id = ? AND em_container = ? AND observado_em < ?", serverID, true, agora).
			Delete(&database.PostgresInstancia{}).Error
		if err != nil {
			return err
		}
	}

	return nil
}

func SondarPostgres(ctx context.Context, t Target) (PostgresProbePayload, error) {
	var vazio PostgresProbePayload

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
	if err := runScript(session, t, scripts.ProbePostgres); err != nil {
		return vazio, err
	}

	payload, lido := vazio, false
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		linha := bytes.TrimSpace(scanner.Bytes())
		if len(linha) == 0 || linha[0] != '{' {
			continue
		}
		var candidata PostgresProbePayload
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
		return vazio, errors.New("sonda do postgres não devolveu JSON")
	}
	return payload, nil
}

func executarSondaPostgres(ctx context.Context, t Target) {
	payload, err := SondarPostgres(ctx, t)
	if err != nil {
		if ctx.Err() == nil {
			log.Printf("[Postgres] sonda de %s falhou: %v", t.Host, err)
		}
		return
	}

	agora := time.Now().UTC()
	if err := gravarSondaPostgres(t.ID, payload, agora); err != nil {
		log.Printf("[Postgres] erro ao gravar a sonda de %s: %v", t.Host, err)
		return
	}

	if !payload.DescobertaHostOk {
		log.Printf("[Postgres] %s: a listagem de portas não respondeu ou não mostra o dono do socket; "+
			"instâncias de host preservadas", t.Host)
	}
	if !payload.DescobertaContainerOk {
		log.Printf("[Postgres] %s: docker ps não respondeu; instâncias em container preservadas", t.Host)
	}

	for _, inst := range normalizarSondaPostgres(payload).Instancias {
		if inst.Estado == estadoPgAtivo {
			continue
		}
		log.Printf("[Postgres] %s porta %d classificada como %s: %s",
			t.Host, inst.Porta, inst.Estado, inst.Motivo)
	}
	log.Printf("[Postgres] %s: %d instância(s) inventariada(s)", t.Host, len(payload.Instancias))
}

func SondarPostgresPeriodicamente(ctx context.Context, t Target) {
	for {
		executarSondaPostgres(ctx, t)

		select {
		case <-ctx.Done():
			return
		case <-time.After(intervaloDaSondaPostgres()):
		}
	}
}

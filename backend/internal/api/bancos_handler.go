package api

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/database"
)

type BancoBaseView struct {
	Nome         string    `json:"nome"`
	Dono         string    `json:"dono"`
	Encoding     string    `json:"encoding"`
	TamanhoBytes *int64    `json:"tamanho_bytes"`
	Conexoes     *int      `json:"conexoes"`
	ObservadoEm  time.Time `json:"observado_em"`
}

type BancoInstanciaView struct {
	ID                uint            `json:"id"`
	ServerID          string          `json:"server_id"`
	ServidorNome      string          `json:"servidor_nome"`
	SiteID            *uint           `json:"site_id"`
	Porta             int             `json:"porta"`
	EmContainer       bool            `json:"em_container"`
	ContainerNome     string          `json:"container_nome"`
	Motor             string          `json:"motor"`
	Versao            string          `json:"versao"`
	Papel             string          `json:"papel"`
	WalLevel          string          `json:"wal_level"`
	MaxWalSenders     *int            `json:"max_wal_senders"`
	ArchiveMode       string          `json:"archive_mode"`
	Estado            string          `json:"estado"`
	Motivo            string          `json:"motivo"`
	ObservadoEm       time.Time       `json:"observado_em"`
	TotalBases        int             `json:"total_bases"`
	TamanhoTotalBytes *int64          `json:"tamanho_total_bytes"`
	Bases             []BancoBaseView `json:"bases"`
}

type bancoInstanciaRow struct {
	ID            uint
	ServerID      string
	ServidorNome  string
	SiteID        *uint
	Porta         int
	EmContainer   bool
	ContainerNome string
	Motor         string
	Versao        string
	Papel         string
	WalLevel      string
	MaxWalSenders *int
	ArchiveMode   string
	Estado        string
	Motivo        string
	ObservadoEm   time.Time
}

const colunasDaInstancia = `postgres_instancias.id,
	postgres_instancias.server_id,
	servers.name AS servidor_nome,
	servers.site_id AS site_id,
	postgres_instancias.porta,
	postgres_instancias.em_container,
	postgres_instancias.container_nome,
	postgres_instancias.motor,
	postgres_instancias.versao,
	postgres_instancias.papel,
	postgres_instancias.wal_level,
	postgres_instancias.max_wal_senders,
	postgres_instancias.archive_mode,
	postgres_instancias.estado,
	postgres_instancias.motivo,
	postgres_instancias.observado_em`

const juncaoComServidor = "JOIN servers ON servers.id = postgres_instancias.server_id AND servers.deleted_at IS NULL"

func bancosHandler(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)
	scope, status := resolveScope(sess, r)
	if status != 0 {
		writeError(w, status, "site_id inválido ou fora do seu alcance")
		return
	}

	linhas, err := instanciasNoEscopo(r.Context(), scope)
	if err != nil {
		log.Printf("[API] erro ao listar instâncias de banco: %v", err)
		writeError(w, http.StatusInternalServerError, "falha ao ler o inventário de bancos")
		return
	}

	ids := make([]uint, 0, len(linhas))
	for _, l := range linhas {
		ids = append(ids, l.ID)
	}

	porInstancia, err := basesPorInstancia(r.Context(), ids)
	if err != nil {
		log.Printf("[API] erro ao listar bases de dados: %v", err)
		writeError(w, http.StatusInternalServerError, "falha ao ler as bases de dados")
		return
	}

	saida := make([]BancoInstanciaView, 0, len(linhas))
	for _, l := range linhas {
		saida = append(saida, montarInstancia(l, porInstancia[l.ID]))
	}
	writeJSON(w, http.StatusOK, saida)
}

func instanciasNoEscopo(ctx context.Context, scope siteScope) ([]bancoInstanciaRow, error) {
	tx := database.From(ctx).
		Model(&database.PostgresInstancia{}).
		Joins(juncaoComServidor)

	var linhas []bancoInstanciaRow
	err := scope.apply(tx).
		Select(colunasDaInstancia).
		Order("servers.name ASC, postgres_instancias.porta ASC").
		Scan(&linhas).Error
	return linhas, err
}

func basesPorInstancia(ctx context.Context, ids []uint) (map[uint][]database.PostgresBase, error) {
	agrupadas := make(map[uint][]database.PostgresBase, len(ids))
	if len(ids) == 0 {
		return agrupadas, nil
	}

	var bases []database.PostgresBase
	if err := database.From(ctx).
		Where("instancia_id IN ?", ids).
		Order("instancia_id ASC, nome ASC").
		Find(&bases).Error; err != nil {
		return nil, err
	}

	for _, b := range bases {
		agrupadas[b.InstanciaID] = append(agrupadas[b.InstanciaID], b)
	}
	return agrupadas, nil
}

func montarInstancia(l bancoInstanciaRow, bases []database.PostgresBase) BancoInstanciaView {
	view := BancoInstanciaView{
		ID:            l.ID,
		ServerID:      l.ServerID,
		ServidorNome:  l.ServidorNome,
		SiteID:        l.SiteID,
		Porta:         l.Porta,
		EmContainer:   l.EmContainer,
		ContainerNome: l.ContainerNome,
		Motor:         l.Motor,
		Versao:        l.Versao,
		Papel:         l.Papel,
		WalLevel:      l.WalLevel,
		MaxWalSenders: l.MaxWalSenders,
		ArchiveMode:   l.ArchiveMode,
		Estado:        l.Estado,
		Motivo:        l.Motivo,
		ObservadoEm:   l.ObservadoEm,
		TotalBases:    len(bases),
		Bases:         make([]BancoBaseView, 0, len(bases)),
	}

	for _, b := range bases {
		view.Bases = append(view.Bases, BancoBaseView{
			Nome:         b.Nome,
			Dono:         b.Dono,
			Encoding:     b.Encoding,
			TamanhoBytes: b.TamanhoBytes,
			Conexoes:     b.Conexoes,
			ObservadoEm:  b.ObservadoEm,
		})
		if b.TamanhoBytes == nil {
			continue
		}
		if view.TamanhoTotalBytes == nil {
			soma := int64(0)
			view.TamanhoTotalBytes = &soma
		}
		*view.TamanhoTotalBytes += *b.TamanhoBytes
	}
	return view
}

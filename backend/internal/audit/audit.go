// Package audit grava o rastro atribuível das escritas do painel.
//
// Mora fora de internal/api porque internal/ssh também precisa escrever, e
// internal/api já importa internal/ssh — um pacote de auditoria dentro do HTTP
// fecharia o ciclo. Aqui os dois lados o importam e ele não importa nenhum.
package audit

import (
	"encoding/json"
	"log"
	"time"

	"github.com/joaov/vd_stats/internal/database"
)

// Resultados possíveis de uma ação auditada.
const (
	ResultOK     = "ok"
	ResultDenied = "denied"
	ResultError  = "error"

	// ResultPending é o estado da linha gravada ANTES da execução, para o caso
	// que mais importa: o comando que travou a máquina e nunca retornou. Quem
	// grava só depois perde exatamente esse.
	ResultPending = "pending"
)

// Entry é uma ação a registrar. Os campos de ator são preenchidos pelo chamador
// a partir da sessão; internal/ssh, que não tem sessão, deixa-os vazios e o
// chamador HTTP os propaga quando a ação nasceu de uma requisição.
type Entry struct {
	ActorUserID   *uint
	ActorUsername string
	ActorRole     string

	SourceIP  string
	UserAgent string

	Action      string
	TargetType  string
	TargetID    string
	TargetLabel string

	SiteID *uint
	Result string

	// Detail é montado por allowlist: só os campos que o chamador escolheu
	// nomear. Nunca receba aqui o corpo da requisição inteiro.
	Detail map[string]any
}

// Record grava a linha e devolve o id, ou 0 quando a gravação falha.
//
// Falha de auditoria não derruba a requisição: o painel é a ferramenta de quem
// está apagando incêndio, e recusar a operação por causa de um soluço no banco
// tira a ferramenta justamente na hora em que ela importa. A contrapartida é
// que a falha precisa gritar no log — auditoria que falha em silêncio é pior
// que auditoria nenhuma, porque a ausência da linha passa a significar duas
// coisas diferentes.
func Record(e Entry) uint {
	row := database.AuditLog{
		At:            time.Now().UTC(),
		ActorUserID:   e.ActorUserID,
		ActorUsername: e.ActorUsername,
		ActorRole:     e.ActorRole,
		SourceIP:      e.SourceIP,
		UserAgent:     truncate(e.UserAgent, 255),
		Action:        e.Action,
		TargetType:    e.TargetType,
		TargetID:      e.TargetID,
		TargetLabel:   truncate(e.TargetLabel, 255),
		SiteID:        e.SiteID,
		Result:        e.Result,
		Detail:        encodeDetail(e.Detail),
	}
	if row.Result == "" {
		row.Result = ResultPending
	}
	if database.DB == nil {
		log.Printf("[Auditoria] AVISO: banco indisponível, ação %q de %q não foi registrada",
			row.Action, row.ActorUsername)
		return 0
	}
	if err := database.DB.Create(&row).Error; err != nil {
		log.Printf("[Auditoria] AVISO: ação %q de %q não foi registrada: %v",
			row.Action, row.ActorUsername, err)
		return 0
	}
	return row.ID
}

// Complete fecha a linha pendente que Record abriu. Detail é mesclado ao que já
// estava gravado, não substituído: o que se sabia antes da execução (o alvo, os
// argumentos) continua valendo depois dela.
func Complete(id uint, result string, detail map[string]any) {
	if id == 0 || database.DB == nil {
		return
	}

	updates := map[string]any{"result": result}
	if len(detail) > 0 {
		var row database.AuditLog
		if err := database.DB.Select("detail").First(&row, id).Error; err == nil {
			updates["detail"] = encodeDetail(merge(decodeDetail(row.Detail), detail))
		} else {
			updates["detail"] = encodeDetail(detail)
		}
	}

	if err := database.DB.Model(&database.AuditLog{}).Where("id = ?", id).
		Updates(updates).Error; err != nil {
		log.Printf("[Auditoria] AVISO: resultado da ação %d não foi registrado: %v", id, err)
	}
}

// encodeDetail sempre devolve JSON válido: a coluna é jsonb e string vazia não
// é documento, então o INSERT inteiro falharia por causa de um campo acessório.
func encodeDetail(detail map[string]any) string {
	if len(detail) == 0 {
		return "{}"
	}
	b, err := json.Marshal(detail)
	if err != nil {
		log.Printf("[Auditoria] AVISO: detalhe descartado, não serializa: %v", err)
		return "{}"
	}
	return string(b)
}

func decodeDetail(raw string) map[string]any {
	if raw == "" {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return nil
	}
	return m
}

func merge(base, novo map[string]any) map[string]any {
	if base == nil {
		base = make(map[string]any, len(novo))
	}
	for k, v := range novo {
		base[k] = v
	}
	return base
}

// truncate corta no limite da coluna. Um User-Agent absurdo não pode fazer o
// INSERT falhar e levar embora o registro da ação junto.
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}

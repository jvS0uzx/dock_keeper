package audit

import (
	"encoding/json"
	"log"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/database"
)

const (
	ResultOK     = "ok"
	ResultDenied = "denied"
	ResultError  = "error"

	ResultPending = "pending"
)

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

	Detail map[string]any
}

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

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}

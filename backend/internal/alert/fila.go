package alert

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/jvS0uzx/dock_keeper/internal/config"
	"github.com/jvS0uzx/dock_keeper/internal/database"
	"github.com/jvS0uzx/dock_keeper/internal/observabilidade"
	"github.com/jvS0uzx/dock_keeper/internal/safego"
)

const (
	defaultMaxAttempts  = 8
	defaultBackoffBase  = time.Minute
	defaultBackoffMax   = time.Hour
	defaultDespachoLote = 10
)

var (
	despachoIntervalo = 5 * time.Second

	retomadaPadraoHoras = 24
	despachoLease       = time.Minute
	backoffBase         = defaultBackoffBase
	backoffMax          = defaultBackoffMax
)

type Entrada struct {
	Key      string
	Text     string
	Severity string
	ServerID *string
	SiteID   *uint
	RuleID   *uint
}

func maxAttempts() int {
	return config.Inteiro("ALERT_MAX_ATTEMPTS", defaultMaxAttempts)
}

func RecoveryEnabled() bool {
	return config.Booleano("ALERT_NOTIFY_RECOVERY", true)
}

func severityFromText(texto string) string {
	switch {
	case strings.HasPrefix(texto, "[CRITICO]"):
		return "critical"
	case strings.HasPrefix(texto, "[ALERTA]"):
		return "warning"
	default:
		return "info"
	}
}

func Notify(key, msg string) bool {
	return Enqueue(Entrada{Key: key, Text: msg, Severity: severityFromText(msg)})
}

func Enqueue(e Entrada) bool {
	e.Key = strings.TrimSpace(e.Key)
	e.Text = strings.TrimSpace(e.Text)
	if e.Key == "" || e.Text == "" {
		return false
	}
	if e.Severity == "" {
		e.Severity = severityFromText(e.Text)
	}

	if database.DB == nil {
		if !claimSlot(e.Key) {
			return false
		}
		Send(e.Text)
		return true
	}

	now := agora().UTC()
	if suppressed(e.Key, now) {
		return false
	}

	alerta := database.Alert{
		Key: e.Key, Severity: e.Severity, Text: e.Text,
		Status: database.AlertStatusOpen, ServerID: e.ServerID, SiteID: e.SiteID, RuleID: e.RuleID,
		CreatedAt: now, Delivery: database.AlertDeliveryPendente, NextAttemptAt: &now,
	}
	if err := database.DB.Create(&alerta).Error; err != nil {
		observabilidade.AlertasDescartados.Add(1)
		log.Printf("[Alert] erro ao enfileirar %q, alerta descartado para não segurar quem detectou: %v | %s",
			e.Key, err, e.Text)
		return false
	}

	observabilidade.AlertasEnfileirados.Add(1)
	log.Printf("[Alert] enfileirado (#%d): %s", alerta.ID, e.Text)
	return true
}

func suppressed(key string, now time.Time) bool {
	var pendentes int64
	if err := database.DB.Model(&database.Alert{}).
		Where("key = ? AND delivery IN ?", key,
			[]string{database.AlertDeliveryPendente, database.AlertDeliverySemCanal}).
		Count(&pendentes).Error; err == nil && pendentes > 0 {
		return true
	}

	var ultimo database.Alert
	err := database.DB.Where("key = ? AND delivery = ?", key, database.AlertDeliveryEnviado).
		Order("last_attempt_at desc").Take(&ultimo).Error
	if err != nil || ultimo.LastAttemptAt == nil {
		return false
	}
	return now.Sub(ultimo.LastAttemptAt.UTC()) < cooldown
}

func Recovered(e Entrada) bool {
	if database.DB == nil {
		return false
	}

	now := agora().UTC()
	err := database.DB.Model(&database.Alert{}).
		Where("key = ? AND status <> ?", e.Key, database.AlertStatusResolved).
		Updates(map[string]any{"status": database.AlertStatusResolved, "resolved_at": now}).Error
	if err != nil {
		log.Printf("[Alert] erro ao resolver %q: %v", e.Key, err)
	}
	if !RecoveryEnabled() {
		return false
	}

	e.Key += ":recuperacao"
	e.Severity = "info"
	return Enqueue(e)
}

func StartDispatcher(ctx context.Context) <-chan struct{} {
	return safego.Run(ctx, "alert:despacho", func(ctx context.Context) {
		ticker := time.NewTicker(despachoIntervalo)
		defer ticker.Stop()
		for {
			dispatchPending(agora().UTC())
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	})
}

func dispatchPending(now time.Time) int {
	if database.DB == nil {
		return 0
	}

	retomarSemCanal(now)

	claimed := claimBatch(now)
	for _, alerta := range claimed {
		err := Deliver(alerta.Text)
		switch {
		case errors.Is(err, ErrSemCanal):
			registerSemCanal(alerta, agora().UTC())
		case err != nil:
			registerFailure(alerta, err, agora().UTC())
		default:
			registerSuccess(alerta, agora().UTC())
		}
	}
	return len(claimed)
}

func janelaDeRetomada() time.Duration {
	return time.Duration(config.Inteiro("ALERT_RESUME_HOURS", retomadaPadraoHoras)) * time.Hour
}

func retomarSemCanal(now time.Time) {
	if Status().Estado == EstadoDesligado {
		return
	}

	corte := now.Add(-janelaDeRetomada())
	res := database.DB.Model(&database.Alert{}).
		Where("delivery = ? AND status = ? AND created_at >= ?",
			database.AlertDeliverySemCanal, database.AlertStatusOpen, corte).
		Updates(map[string]any{
			"delivery":        database.AlertDeliveryPendente,
			"next_attempt_at": nil,
			"last_error":      "",
		})
	if res.Error != nil {
		log.Printf("[Alert] erro ao retomar alertas sem canal: %v", res.Error)
		return
	}
	if res.RowsAffected > 0 {
		observabilidade.AlertasSemCanal.Add(-res.RowsAffected)
		log.Printf("[Alert] canal de volta: %d alerta(s) preso(s) voltaram para a fila", res.RowsAffected)
	}
}

func registerSemCanal(alerta database.Alert, now time.Time) {
	err := database.DB.Model(&database.Alert{}).Where("id = ?", alerta.ID).
		Updates(map[string]any{
			"delivery":        database.AlertDeliverySemCanal,
			"last_attempt_at": now,
			"next_attempt_at": nil,
			"last_error":      ErrSemCanal.Error(),
		}).Error
	if err != nil {
		log.Printf("[Alert] erro ao marcar alerta %d como sem canal: %v", alerta.ID, err)
		return
	}
	observabilidade.AlertasSemCanal.Add(1)
}

func claimBatch(now time.Time) []database.Alert {
	var lote []database.Alert

	err := database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("delivery = ? AND (next_attempt_at IS NULL OR next_attempt_at <= ?)",
				database.AlertDeliveryPendente, now).
			Order("created_at asc, id asc").
			Limit(defaultDespachoLote).
			Find(&lote).Error; err != nil {
			return err
		}
		if len(lote) == 0 {
			return nil
		}

		ids := make([]uint, 0, len(lote))
		for _, a := range lote {
			ids = append(ids, a.ID)
		}
		return tx.Model(&database.Alert{}).Where("id IN ?", ids).
			Updates(map[string]any{
				"last_attempt_at": now,
				"next_attempt_at": now.Add(despachoLease),
			}).Error
	})
	if err != nil {
		log.Printf("[Alert] erro ao reservar alertas pendentes: %v", err)
		return nil
	}
	return lote
}

func registerSuccess(alerta database.Alert, now time.Time) {
	err := database.DB.Model(&database.Alert{}).Where("id = ?", alerta.ID).
		Updates(map[string]any{
			"delivery":        database.AlertDeliveryEnviado,
			"attempts":        alerta.Attempts + 1,
			"last_attempt_at": now,
			"next_attempt_at": nil,
			"last_error":      "",
		}).Error
	if err != nil {
		log.Printf("[Alert] erro ao marcar o alerta %d como enviado: %v", alerta.ID, err)
		return
	}
	observabilidade.AlertasEntregues.Add(1)
}

func registerFailure(alerta database.Alert, falha error, now time.Time) {
	tentativas := alerta.Attempts + 1
	mudanca := map[string]any{
		"attempts":        tentativas,
		"last_attempt_at": now,
		"last_error":      falha.Error(),
	}

	if tentativas >= maxAttempts() {
		mudanca["delivery"] = database.AlertDeliveryFalhou
		mudanca["next_attempt_at"] = nil
		observabilidade.AlertasFalhos.Add(1)
		log.Printf("[Alert] alerta %d desistiu depois de %d tentativas e segue aberto no painel: %v",
			alerta.ID, tentativas, falha)
	} else {
		proxima := now.Add(retryDelay(tentativas))
		mudanca["next_attempt_at"] = proxima
		log.Printf("[Alert] alerta %d falhou na tentativa %d, nova tentativa em %s: %v",
			alerta.ID, tentativas, retryDelay(tentativas), falha)
	}

	if err := database.DB.Model(&database.Alert{}).Where("id = ?", alerta.ID).
		Updates(mudanca).Error; err != nil {
		log.Printf("[Alert] erro ao registrar a falha do alerta %d: %v", alerta.ID, err)
	}
}

func retryDelay(tentativas int) time.Duration {
	espera := backoffBase
	for i := 1; i < tentativas; i++ {
		espera *= 2
		if espera >= backoffMax {
			return backoffMax
		}
	}
	return espera
}

package alert

import (
	"errors"
	"log"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/jvS0uzx/dock_keeper/internal/database"
	"github.com/jvS0uzx/dock_keeper/internal/observabilidade"
)

func entregarAlerta(alerta database.Alert, ativos []Canal, now time.Time) {
	if len(ativos) == 0 {
		registerSemCanal(alerta, now)
		return
	}

	linhas, err := garantirLinhas(alerta, ativos, now)
	if err != nil {
		log.Printf("[Alert] erro ao preparar a entrega do alerta %d: %v", alerta.ID, err)
		return
	}

	alerta.Text = textoDoAlerta(alerta)
	entregouAgora := false

	for _, c := range ativos {
		linha, existe := linhas[c.Nome()]
		if !existe || !linhaDevida(linha, now) {
			continue
		}
		falha := c.Entregar(alerta)
		if falha == nil {
			entregouAgora = true
		}
		linhas[c.Nome()] = registrarResultado(linha, falha, now)
	}

	consolidar(alerta, linhas, ativos, entregouAgora, now)
}

func linhaDevida(linha database.AlertDelivery, now time.Time) bool {
	if linha.Status != database.AlertDeliveryPendente {
		return false
	}
	return linha.NextAttemptAt == nil || !linha.NextAttemptAt.After(now)
}

func garantirLinhas(alerta database.Alert, ativos []Canal, now time.Time) (map[string]database.AlertDelivery, error) {
	novas := make([]database.AlertDelivery, 0, len(ativos))
	for _, c := range ativos {
		novas = append(novas, database.AlertDelivery{
			AlertID:   alerta.ID,
			Canal:     c.Nome(),
			Status:    database.AlertDeliveryPendente,
			CreatedAt: now,
			UpdatedAt: now,
		})
	}

	err := database.DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "alert_id"}, {Name: "canal"}},
		DoNothing: true,
	}).Create(&novas).Error
	if err != nil {
		return nil, err
	}

	var linhas []database.AlertDelivery
	if err := database.DB.Where("alert_id = ?", alerta.ID).Find(&linhas).Error; err != nil {
		return nil, err
	}

	porCanal := make(map[string]database.AlertDelivery, len(linhas))
	for _, l := range linhas {
		porCanal[l.Canal] = l
	}
	return porCanal, nil
}

func registrarResultado(linha database.AlertDelivery, falha error, now time.Time) database.AlertDelivery {
	linha.LastAttemptAt = &now
	linha.UpdatedAt = now

	switch {
	case errors.Is(falha, ErrSemCanal):
		linha.Status = database.AlertDeliverySemCanal
		linha.NextAttemptAt = nil
		linha.LastError = falha.Error()

	case falha != nil:
		linha.Attempts++
		linha.LastError = falha.Error()
		if linha.Attempts >= maxAttempts() {
			linha.Status = database.AlertDeliveryFalhou
			linha.NextAttemptAt = nil
			log.Printf("[Alert] alerta %d desistiu no canal %s depois de %d tentativas e segue aberto no painel: %v",
				linha.AlertID, linha.Canal, linha.Attempts, falha)
		} else {
			espera := retryDelay(linha.Attempts)
			proxima := now.Add(espera)
			linha.Status = database.AlertDeliveryPendente
			linha.NextAttemptAt = &proxima
			log.Printf("[Alert] alerta %d falhou no canal %s na tentativa %d, nova tentativa em %s: %v",
				linha.AlertID, linha.Canal, linha.Attempts, espera, falha)
		}

	default:
		linha.Attempts++
		linha.Status = database.AlertDeliveryEnviado
		linha.NextAttemptAt = nil
		linha.LastError = ""
	}

	err := database.DB.Model(&database.AlertDelivery{}).Where("id = ?", linha.ID).
		Updates(map[string]any{
			"status":          linha.Status,
			"attempts":        linha.Attempts,
			"next_attempt_at": linha.NextAttemptAt,
			"last_attempt_at": linha.LastAttemptAt,
			"last_error":      linha.LastError,
			"updated_at":      now,
		}).Error
	if err != nil {
		log.Printf("[Alert] erro ao registrar a entrega do alerta %d pelo canal %s: %v",
			linha.AlertID, linha.Canal, err)
	}
	return linha
}

func consolidar(alerta database.Alert, linhas map[string]database.AlertDelivery, ativos []Canal, entregouAgora bool, now time.Time) {
	tentativas := 0
	ultimoErro := ""
	var proxima *time.Time
	var ultimaTentativa *time.Time
	pendente, falhou, semCanal := false, false, false

	for _, c := range ativos {
		linha, existe := linhas[c.Nome()]
		if !existe {
			continue
		}

		if linha.Attempts > tentativas {
			tentativas = linha.Attempts
		}
		if linha.LastAttemptAt != nil && (ultimaTentativa == nil || linha.LastAttemptAt.After(*ultimaTentativa)) {
			ultimaTentativa = linha.LastAttemptAt
		}
		if linha.LastError != "" && ultimoErro == "" {
			ultimoErro = linha.LastError
		}

		switch linha.Status {
		case database.AlertDeliveryPendente:
			pendente = true
			if linha.NextAttemptAt != nil && (proxima == nil || linha.NextAttemptAt.Before(*proxima)) {
				proxima = linha.NextAttemptAt
			}
		case database.AlertDeliveryFalhou:
			falhou = true
		case database.AlertDeliverySemCanal:
			semCanal = true
		}
	}

	consolidado := database.AlertDeliveryEnviado
	switch {
	case pendente:
		consolidado = database.AlertDeliveryPendente
		if proxima == nil {
			proxima = &now
		}
	case semCanal:
		consolidado = database.AlertDeliverySemCanal
	case falhou:
		consolidado = database.AlertDeliveryFalhou
	}

	mudanca := map[string]any{
		"delivery":        consolidado,
		"attempts":        tentativas,
		"last_error":      ultimoErro,
		"next_attempt_at": proxima,
	}
	if ultimaTentativa != nil {
		mudanca["last_attempt_at"] = *ultimaTentativa
	}
	if entregouAgora {
		mudanca["last_notified_at"] = now
	}

	if err := database.DB.Model(&database.Alert{}).Where("id = ?", alerta.ID).Updates(mudanca).Error; err != nil {
		log.Printf("[Alert] erro ao consolidar a entrega do alerta %d: %v", alerta.ID, err)
		return
	}

	switch {
	case consolidado == database.AlertDeliveryEnviado && alerta.Delivery != database.AlertDeliveryEnviado:
		observabilidade.AlertasEntregues.Add(1)
	case consolidado == database.AlertDeliveryFalhou && alerta.Delivery != database.AlertDeliveryFalhou:
		observabilidade.AlertasFalhos.Add(1)
	case consolidado == database.AlertDeliverySemCanal && alerta.Delivery != database.AlertDeliverySemCanal:
		observabilidade.AlertasSemCanal.Add(1)
	}
}

func idsDeAlertas(consulta *gorm.DB) []uint {
	var ids []uint
	if err := consulta.Pluck("id", &ids).Error; err != nil {
		log.Printf("[Alert] erro ao listar alertas para retomada: %v", err)
		return nil
	}
	return ids
}

func retomarAlertas(ids []uint, now time.Time) int64 {
	if len(ids) == 0 {
		return 0
	}

	res := database.DB.Model(&database.Alert{}).Where("id IN ?", ids).
		Updates(map[string]any{
			"delivery":        database.AlertDeliveryPendente,
			"next_attempt_at": nil,
			"last_error":      "",
		})
	if res.Error != nil {
		log.Printf("[Alert] erro ao retomar alertas presos: %v", res.Error)
		return 0
	}

	err := database.DB.Model(&database.AlertDelivery{}).
		Where("alert_id IN ? AND status IN ?", ids,
			[]string{database.AlertDeliverySemCanal, database.AlertDeliveryFalhou}).
		Updates(map[string]any{
			"status":          database.AlertDeliveryPendente,
			"attempts":        0,
			"next_attempt_at": nil,
			"last_error":      "",
			"updated_at":      now,
		}).Error
	if err != nil {
		log.Printf("[Alert] erro ao retomar a entrega por canal: %v", err)
	}
	return res.RowsAffected
}

func dispensarLinhas(ids []uint, destino string, now time.Time) {
	if len(ids) == 0 {
		return
	}

	err := database.DB.Model(&database.AlertDelivery{}).
		Where("alert_id IN ? AND status = ?", ids, database.AlertDeliveryPendente).
		Updates(map[string]any{
			"status":          destino,
			"next_attempt_at": nil,
			"updated_at":      now,
		}).Error
	if err != nil {
		log.Printf("[Alert] erro ao dispensar a entrega por canal: %v", err)
	}
}

func reabrirLinhas(alertID uint, now time.Time) {
	err := database.DB.Model(&database.AlertDelivery{}).Where("alert_id = ?", alertID).
		Updates(map[string]any{
			"status":          database.AlertDeliveryPendente,
			"attempts":        0,
			"next_attempt_at": nil,
			"last_error":      "",
			"updated_at":      now,
		}).Error
	if err != nil {
		log.Printf("[Alert] erro ao reabrir a entrega do alerta %d: %v", alertID, err)
	}
}

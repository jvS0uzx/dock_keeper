package alert

import (
	"context"
	"database/sql"
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

	sufixoRecuperacao = ":recuperacao"
	vistoACada        = time.Minute
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

	AlvoTipo string
	AlvoID   string
	AlvoNome string
	Metrica  string
	Valor    *float64
	Limiar   *float64
	Unidade  string
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
		return "high"
	case strings.HasPrefix(texto, "[AVISO]"):
		return "warning"
	default:
		return "info"
	}
}

func Enqueue(e Entrada) bool {
	e.Key = strings.TrimSpace(e.Key)
	e.Text = strings.TrimSpace(e.Text)
	if e.Key == "" {
		return false
	}
	if e.Severity == "" {
		e.Severity = severityFromText(e.Text)
	}
	if e.Text == "" {
		e.Text = textoDoAlvo(alertaDaEntrada(e))
	}
	if e.Text == "" {
		return false
	}
	if abaixoDoMinimo(e.Severity) {
		if claimSlot("abaixo-do-minimo:" + e.Key) {
			log.Printf("[Alert] (abaixo do mínimo notificável) %s", e.Text)
		}
		return false
	}

	if database.DB == nil {
		if !claimSlot(e.Key) {
			return false
		}
		Send(e.Text)
		return true
	}

	now := agora().UTC()
	incidente, existe, err := incidenteVivo(e.Key, now)
	if err != nil {
		return descartar(e, err)
	}
	if existe {
		return renotificar(incidente, e, now)
	}

	proxima := now
	if ultimo := ultimoAviso(e.Key); ultimo != nil && now.Sub(ultimo.UTC()) < cooldown {
		proxima = ultimo.UTC().Add(cooldown)
	}
	alerta := alertaDaEntrada(e)
	alerta.Status = database.AlertStatusOpen
	alerta.ServerID, alerta.SiteID, alerta.RuleID = e.ServerID, e.SiteID, e.RuleID
	alerta.CreatedAt, alerta.LastSeenAt = now, &now
	alerta.Delivery, alerta.NextAttemptAt = database.AlertDeliveryPendente, &proxima
	if err := database.DB.Create(&alerta).Error; err != nil {
		return descartar(e, err)
	}

	observabilidade.AlertasEnfileirados.Add(1)
	log.Printf("[Alert] enfileirado (#%d): %s", alerta.ID, e.Text)
	return true
}

func descartar(e Entrada, err error) bool {
	observabilidade.AlertasDescartados.Add(1)
	log.Printf("[Alert] erro ao enfileirar %q, alerta descartado para não segurar quem detectou: %v | %s",
		e.Key, err, e.Text)
	return false
}

func incidenteVivo(key string, now time.Time) (database.Alert, bool, error) {
	var incidente database.Alert
	err := database.DB.
		Where("key = ? AND status <> ? AND COALESCE(last_seen_at, created_at) >= ?",
			key, database.AlertStatusResolved, now.Add(-janelaDeRetomada())).
		Order("id desc").Take(&incidente).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return incidente, false, nil
	}
	return incidente, err == nil, err
}

func ultimoAviso(key string) *time.Time {
	var ultimo sql.NullTime
	err := database.DB.Model(&database.Alert{}).Where("key = ?", key).
		Select("MAX(last_notified_at)").Scan(&ultimo).Error
	if err != nil || !ultimo.Valid {
		return nil
	}
	return &ultimo.Time
}

func renotificar(incidente database.Alert, e Entrada, now time.Time) bool {
	mudanca := map[string]any{}
	if incidente.LastSeenAt == nil || now.Sub(incidente.LastSeenAt.UTC()) >= vistoACada {
		mudanca["last_seen_at"] = now
	}

	avisar := incidente.Status == database.AlertStatusOpen &&
		incidente.Delivery == database.AlertDeliveryEnviado &&
		(incidente.LastNotifiedAt == nil || now.Sub(incidente.LastNotifiedAt.UTC()) >= cooldown)
	if avisar {
		mudanca["last_seen_at"] = now
		mudanca["text"] = e.Text
		mudanca["severity"] = e.Severity
		mudanca["alvo_tipo"] = ponteiroDeTexto(e.AlvoTipo)
		mudanca["alvo_id"] = ponteiroDeTexto(e.AlvoID)
		mudanca["alvo_nome"] = ponteiroDeTexto(e.AlvoNome)
		mudanca["metrica"] = ponteiroDeTexto(e.Metrica)
		mudanca["valor"] = e.Valor
		mudanca["limiar"] = e.Limiar
		mudanca["unidade"] = ponteiroDeTexto(e.Unidade)
		mudanca["delivery"] = database.AlertDeliveryPendente
		mudanca["attempts"] = 0
		mudanca["next_attempt_at"] = now
		mudanca["last_error"] = ""
		mudanca["renotify_count"] = gorm.Expr("renotify_count + 1")
	}
	if len(mudanca) == 0 {
		return false
	}

	err := database.DB.Model(&database.Alert{}).Where("id = ?", incidente.ID).Updates(mudanca).Error
	if err != nil {
		log.Printf("[Alert] erro ao atualizar o incidente #%d de %q: %v", incidente.ID, e.Key, err)
		return false
	}
	if avisar {
		reabrirLinhas(incidente.ID, now)
		observabilidade.AlertasEnfileirados.Add(1)
		log.Printf("[Alert] renotificado (#%d): %s", incidente.ID, e.Text)
	}
	return avisar
}

func Recovered(e Entrada) bool {
	if database.DB == nil {
		return false
	}

	var abertos []database.Alert
	err := database.DB.Where("key = ? AND status <> ?", e.Key, database.AlertStatusResolved).
		Order("id asc").Find(&abertos).Error
	if err != nil {
		log.Printf("[Alert] erro ao procurar o alerta aberto de %q: %v", e.Key, err)
		return false
	}
	if len(abertos) == 0 {
		return false
	}

	now := agora().UTC()
	err = database.DB.Model(&database.Alert{}).
		Where("key = ? AND status <> ?", e.Key, database.AlertStatusResolved).
		Updates(map[string]any{"status": database.AlertStatusResolved, "resolved_at": now}).Error
	if err != nil {
		log.Printf("[Alert] erro ao resolver %q: %v", e.Key, err)
		return false
	}

	anunciado := false
	for _, a := range abertos {
		anunciado = anunciado || a.LastNotifiedAt != nil
		if e.ServerID == nil {
			e.ServerID = a.ServerID
		}
		if e.SiteID == nil {
			e.SiteID = a.SiteID
		}
		if e.RuleID == nil {
			e.RuleID = a.RuleID
		}
	}
	if !anunciado || !RecoveryEnabled() || strings.TrimSpace(e.Text) == "" {
		return false
	}

	ok := alertaDaEntrada(e)
	ok.Key, ok.Severity, ok.Text = e.Key+sufixoRecuperacao, "info", strings.TrimSpace(e.Text)
	ok.Status, ok.ResolvedAt = database.AlertStatusResolved, &now
	ok.ServerID, ok.SiteID, ok.RuleID = e.ServerID, e.SiteID, e.RuleID
	ok.CreatedAt, ok.LastSeenAt = now, &now
	ok.Delivery, ok.NextAttemptAt = database.AlertDeliveryPendente, &now
	if err := database.DB.Create(&ok).Error; err != nil {
		log.Printf("[Alert] erro ao enfileirar a recuperação de %q: %v", e.Key, err)
		return false
	}
	observabilidade.AlertasEnfileirados.Add(1)
	log.Printf("[Alert] enfileirado (#%d): %s", ok.ID, ok.Text)
	return true
}

func ChavesAbertas(prefixo string) []string {
	if database.DB == nil {
		return nil
	}

	var chaves []string
	err := database.DB.Model(&database.Alert{}).
		Where("status <> ? AND starts_with(key, ?)", database.AlertStatusResolved, prefixo).
		Distinct().Pluck("key", &chaves).Error
	if err != nil {
		log.Printf("[Alert] erro ao listar os alertas abertos de %q: %v", prefixo, err)
		return nil
	}
	return chaves
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

	dispensarResolvidos()
	retomarPresos(now)

	ativos := canaisAtivos()
	claimed := claimBatch(now)
	for _, alerta := range claimed {
		entregarAlerta(alerta, ativos, agora().UTC())
	}
	return len(claimed)
}

func janelaDeRetomada() time.Duration {
	return time.Duration(config.Inteiro("ALERT_RESUME_HOURS", retomadaPadraoHoras)) * time.Hour
}

func dispensarResolvidos() {
	now := agora().UTC()
	for _, caso := range []struct{ filtro, destino string }{
		{"last_notified_at IS NULL", database.AlertDeliveryDispensado},
		{"last_notified_at IS NOT NULL", database.AlertDeliveryEnviado},
	} {
		filtrar := func() *gorm.DB {
			return database.DB.Model(&database.Alert{}).
				Where("status = ? AND delivery = ? AND key NOT LIKE ?",
					database.AlertStatusResolved, database.AlertDeliveryPendente, "%"+sufixoRecuperacao).
				Where(caso.filtro)
		}

		ids := idsDeAlertas(filtrar())
		err := filtrar().Updates(map[string]any{"delivery": caso.destino, "next_attempt_at": nil}).Error
		if err != nil {
			log.Printf("[Alert] erro ao dispensar a entrega de alertas já resolvidos: %v", err)
			continue
		}
		dispensarLinhas(ids, caso.destino, now)
	}
}

func retomarPresos(now time.Time) {
	saude := Status()
	if saude.Estado == EstadoDesligado {
		return
	}

	corte := now.Add(-janelaDeRetomada())

	presos := idsDeAlertas(database.DB.Model(&database.Alert{}).
		Where("delivery = ? AND status = ? AND COALESCE(last_seen_at, created_at) >= ?",
			database.AlertDeliverySemCanal, database.AlertStatusOpen, corte))
	if n := retomarAlertas(presos, now); n > 0 {
		observabilidade.AlertasSemCanal.Add(-n)
		log.Printf("[Alert] canal de volta: %d alerta(s) preso(s) voltaram para a fila", n)
	}

	if saude.Estado != EstadoOK || saude.VerificadoEm == nil {
		return
	}
	falhos := idsDeAlertas(database.DB.Model(&database.Alert{}).
		Where("delivery = ? AND status = ? AND COALESCE(last_seen_at, created_at) >= ? AND last_attempt_at < ?",
			database.AlertDeliveryFalhou, database.AlertStatusOpen, corte, saude.VerificadoEm.UTC()))
	if n := retomarAlertas(falhos, now); n > 0 {
		log.Printf("[Alert] canal respondendo de novo: %d alerta(s) que falharam voltaram para a fila", n)
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
			Where("status <> ? OR key LIKE ?", database.AlertStatusResolved, "%"+sufixoRecuperacao).
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

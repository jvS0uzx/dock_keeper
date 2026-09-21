package api

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/alert"
	"github.com/jvS0uzx/dock_keeper/internal/auth"
	"github.com/jvS0uzx/dock_keeper/internal/database"
	"github.com/jvS0uzx/dock_keeper/internal/logstore"
	"github.com/jvS0uzx/dock_keeper/internal/observabilidade"
)

const readyPingTimeout = 2 * time.Second

func (c Config) readyHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeError(w, http.StatusMethodNotAllowed, "método não permitido")
		return
	}
	if err := pingDatabase(r.Context()); err != nil {
		log.Printf("[API] /readyz: banco indisponível: %v", err)
		writeJSON(w, http.StatusServiceUnavailable, corpoReadyz(r, c.temCredencial(r), "indisponivel", "banco indisponível"))
		return
	}
	writeJSON(w, http.StatusOK, corpoReadyz(r, c.temCredencial(r), "ok", ""))
}

func (c Config) temCredencial(r *http.Request) bool {
	if _, ok := auth.Lookup(bearerToken(r)); ok {
		return true
	}
	return c.tokenMatches(r)
}

func corpoReadyz(r *http.Request, detalhado bool, status, db string) map[string]any {
	corpo := map[string]any{"status": status}
	if db != "" {
		corpo["db"] = db
	}

	saude := alert.Status()
	corpo["alertas"] = saude.Estado
	if detalhado && saude.Detalhe != "" {
		corpo["alertas_detalhe"] = saude.Detalhe
	}

	descartados := logstore.Descartadas()
	corpo["logs_descartados"] = descartados

	motivos := []string{}

	falhos, errFalhos := contarAlertas(r, database.AlertDeliveryFalhou)
	presos, errPresos := contarAlertas(r, database.AlertDeliverySemCanal)
	corpo["alertas_falhos"], corpo["alertas_sem_canal"] = falhos, presos
	if errFalhos != nil {
		corpo["alertas_falhos"] = nil
	}
	if errPresos != nil {
		corpo["alertas_sem_canal"] = nil
	}
	if errFalhos != nil || errPresos != nil {
		log.Printf("[API] /readyz: erro ao contar alertas: %v %v", errFalhos, errPresos)
		motivos = append(motivos, "não foi possível ler os contadores de alerta no banco")
	}

	if saude.Estado == alert.EstadoDegradado {
		motivos = append(motivos, "canal de alerta degradado")
	}
	if falhos > 0 {
		motivos = append(motivos, "alerta com entrega falhou")
	}
	if presos > 0 {
		motivos = append(motivos, "alerta preso sem canal de entrega configurado")
	}
	if descartados > 0 {
		motivos = append(motivos, "linha de log descartada pela fila")
	}
	if detalhado && len(motivos) > 0 {
		corpo["degradado"] = motivos
	}
	return corpo
}

func contarAlertas(r *http.Request, entrega string) (int64, error) {
	db := database.From(r.Context())
	if db == nil {
		return 0, errDatabaseNotConnected
	}

	var n int64
	err := db.Model(&database.Alert{}).
		Where("delivery = ? AND status <> ?", entrega, database.AlertStatusResolved).
		Count(&n).Error
	return n, err
}

func pingDatabase(ctx context.Context) error {
	if database.DB == nil {
		return errDatabaseNotConnected
	}
	sqlDB, err := database.DB.DB()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, readyPingTimeout)
	defer cancel()
	return sqlDB.PingContext(ctx)
}

const errDatabaseNotConnected = configError("conexão com o banco não inicializada")

func metricsHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	observabilidade.Escrever(w)
}

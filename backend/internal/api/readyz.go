package api

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/alert"
	"github.com/jvS0uzx/dock_keeper/internal/database"
	"github.com/jvS0uzx/dock_keeper/internal/logstore"
	"github.com/jvS0uzx/dock_keeper/internal/observabilidade"
)

const readyPingTimeout = 2 * time.Second

func readyHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeError(w, http.StatusMethodNotAllowed, "método não permitido")
		return
	}
	if err := pingDatabase(r.Context()); err != nil {
		log.Printf("[API] /readyz: banco indisponível: %v", err)
		writeJSON(w, http.StatusServiceUnavailable, corpoReadyz(r, "indisponivel", "banco indisponível"))
		return
	}
	writeJSON(w, http.StatusOK, corpoReadyz(r, "ok", ""))
}

func corpoReadyz(r *http.Request, status, db string) map[string]any {
	corpo := map[string]any{"status": status}
	if db != "" {
		corpo["db"] = db
	}

	saude := alert.Status()
	corpo["alertas"] = saude.Estado
	if saude.Detalhe != "" {
		corpo["alertas_detalhe"] = saude.Detalhe
	}

	descartados := logstore.Descartadas()
	corpo["logs_descartados"] = descartados

	falhos := alertasFalhos(r)
	corpo["alertas_falhos"] = falhos

	presos := alertasSemCanal(r)
	corpo["alertas_sem_canal"] = presos

	motivos := []string{}
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
	if len(motivos) > 0 {
		corpo["degradado"] = motivos
	}
	return corpo
}

func alertasSemCanal(r *http.Request) int64 {
	db := database.From(r.Context())
	if db == nil {
		return 0
	}

	var n int64
	err := db.Model(&database.Alert{}).
		Where("delivery = ? AND status <> ?", database.AlertDeliverySemCanal, database.AlertStatusResolved).
		Count(&n).Error
	if err != nil {
		return 0
	}
	return n
}

func alertasFalhos(r *http.Request) int64 {
	db := database.From(r.Context())
	if db == nil {
		return 0
	}

	var n int64
	err := db.Model(&database.Alert{}).
		Where("delivery = ? AND status <> ?", database.AlertDeliveryFalhou, database.AlertStatusResolved).
		Count(&n).Error
	if err != nil {
		return 0
	}
	return n
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

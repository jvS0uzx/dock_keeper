package api

import (
	"context"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/auth"
)

const (
	readHeaderTimeout = 10 * time.Second
	readTimeout       = 30 * time.Second
	writeTimeout      = 60 * time.Second
	idleTimeout       = 120 * time.Second
	shutdownTimeout   = 15 * time.Second
)

func Routes(cfg Config) http.Handler {
	mux := http.NewServeMux()

	if cfg.logins == nil {
		cfg.logins = newLoginLimiter(defaultLoginWindow, defaultLoginMaxPerIP, defaultLoginMaxPerUser)
	}

	api := func(h http.HandlerFunc, methods ...string) http.HandlerFunc {
		return chain(h, cfg.withCORS, cfg.audit(auditAll), limitBody(maxFormBodyBytes),
			cfg.requireAuth, allowMethods(methods...))
	}
	admin := func(h http.HandlerFunc, methods ...string) http.HandlerFunc {
		return chain(h, cfg.withCORS, cfg.audit(auditAll), limitBody(maxFormBodyBytes),
			cfg.requireGlobalRole(auth.RoleAdmin), allowMethods(methods...))
	}
	public := func(h http.HandlerFunc, methods ...string) http.HandlerFunc {
		return chain(h, cfg.withCORS, cfg.audit(auditAll), limitBody(maxFormBodyBytes),
			allowMethods(methods...))
	}
	readViewerWriteOperator := func(h http.HandlerFunc, methods ...string) http.HandlerFunc {
		return chain(h, cfg.withCORS, cfg.audit(auditAll), limitBody(maxFormBodyBytes),
			cfg.requireRoleByMethod(auth.RoleViewer, auth.RoleOperator),
			allowMethods(methods...))
	}
	upload := func(h http.HandlerFunc, methods ...string) http.HandlerFunc {
		return chain(h, cfg.withCORS, cfg.audit(auditAll), limitBody(maxPlanUploadBytes),
			cfg.requireRoleByMethod(auth.RoleViewer, auth.RoleOperator),
			allowMethods(methods...))
	}
	globalWrite := func(h http.HandlerFunc, methods ...string) http.HandlerFunc {
		return chain(h, cfg.withCORS, cfg.audit(auditAll), limitBody(maxFormBodyBytes),
			cfg.requireGlobalWrite(auth.RoleOperator),
			allowMethods(methods...))
	}
	globalWriteSelfAudited := func(h http.HandlerFunc, methods ...string) http.HandlerFunc {
		return chain(h, cfg.withCORS, limitBody(maxFormBodyBytes),
			cfg.requireGlobalWrite(auth.RoleOperator),
			allowMethods(methods...))
	}
	stream := func(h http.HandlerFunc) http.HandlerFunc {
		return chain(semPrazoDeEscrita(h), cfg.withCORS, cfg.requireTicket, allowMethods(http.MethodGet))
	}

	mux.HandleFunc("/api/servers", admin(cfg.serversHandler,
		http.MethodGet, http.MethodPost, http.MethodPatch, http.MethodDelete))
	mux.HandleFunc("/api/metrics/live", api(liveMetricsHandler, http.MethodGet))
	mux.HandleFunc("/api/metrics/catalogo", api(catalogoDeMetricasHandler, http.MethodGet))
	mux.HandleFunc("/api/metrics/history", api(HistoryHandler, http.MethodGet))
	mux.HandleFunc("/api/dashboards", api(dashboardsHandler,
		http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete))
	mux.HandleFunc("/api/annotations", api(annotationsHandler, http.MethodGet, http.MethodPost, http.MethodDelete))

	mux.HandleFunc("/api/stream-ticket", api(cfg.streamTicketHandler, http.MethodPost))

	mux.HandleFunc("/api/containers/action", globalWriteSelfAudited(cfg.containerActionHandler, http.MethodPost))
	mux.HandleFunc("/api/containers/logs/stream", stream(cfg.containerLogsStreamHandler))

	mux.HandleFunc("/api/security/radar", api(cfg.securityRadarHandler, http.MethodGet))
	mux.HandleFunc("/api/security/authlog/stream", stream(cfg.authLogStreamHandler))

	mux.HandleFunc("/api/ssl/domains", globalWrite(sslDomainsHandler, http.MethodGet, http.MethodPost, http.MethodDelete))
	mux.HandleFunc("/api/ssl/discover", api(sslDiscoverHandler, http.MethodGet))
	mux.HandleFunc("/api/ssl/import", globalWrite(sslImportHandler, http.MethodPost))
	mux.HandleFunc("/api/ssl/recheck", globalWrite(sslRecheckHandler, http.MethodPost))
	mux.HandleFunc("/api/ssl/recheck-all", globalWrite(sslRecheckAllHandler, http.MethodPost))

	mux.HandleFunc("/api/network/hosts", api(networkHostsHandler, http.MethodGet))
	mux.HandleFunc("/api/network/scan", globalWrite(networkScanHandler, http.MethodPost))
	mux.HandleFunc("/api/network/host", api(networkHostUpdateHandler, http.MethodPatch))

	mux.HandleFunc("/api/sites", readViewerWriteOperator(sitesHandler, http.MethodGet, http.MethodPost, http.MethodDelete))

	mux.HandleFunc("/api/floorplans", upload(floorPlansHandler, http.MethodGet, http.MethodPost))
	mux.HandleFunc("/api/floorplans/", readViewerWriteOperator(floorPlanRouter,
		http.MethodGet, http.MethodPut, http.MethodDelete))

	mux.HandleFunc("/api/alerts", api(alertsHandler, http.MethodGet))
	mux.HandleFunc("/api/alerts/summary", api(alertsSummaryHandler, http.MethodGet))
	mux.HandleFunc("/api/alerts/ack", api(alertAckHandler, http.MethodPost))
	mux.HandleFunc("/api/alerts/resolve", api(alertResolveHandler, http.MethodPost))
	mux.HandleFunc("/api/alerts/rules", globalWrite(AlertRulesHandler,
		http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete))
	mux.HandleFunc("/api/logs/search", api(LogSearchHandler, http.MethodGet))

	mux.HandleFunc("/api/ingest/metrics",
		chain(IngestHandler, cfg.audit(auditOnlyDenied), limitBody(maxFormBodyBytes)))
	mux.HandleFunc("/api/ingest/inventory",
		chain(InventoryIngestHandler, cfg.audit(auditOnlyDenied), limitBody(maxIngestBodyBytes)))

	mux.HandleFunc("/api/enroll/tokens", admin(cfg.enrollTokensHandler, http.MethodPost))
	mux.HandleFunc("/api/devices", admin(cfg.devicesHandler, http.MethodGet, http.MethodDelete))
	mux.HandleFunc("/api/enroll", public(cfg.enrollHandler, http.MethodPost))

	mux.HandleFunc("/api/auth/login", public(cfg.loginHandler, http.MethodPost))
	mux.HandleFunc("/api/auth/logout", api(logoutHandler, http.MethodPost))
	mux.HandleFunc("/api/auth/me", api(cfg.meHandler, http.MethodGet))
	mux.HandleFunc("/api/users", admin(usersHandler,
		http.MethodGet, http.MethodPost, http.MethodPatch, http.MethodDelete))

	mux.HandleFunc("/api/audit", admin(auditListHandler, http.MethodGet))

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("/readyz", cfg.readyHandler)
	mux.HandleFunc("/api/readyz", public(cfg.readyHandler, http.MethodGet, http.MethodHead))
	mux.HandleFunc("/metrics", api(metricsHandler, http.MethodGet))

	return mux
}

func semPrazoDeEscrita(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := http.NewResponseController(w).SetWriteDeadline(time.Time{}); err != nil {
			log.Printf("[API] stream sem prazo de escrita não pôde ser configurado: %v", err)
		}
		h(w, r)
	}
}

func StartServer(ctx context.Context, cfg Config) error {
	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           Routes(cfg),
		BaseContext:       func(net.Listener) context.Context { return ctx },
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("[API] escutando em http://localhost%s", cfg.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		log.Println("[API] encerrando, drenando conexões...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}

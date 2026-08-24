package api

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/joaov/vd_stats/internal/auth"
)

const (
	// Sem ReadHeaderTimeout uma conexão que nunca termina o cabeçalho segura um
	// worker para sempre (Slowloris).
	readHeaderTimeout = 10 * time.Second
	readTimeout       = 30 * time.Second
	idleTimeout       = 120 * time.Second
	shutdownTimeout   = 15 * time.Second
)

// Routes monta o mux com toda a superfície HTTP da API.
// Tudo exige o token: o painel comanda SSH como root nas VPS.
func Routes(cfg Config) http.Handler {
	mux := http.NewServeMux()

	// Config chega por valor, então o padrão vale só para este mux. Cobre quem
	// monta a Config à mão — testes — sem obrigar cada ponto de construção a
	// conhecer o limitador.
	if cfg.logins == nil {
		cfg.logins = newLoginLimiter(defaultLoginWindow, defaultLoginMaxPerIP, defaultLoginMaxPerUser)
	}

	// Rotas normais: CORS + teto de corpo + credencial por cabeçalho, papel
	// mínimo viewer. O teto vem antes da autenticação de propósito: corpo
	// absurdo é recusado sem custar nem uma consulta de sessão.
	api := func(h http.HandlerFunc, methods ...string) http.HandlerFunc {
		return chain(h, cfg.withCORS, cfg.audit(auditAll), limitBody(maxFormBodyBytes),
			cfg.requireAuth, allowMethods(methods...))
	}
	// Rotas de administração: usuários e servidores com acesso SSH. O papel
	// precisa valer globalmente — administrar uma filial não vira administrar
	// o parque, nem na leitura.
	admin := func(h http.HandlerFunc, methods ...string) http.HandlerFunc {
		return chain(h, cfg.withCORS, cfg.audit(auditAll), limitBody(maxFormBodyBytes),
			cfg.requireGlobalRole(auth.RoleAdmin), allowMethods(methods...))
	}
	// Rotas abertas a quem ainda não tem credencial.
	public := func(h http.HandlerFunc, methods ...string) http.HandlerFunc {
		return chain(h, cfg.withCORS, cfg.audit(auditAll), limitBody(maxFormBodyBytes),
			allowMethods(methods...))
	}
	// Leitura para todos os papéis, escrita só a partir de operador ("Suporte
	// TI" na interface). É a regra "Visualizador não cadastra nada".
	readViewerWriteOperator := func(h http.HandlerFunc, methods ...string) http.HandlerFunc {
		return chain(h, cfg.withCORS, cfg.audit(auditAll), limitBody(maxFormBodyBytes),
			cfg.requireRoleByMethod(auth.RoleViewer, auth.RoleOperator),
			allowMethods(methods...))
	}
	// Mesmo gate do readViewerWriteOperator, com teto próprio: o corpo aqui é
	// a imagem da planta baixa, não um formulário.
	upload := func(h http.HandlerFunc, methods ...string) http.HandlerFunc {
		return chain(h, cfg.withCORS, cfg.audit(auditAll), limitBody(maxPlanUploadBytes),
			cfg.requireRoleByMethod(auth.RoleViewer, auth.RoleOperator),
			allowMethods(methods...))
	}
	// Infra sem unidade: leitura livre, escrita exige operador GLOBAL.
	globalWrite := func(h http.HandlerFunc, methods ...string) http.HandlerFunc {
		return chain(h, cfg.withCORS, cfg.audit(auditAll), limitBody(maxFormBodyBytes),
			cfg.requireGlobalWrite(auth.RoleOperator),
			allowMethods(methods...))
	}
	// Mesma cadeia do globalWrite, sem o middleware de auditoria: o handler
	// audita por conta própria, com o verbo, o servidor alvo, a unidade e o
	// nome do container, e gravando ANTES de o comando sair — nada disso o
	// middleware genérico, que enxerga método, rota e status, tem como saber.
	// Duas linhas por ação diriam a mesma coisa duas vezes, e a menos
	// informativa apareceria junto na consulta.
	globalWriteSelfAudited := func(h http.HandlerFunc, methods ...string) http.HandlerFunc {
		return chain(h, cfg.withCORS, limitBody(maxFormBodyBytes),
			cfg.requireGlobalWrite(auth.RoleOperator),
			allowMethods(methods...))
	}
	// Rotas SSE: autorizadas por ticket de uso único, para o segredo permanente
	// não acabar no access log do proxy nem no histórico do browser.
	//
	// Sem auditoria e sem teto de corpo de propósito: são GET, que o middleware
	// de auditoria ignora, e a resposta fica aberta indefinidamente — envelopar
	// o ResponseWriter aqui só acrescentaria risco de quebrar o streaming. A
	// emissão do ticket, essa sim, é auditada em /api/stream-ticket.
	stream := func(h http.HandlerFunc) http.HandlerFunc {
		return chain(h, cfg.withCORS, cfg.requireTicket, allowMethods(http.MethodGet))
	}

	// Cadastrar servidor entrega acesso SSH como root: é operação de admin.
	mux.HandleFunc("/api/servers", admin(cfg.serversHandler, http.MethodGet, http.MethodPost, http.MethodDelete))
	mux.HandleFunc("/api/metrics/live", api(liveMetricsHandler, http.MethodGet))
	mux.HandleFunc("/api/metrics/history", api(HistoryHandler, http.MethodGet))

	mux.HandleFunc("/api/stream-ticket", api(cfg.streamTicketHandler, http.MethodPost))

	mux.HandleFunc("/api/containers/action", globalWriteSelfAudited(cfg.containerActionHandler, http.MethodPost))
	mux.HandleFunc("/api/containers/logs/stream", stream(cfg.containerLogsStreamHandler))

	mux.HandleFunc("/api/security/radar", api(cfg.securityRadarHandler, http.MethodGet))
	mux.HandleFunc("/api/security/authlog/stream", stream(cfg.authLogStreamHandler))

	mux.HandleFunc("/api/ssl/domains", globalWrite(sslDomainsHandler, http.MethodGet, http.MethodPost, http.MethodDelete))
	// Descoberta de domínios a partir do access log do Nginx: leitura para
	// qualquer papel, importação exige operador global como o resto do SSL.
	mux.HandleFunc("/api/ssl/discover", api(sslDiscoverHandler, http.MethodGet))
	mux.HandleFunc("/api/ssl/import", globalWrite(sslImportHandler, http.MethodPost))
	mux.HandleFunc("/api/ssl/recheck", globalWrite(sslRecheckHandler, http.MethodPost))
	mux.HandleFunc("/api/ssl/recheck-all", globalWrite(sslRecheckAllHandler, http.MethodPost))

	// Inventário da rede local (2o painel).
	mux.HandleFunc("/api/network/hosts", api(networkHostsHandler, http.MethodGet))
	mux.HandleFunc("/api/network/scan", globalWrite(networkScanHandler, http.MethodPost))
	// O PATCH valida o papel na unidade do host dentro do handler.
	mux.HandleFunc("/api/network/host", api(networkHostUpdateHandler, http.MethodPatch))

	mux.HandleFunc("/api/sites", readViewerWriteOperator(sitesHandler, http.MethodGet, http.MethodPost, http.MethodDelete))

	// Plantas baixas. O sufixo decide o handler porque o ServeMux casa por
	// prefixo em rotas terminadas em barra.
	mux.HandleFunc("/api/floorplans", upload(floorPlansHandler, http.MethodGet, http.MethodPost))
	mux.HandleFunc("/api/floorplans/", readViewerWriteOperator(floorPlanRouter,
		http.MethodGet, http.MethodPut, http.MethodDelete))

	mux.HandleFunc("/api/alerts/rules", globalWrite(AlertRulesHandler,
		http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete))
	mux.HandleFunc("/api/logs/search", api(LogSearchHandler, http.MethodGet))

	// Ingestão do agente push: máquina-a-máquina, autenticada pelo próprio
	// token de agente (X-Agent-Token), sem CORS de browser. O teto de corpo é o
	// único middleware daqui, e no inventário ele é o que impede a lista de
	// hosts de virar memória antes de o teto de maxInventoryHosts ser conferido.
	//
	// A auditoria aqui registra SOMENTE a recusa, e isso não é esquecimento:
	// são milhares de push por minuto num parque de algumas centenas de hosts,
	// e gravar o sucesso de cada um destruiria a tabela e afogaria o sinal.
	// Token inválido, ao contrário, é exatamente o que a auditoria existe para
	// capturar.
	mux.HandleFunc("/api/ingest/metrics",
		chain(IngestHandler, cfg.audit(auditOnlyDenied), limitBody(maxFormBodyBytes)))
	mux.HandleFunc("/api/ingest/inventory",
		chain(InventoryIngestHandler, cfg.audit(auditOnlyDenied), limitBody(maxIngestBodyBytes)))

	// Identidade por dispositivo. Emitir convite é conceder a uma máquina o
	// direito de escrever métrica e inventário de uma filial inteira, então a
	// emissão e a revogação são de admin global.
	mux.HandleFunc("/api/enroll/tokens", admin(cfg.enrollTokensHandler, http.MethodPost))
	mux.HandleFunc("/api/devices", admin(cfg.devicesHandler, http.MethodGet, http.MethodDelete))
	// A troca do convite pela credencial é pública porque quem chama ainda não
	// tem credencial — é o que vem buscar. A proteção é o convite ser de uso
	// único e de validade curta, com o teto de corpo e o limite de tentativa do
	// wrapper valendo aqui como em qualquer rota aberta.
	mux.HandleFunc("/api/enroll", public(cfg.enrollHandler, http.MethodPost))

	// Autenticação de pessoas.
	mux.HandleFunc("/api/auth/login", public(cfg.loginHandler, http.MethodPost))
	mux.HandleFunc("/api/auth/logout", api(logoutHandler, http.MethodPost))
	mux.HandleFunc("/api/auth/me", api(cfg.meHandler, http.MethodGet))
	mux.HandleFunc("/api/users", admin(usersHandler,
		http.MethodGet, http.MethodPost, http.MethodPatch, http.MethodDelete))

	// Auditoria: admin GLOBAL, porque a tabela mostra ação de todas as unidades.
	mux.HandleFunc("/api/audit", admin(auditListHandler, http.MethodGet))

	// Liveness para o orquestrador; endpoint sem credencial.
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	return mux
}

// StartServer sobe a API e bloqueia até ctx ser cancelado, aí drena as
// conexões abertas antes de devolver.
func StartServer(ctx context.Context, cfg Config) error {
	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           Routes(cfg),
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		IdleTimeout:       idleTimeout,
		// WriteTimeout fica zerado de propósito: as rotas de SSE mantêm a
		// resposta aberta indefinidamente e seriam cortadas no meio.
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

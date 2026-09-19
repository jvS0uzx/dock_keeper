package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/jvS0uzx/dock_keeper/internal/audit"
	"github.com/jvS0uzx/dock_keeper/internal/auth"
	"github.com/jvS0uzx/dock_keeper/internal/config"
	"github.com/jvS0uzx/dock_keeper/internal/database"
	"gorm.io/gorm"
)

type Config struct {
	Addr           string
	Token          string
	AllowedOrigins []string
	SSHKeyPath     string

	TrustProxyHeaders bool

	tickets *ticketStore
	logins  *loginLimiter
}

func LoadConfig(addr string) (Config, error) {
	cfg := Config{
		Addr:              addr,
		Token:             os.Getenv("API_TOKEN"),
		SSHKeyPath:        os.Getenv("SSH_KEY_PATH"),
		TrustProxyHeaders: strings.EqualFold(strings.TrimSpace(os.Getenv("TRUST_PROXY_HEADERS")), "true"),
		tickets:           newTicketStore(),
		logins: newLoginLimiter(
			config.Duracao("LOGIN_RATE_WINDOW", defaultLoginWindow),
			config.Inteiro("LOGIN_RATE_MAX_IP", defaultLoginMaxPerIP),
			config.Inteiro("LOGIN_RATE_MAX_USER", defaultLoginMaxPerUser),
		),
	}

	hostOfflineAfter = database.EnvDuration("HOST_OFFLINE_AFTER", defaultHostOfflineAfter)

	for _, o := range strings.Split(os.Getenv("ALLOWED_ORIGINS"), ",") {
		if o = strings.TrimSpace(o); o != "" {
			cfg.AllowedOrigins = append(cfg.AllowedOrigins, o)
		}
	}
	if len(cfg.AllowedOrigins) == 0 {
		cfg.AllowedOrigins = []string{"http://localhost:5173"}
		log.Printf("[API] ALLOWED_ORIGINS não definido; liberando apenas %s", cfg.AllowedOrigins[0])
	}

	if cfg.Token == "" {
		return cfg, errTokenRequired
	}
	return cfg, nil
}

type configError string

func (e configError) Error() string { return string(e) }

const errTokenRequired = configError("API_TOKEN não definido: defina um token forte no .env antes de subir a API")

type middleware func(http.HandlerFunc) http.HandlerFunc

func chain(h http.HandlerFunc, ms ...middleware) http.HandlerFunc {
	for i := len(ms) - 1; i >= 0; i-- {
		h = ms[i](h)
	}
	return h
}

func (c Config) withCORS(next http.HandlerFunc) http.HandlerFunc {
	allowed := make(map[string]bool, len(c.AllowedOrigins))
	for _, o := range c.AllowedOrigins {
		allowed[o] = true
	}

	return func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		w.Header().Add("Vary", "Origin")
		if origin != "" && allowed[origin] {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Token")
		w.Header().Set("Access-Control-Max-Age", "600")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next(w, r)
	}
}

type sessionCtxKey struct{}

var machineSession = auth.Session{
	Username: "api-token",
	Role:     auth.RoleAdmin,
	Accesses: []auth.Access{{SiteID: nil, Role: auth.RoleAdmin}},
}

func sessionFrom(r *http.Request) auth.Session {
	if s, ok := r.Context().Value(sessionCtxKey{}).(auth.Session); ok {
		return s
	}
	return auth.Session{}
}

func withSession(r *http.Request, s auth.Session) *http.Request {
	if info, ok := r.Context().Value(auditTargetCtxKey{}).(*auditTargetInfo); ok {
		info.sess = s
		info.sessSet = true
	}
	return r.WithContext(context.WithValue(r.Context(), sessionCtxKey{}, s))
}

func (c Config) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return c.requireRole(auth.RoleViewer)(next)
}

func (c Config) checkRole(w http.ResponseWriter, r *http.Request, minRole string, next http.HandlerFunc) {
	token := bearerToken(r)

	if session, ok := auth.Lookup(token); ok {
		if !auth.Allows(auth.MaxRole(session.Accesses), minRole) {
			writeError(w, http.StatusForbidden, "seu perfil não permite esta ação")
			return
		}
		next(w, withSession(r, session))
		return
	}

	if c.tokenMatches(r) {
		if !maquinaPodeEscrever(r) {
			writeError(w, http.StatusForbidden,
				"o token de máquina é somente leitura; ligue API_TOKEN_ALLOW_WRITE no painel para permitir escrita")
			return
		}
		next(w, withSession(r, machineSession))
		return
	}
	writeError(w, http.StatusUnauthorized, "unauthorized")
}

func maquinaPodeEscrever(r *http.Request) bool {
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		return true
	}
	return escritaDeMaquinaLiberada()
}

func escritaDeMaquinaLiberada() bool {
	return config.Booleano("API_TOKEN_ALLOW_WRITE", false)
}

func (c Config) requireRole(minRole string) middleware {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			c.checkRole(w, r, minRole, next)
		}
	}
}

func (c Config) requireRoleByMethod(readRole, writeRole string) middleware {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			min := writeRole
			if r.Method == http.MethodGet || r.Method == http.MethodHead {
				min = readRole
			}
			c.checkRole(w, r, min, next)
		}
	}
}

func (c Config) requireGlobalWrite(writeRole string) middleware {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			c.checkRole(w, r, auth.RoleViewer, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet && r.Method != http.MethodHead {
					sess := sessionFrom(r)
					if !auth.Allows(auth.GlobalRole(sess.Accesses), writeRole) {
						writeError(w, http.StatusForbidden, "esta ação exige acesso global")
						return
					}
				}
				next(w, r)
			})
		}
	}
}

func (c Config) requireGlobalRole(minRole string) middleware {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			c.checkRole(w, r, auth.RoleViewer, func(w http.ResponseWriter, r *http.Request) {
				sess := sessionFrom(r)
				if !auth.Allows(auth.GlobalRole(sess.Accesses), minRole) {
					writeError(w, http.StatusForbidden, "esta ação exige acesso global")
					return
				}
				next(w, r)
			})
		}
	}
}

func (c Config) requireTicket(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if session, ok := auth.Lookup(bearerToken(r)); ok {
			next(w, withSession(r, session))
			return
		}
		if c.tokenMatches(r) {
			if !maquinaPodeEscrever(r) {
				writeError(w, http.StatusForbidden,
					"o token de máquina é somente leitura; ligue API_TOKEN_ALLOW_WRITE no painel para permitir escrita")
				return
			}
			next(w, withSession(r, machineSession))
			return
		}
		if session, ok := c.tickets.consume(r.URL.Query().Get("ticket")); ok {
			next(w, withSession(r, session))
			return
		}
		writeError(w, http.StatusUnauthorized, "unauthorized")
	}
}

func (c Config) tokenMatches(r *http.Request) bool {
	candidates := []string{
		strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "),
		r.Header.Get("X-API-Token"),
	}
	for _, got := range candidates {
		if got != "" && subtle.ConstantTimeCompare([]byte(got), []byte(c.Token)) == 1 {
			return true
		}
	}
	return false
}

const (
	maxFormBodyBytes   = 128 << 10
	maxIngestBodyBytes = 4 << 20
)

func limitBody(max int64) middleware {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Body == nil || r.Body == http.NoBody {
				next(w, r)
				return
			}
			if r.ContentLength > max {
				writeError(w, http.StatusRequestEntityTooLarge, "corpo da requisição grande demais")
				return
			}
			guard := &bodyGuard{ReadCloser: http.MaxBytesReader(w, r.Body, max)}
			r.Body = guard
			next(&limitedWriter{ResponseWriter: w, guard: guard}, r)
		}
	}
}

type bodyGuard struct {
	io.ReadCloser
	tripped bool
}

func (g *bodyGuard) Read(p []byte) (int, error) {
	n, err := g.ReadCloser.Read(p)
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		g.tripped = true
	}
	return n, err
}

type limitedWriter struct {
	http.ResponseWriter
	guard   *bodyGuard
	written bool
	dropped bool
}

func (l *limitedWriter) WriteHeader(status int) {
	if l.written {
		return
	}
	l.written = true
	if l.guard.tripped {
		l.dropped = true
		writeError(l.ResponseWriter, http.StatusRequestEntityTooLarge, "corpo da requisição grande demais")
		return
	}
	l.ResponseWriter.WriteHeader(status)
}

func (l *limitedWriter) Write(p []byte) (int, error) {
	if !l.written {
		l.WriteHeader(http.StatusOK)
	}
	if l.dropped {
		return len(p), nil
	}
	return l.ResponseWriter.Write(p)
}

func allowMethods(methods ...string) middleware {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			for _, m := range methods {
				if r.Method == m {
					next(w, r)
					return
				}
			}
			w.Header().Set("Allow", strings.Join(methods, ", "))
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	}
}

const (
	auditAll        = false
	auditOnlyDenied = true
)

var auditVerbs = map[string]string{
	http.MethodPost:   "create",
	http.MethodPut:    "update",
	http.MethodPatch:  "update",
	http.MethodDelete: "delete",
}

var auditRouteActions = map[string]string{
	"/api/ssl/import":       "ssl.import",
	"/api/ssl/recheck":      "ssl.recheck",
	"/api/ssl/recheck-all":  "ssl.recheck-all",
	"/api/network/scan":     "network.scan",
	"/api/auth/login":       "auth.login",
	"/api/enroll":           "device.enroll-http",
	"/api/enroll/tokens":    "enroll-token.create",
	"/api/devices":          "device.revoke",
	"/api/auth/logout":      "auth.logout",
	"/api/stream-ticket":    "stream-ticket.create",
	"/api/alerts/ack":       "alert.ack",
	"/api/alerts/resolve":   "alert.resolve",
	"/api/ingest/metrics":   "ingest.metrics",
	"/api/ingest/inventory": "ingest.inventory",
}

var auditRouteResources = map[string]string{
	"/api/servers":      "server",
	"/api/ssl/domains":  "ssl-domain",
	"/api/network/host": "network-host",
	"/api/sites":        "site",
	"/api/floorplans":   "floorplan",
	"/api/alerts/rules": "alert-rule",
	"/api/users":        "user",
	"/api/dashboards":   "dashboards",
	"/api/annotations":  "annotations",
}

var auditQueryAllowlist = []string{
	"server_id", "site_id", "id", "plan_id", "container_name", "domain", "ip", "host_ip",
}

func (c Config) audit(onlyDenied bool) middleware {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet || r.Method == http.MethodHead {
				next(w, r)
				return
			}

			aw := &auditWriter{ResponseWriter: w, status: http.StatusOK}
			r, target := withAuditTarget(r)
			next(aw, r)

			if target.handled {
				return
			}
			result := auditResultFor(aw.status)
			if onlyDenied && result != audit.ResultDenied {
				return
			}

			sess := sessionFrom(r)
			if target.sessSet {
				sess = target.sess
			}
			e := c.auditActorOf(sess, r)
			e.Action = auditAction(r)
			e.TargetType, _, _ = strings.Cut(e.Action, ".")
			e.TargetID = auditTargetID(r)
			e.SiteID = auditSiteID(r)
			e.Result = result
			e.Detail = auditDetail(r, aw.status)
			applyAuditTarget(&e, target)
			audit.Record(e)
		}
	}
}

func applyAuditTarget(e *audit.Entry, target *auditTargetInfo) {
	if target == nil || !target.set {
		return
	}
	if target.typ != "" {
		e.TargetType = target.typ
	}
	if target.id != "" {
		e.TargetID = target.id
	}
	e.TargetLabel = target.label
	e.SiteID = target.siteID
}

type auditWriter struct {
	http.ResponseWriter
	status  int
	written bool
}

func (a *auditWriter) WriteHeader(status int) {
	if !a.written {
		a.status = status
		a.written = true
	}
	a.ResponseWriter.WriteHeader(status)
}

func (a *auditWriter) Write(p []byte) (int, error) {
	a.written = true
	return a.ResponseWriter.Write(p)
}

func (a *auditWriter) Flush() {
	if f, ok := a.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func auditResultFor(status int) string {
	switch {
	case status >= 200 && status < 300:
		return audit.ResultOK
	case status == http.StatusUnauthorized,
		status == http.StatusForbidden,
		status == http.StatusNotFound,
		status == http.StatusTooManyRequests:
		return audit.ResultDenied
	default:
		return audit.ResultError
	}
}

func auditAction(r *http.Request) string {
	path := r.URL.Path
	if a, ok := auditRouteActions[path]; ok {
		return a
	}

	resource, ok := auditRouteResources[path]
	if !ok && strings.HasPrefix(path, "/api/floorplans/") {
		resource, ok = "floorplan", true
	}
	if !ok {
		resource = "desconhecido"
	}

	verb, ok := auditVerbs[r.Method]
	if !ok {
		verb = strings.ToLower(r.Method)
	}
	return resource + "." + verb
}

func auditTargetID(r *http.Request) string {
	q := r.URL.Query()
	for _, k := range []string{"id", "server_id", "container_name", "domain"} {
		if v := strings.TrimSpace(q.Get(k)); v != "" {
			return v
		}
	}
	if rest := strings.TrimPrefix(r.URL.Path, "/api/floorplans/"); rest != r.URL.Path {
		if seg, _, _ := strings.Cut(rest, "/"); seg != "" {
			return seg
		}
	}
	return ""
}

func auditSiteID(r *http.Request) *uint {
	raw := strings.TrimSpace(r.URL.Query().Get("site_id"))
	if raw == "" {
		return nil
	}
	id, err := strconv.ParseUint(raw, 10, 32)
	if err != nil || id == 0 {
		return nil
	}
	u := uint(id)
	return &u
}

func auditDetail(r *http.Request, status int) map[string]any {
	d := map[string]any{
		"metodo": r.Method,
		"rota":   r.URL.Path,
		"status": status,
	}
	q := r.URL.Query()
	for _, k := range auditQueryAllowlist {
		if v := strings.TrimSpace(q.Get(k)); v != "" {
			d[k] = v
		}
	}
	return d
}

type siteScope struct {
	filter     bool
	includeNil bool
	ids        []uint
}

func parseSiteScope(r *http.Request) (siteScope, bool) {
	raw := strings.TrimSpace(r.URL.Query().Get("site_id"))
	switch {
	case raw == "", raw == "all":
		return siteScope{}, true
	case raw == "none":
		return siteScope{filter: true, includeNil: true}, true
	}

	n, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || n == 0 {
		return siteScope{}, false
	}
	return siteScope{filter: true, ids: []uint{uint(n)}}, true
}

func resolveScope(sess auth.Session, r *http.Request) (siteScope, int) {
	requested, ok := parseSiteScope(r)
	if !ok {
		return siteScope{}, http.StatusBadRequest
	}

	if auth.HasGlobal(sess.Accesses) {
		return requested, 0
	}

	allowed := auth.SiteIDs(sess.Accesses)

	if !requested.filter {
		return siteScope{filter: true, ids: allowed}, 0
	}
	if requested.includeNil {
		return siteScope{}, http.StatusForbidden
	}
	for _, id := range requested.ids {
		if !auth.CanSeeSite(sess.Accesses, &id) {
			return siteScope{}, http.StatusForbidden
		}
	}
	return requested, 0
}

func (s siteScope) apply(tx *gorm.DB) *gorm.DB {
	switch {
	case !s.filter:
		return tx
	case s.includeNil && len(s.ids) == 0:
		return tx.Where("site_id IS NULL")
	case s.includeNil:
		return tx.Where("site_id IN ? OR site_id IS NULL", s.ids)
	case len(s.ids) == 0:
		return tx.Where("1 = 0")
	default:
		return tx.Where("site_id IN ?", s.ids)
	}
}

func (s siteScope) matches(siteID *uint) bool {
	if !s.filter {
		return true
	}
	if siteID == nil {
		return s.includeNil
	}
	for _, id := range s.ids {
		if id == *siteID {
			return true
		}
	}
	return false
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("[API] erro ao escrever resposta: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

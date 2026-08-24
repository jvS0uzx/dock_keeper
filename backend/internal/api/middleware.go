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

	"github.com/joaov/vd_stats/internal/audit"
	"github.com/joaov/vd_stats/internal/auth"
	"gorm.io/gorm"
)

// Config concentra o que a API lê do ambiente. Carregado uma vez no boot para
// não chamar os.Getenv a cada request.
type Config struct {
	Addr           string
	Token          string
	AllowedOrigins []string
	SSHKeyPath     string

	// TrustProxyHeaders libera a leitura de X-Real-IP e X-Forwarded-For para
	// descobrir a origem da requisição. Desligado por padrão: sem proxy à
	// frente esses cabeçalhos vêm do próprio cliente, e acreditar neles faria
	// do limite de login enfeite.
	TrustProxyHeaders bool

	// tickets autoriza as rotas de SSE sem expor o API_TOKEN na URL.
	tickets *ticketStore
	// logins segura a força bruta e o custo de bcrypt da rota de login.
	logins *loginLimiter
}

// LoadConfig lê o ambiente e valida o que é obrigatório para subir.
func LoadConfig(addr string) (Config, error) {
	cfg := Config{
		Addr:              addr,
		Token:             os.Getenv("API_TOKEN"),
		SSHKeyPath:        os.Getenv("SSH_KEY_PATH"),
		TrustProxyHeaders: strings.EqualFold(strings.TrimSpace(os.Getenv("TRUST_PROXY_HEADERS")), "true"),
		tickets:           newTicketStore(),
		logins: newLoginLimiter(
			envDuration("LOGIN_RATE_WINDOW", defaultLoginWindow),
			envInt("LOGIN_RATE_MAX_IP", defaultLoginMaxPerIP),
			envInt("LOGIN_RATE_MAX_USER", defaultLoginMaxPerUser),
		),
	}

	for _, o := range strings.Split(os.Getenv("ALLOWED_ORIGINS"), ",") {
		if o = strings.TrimSpace(o); o != "" {
			cfg.AllowedOrigins = append(cfg.AllowedOrigins, o)
		}
	}
	if len(cfg.AllowedOrigins) == 0 {
		cfg.AllowedOrigins = []string{"http://localhost:5173"}
		log.Printf("[API] ALLOWED_ORIGINS não definido; liberando apenas %s", cfg.AllowedOrigins[0])
	}

	// Fail-closed: sem token a API exporia SSH root de todas as VPS a qualquer
	// um que alcance a porta. Preferimos não subir a subir aberto.
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

// withCORS responde ao preflight e devolve o Origin apenas se ele estiver na
// allowlist. Refletir "*" impediria o uso de credenciais e liberaria qualquer
// página da internet a falar com a API pelo browser do operador.
func (c Config) withCORS(next http.HandlerFunc) http.HandlerFunc {
	allowed := make(map[string]bool, len(c.AllowedOrigins))
	for _, o := range c.AllowedOrigins {
		allowed[o] = true
	}

	return func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		// Vary é obrigatório: sem ele o cache guarda a resposta de uma origem
		// e devolve para outra.
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

// sessionKey guarda a sessão autenticada no contexto da requisição, para os
// handlers aplicarem o recorte por unidade sem repetir a autenticação.
type sessionCtxKey struct{}

// machineSession é a identidade sintética do API_TOKEN: máquina (agente,
// coletor, script) equivale a um admin global — não há pessoa para restringir.
var machineSession = auth.Session{
	Username: "api-token",
	Role:     auth.RoleAdmin,
	Accesses: []auth.Access{{SiteID: nil, Role: auth.RoleAdmin}},
}

// sessionFrom recupera a sessão posta pelo middleware.
//
// Falha fechado: sem sessão no contexto devolve uma sessão sem concessão
// nenhuma, que não alcança unidade alguma. O fallback anterior devolvia o
// admin global de máquina, e bastava um handler ser alcançado sem passar pelo
// gate — era o caso das rotas de SSE — para o recorte por unidade evaporar.
func sessionFrom(r *http.Request) auth.Session {
	if s, ok := r.Context().Value(sessionCtxKey{}).(auth.Session); ok {
		return s
	}
	return auth.Session{}
}

// withSession devolve a requisição carregando a sessão para os handlers.
//
// Também avisa a auditoria: o middleware que grava a linha corre antes deste
// ponto, numa requisição que ainda não conhece ninguém, e o valor mutável que
// ele deixou no contexto é a única via para o ator chegar até lá.
func withSession(r *http.Request, s auth.Session) *http.Request {
	if info, ok := r.Context().Value(auditTargetCtxKey{}).(*auditTargetInfo); ok {
		info.sess = s
		info.sessSet = true
	}
	return r.WithContext(context.WithValue(r.Context(), sessionCtxKey{}, s))
}

// requireAuth exige credencial por cabeçalho: Authorization: Bearer ou
// X-API-Token. Nunca por query string — a URL vai parar no access log do
// Nginx, no histórico do browser e em qualquer proxy do caminho.
//
// Aceita dois tipos de credencial: sessão de usuário (pessoa, com papéis por
// unidade) e o API_TOKEN (máquina), que vale como admin global.
func (c Config) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return c.requireRole(auth.RoleViewer)(next)
}

// checkRole autentica e valida o papel mínimo. O gate usa o MAIOR papel do
// usuário em qualquer escopo; o recorte fino por unidade fica nos handlers,
// que leem a sessão do contexto.
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
		next(w, withSession(r, machineSession))
		return
	}
	writeError(w, http.StatusUnauthorized, "unauthorized")
}

// requireRole exige o papel mínimo em toda a rota.
func (c Config) requireRole(minRole string) middleware {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			c.checkRole(w, r, minRole, next)
		}
	}
}

// requireRoleByMethod separa leitura de escrita na mesma rota: GET/HEAD pedem
// o papel de leitura, o resto pede o de escrita. É o que garante que o
// Visualizador liste tudo sem conseguir cadastrar nada.
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

// requireGlobalWrite protege recurso de infraestrutura sem unidade (container
// de VPS, SSL, regras de alerta, varredura do painel): leitura vale para
// qualquer papel, mas a escrita exige o papel em concessão GLOBAL. Sem isso um
// Suporte TI restrito a uma filial mandaria parar container de VPS — o gate
// grosso só olha o maior papel, não onde ele vale.
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

// requireGlobalRole exige o papel em concessão GLOBAL, em todos os métodos —
// inclusive na leitura.
//
// Cadastro de servidor e gestão de usuários não pertencem a unidade nenhuma:
// quem administra uma filial não pode listar o parque inteiro, cadastrar VPS
// com chave SSH root nem se promover a admin global. O gate grosso olha o
// maior papel do usuário em qualquer escopo, não onde ele vale, então sozinho
// ele deixaria o admin de uma filial passar.
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

// requireTicket autoriza uma rota de SSE. O EventSource do browser não permite
// cabeçalhos, então aceita ?ticket= — de uso único e válido por 30s. O
// cabeçalho continua valendo para clientes que conseguem enviá-lo (curl, agente).
func (c Config) requireTicket(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if session, ok := auth.Lookup(bearerToken(r)); ok {
			next(w, withSession(r, session))
			return
		}
		if c.tokenMatches(r) {
			next(w, withSession(r, machineSession))
			return
		}
		// O ticket carrega a sessão de quem o pediu: o stream corre com o
		// alcance daquela pessoa, não com o do processo.
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

// Tetos de corpo por natureza de rota. Não são estimativas justas do payload:
// existem para que um envio de gigabytes seja recusado antes de virar memória,
// e por isso ficam bem acima do maior corpo legítimo de cada grupo.
const (
	// Formulário e JSON de cadastro. Folgado para o maior deles — a lista de
	// pins de uma planta e a lista de concessões de um usuário.
	maxFormBodyBytes = 128 << 10
	// Inventário de uma unidade: os 5000 hosts do teto de maxInventoryHosts
	// cabem em cerca de 1 MB, e o excedente aqui é margem para hostname longo.
	maxIngestBodyBytes = 4 << 20
)

// limitBody recusa corpo acima do teto antes de o handler alocá-lo.
//
// Fica na cadeia e não dentro de cada handler porque são treze rotas
// decodificando JSON: um teto esquecido em uma delas já basta para o problema
// continuar de pé.
func limitBody(max int64) middleware {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Body == nil || r.Body == http.NoBody {
				next(w, r)
				return
			}
			// Content-Length acima do teto morre aqui, sem ler um byte do corpo.
			if r.ContentLength > max {
				writeError(w, http.StatusRequestEntityTooLarge, "corpo da requisição grande demais")
				return
			}
			// O MaxBytesReader recebe o w original de propósito: é por ele que
			// o net/http descobre que precisa fechar a conexão em vez de
			// esperar o resto de um corpo que não vai aceitar.
			guard := &bodyGuard{ReadCloser: http.MaxBytesReader(w, r.Body, max)}
			r.Body = guard
			next(&limitedWriter{ResponseWriter: w, guard: guard}, r)
		}
	}
}

// bodyGuard marca que o teto estourou durante a leitura.
//
// Só faz falta quando o cliente não declara Content-Length (transferência
// chunked), porque aí o tamanho só aparece lendo. Sem a marca o handler
// devolveria 400 "corpo inválido", que manda o operador procurar erro de
// sintaxe onde o problema é tamanho.
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

// limitedWriter troca a resposta do handler por 413 quando o corpo estourou o
// teto. A troca acontece no WriteHeader porque depois de escrito o status já
// não pode ser mudado.
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

// allowMethods rejeita verbos fora da lista antes de o handler tocar no banco.
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

// Modos do middleware de auditoria. Constantes nomeadas porque um booleano
// solto no call site não diz qual dos dois comportamentos está sendo pedido.
const (
	auditAll        = false
	auditOnlyDenied = true
)

// auditVerbs traduz o método HTTP para o verbo do nome da ação.
var auditVerbs = map[string]string{
	http.MethodPost:   "create",
	http.MethodPut:    "update",
	http.MethodPatch:  "update",
	http.MethodDelete: "delete",
}

// auditRouteActions nomeia as rotas cujo último segmento já é um verbo, e para
// as quais "recurso.create" mentiria: POST /api/ssl/recheck não cria nada.
var auditRouteActions = map[string]string{
	"/api/ssl/import":      "ssl.import",
	"/api/ssl/recheck":     "ssl.recheck",
	"/api/ssl/recheck-all": "ssl.recheck-all",
	"/api/network/scan":    "network.scan",
	"/api/auth/login":      "auth.login",
	// device.enroll é gravado pelo próprio handler, com o motivo da recusa e o
	// device_id emitido — o middleware só saberia dizer que houve um POST.
	"/api/enroll":           "device.enroll-http",
	"/api/enroll/tokens":    "enroll-token.create",
	"/api/devices":          "device.revoke",
	"/api/auth/logout":      "auth.logout",
	"/api/stream-ticket":    "stream-ticket.create",
	"/api/ingest/metrics":   "ingest.metrics",
	"/api/ingest/inventory": "ingest.inventory",
}

// auditRouteResources dá o nome do recurso às rotas REST comuns.
//
// Tabela explícita em vez de derivação do caminho: o caminho carrega o prefixo
// /api/, plural inconsistente e segmento dinâmico. O nome da ação é a chave que
// alguém vai usar para filtrar a auditoria depois de um incidente, então ele
// precisa ser estável, e não subproduto do roteamento.
var auditRouteResources = map[string]string{
	"/api/servers":      "server",
	"/api/ssl/domains":  "ssl-domain",
	"/api/network/host": "network-host",
	"/api/sites":        "site",
	"/api/floorplans":   "floorplan",
	"/api/alerts/rules": "alert-rule",
	"/api/users":        "user",
}

// auditQueryAllowlist são os parâmetros de query que podem entrar no detalhe.
//
// Allowlist e não blocklist: parâmetro novo nasce FORA do log, então esquecer
// de atualizar esta lista perde informação — esquecer de atualizar uma
// blocklist vazaria credencial. "ticket" está ausente de propósito: é a
// credencial de uso único das rotas de SSE.
var auditQueryAllowlist = []string{
	"server_id", "site_id", "id", "plan_id", "container_name", "domain", "ip", "host_ip",
}

// audit registra as escritas do painel na fronteira da cadeia.
//
// Fica no wrapper, e não dentro de cada handler, porque é isso que garante a
// cobertura: handler novo que esqueça de auditar não existe, já que o registro
// está no mesmo wrapper que ele precisa usar para ter CORS e gate de papel.
//
// onlyDenied atende as rotas de ingestão — ver o comentário em Routes.
func (c Config) audit(onlyDenied bool) middleware {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			// Leitura não é escrita. Auditar GET encheria a tabela com o
			// polling do painel e afogaria justamente o que se procura nela.
			if r.Method == http.MethodGet || r.Method == http.MethodHead {
				next(w, r)
				return
			}

			aw := &auditWriter{ResponseWriter: w, status: http.StatusOK}
			r, target := withAuditTarget(r)
			next(aw, r)

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

// applyAuditTarget sobrepõe o que o handler soube sobre o alvo.
//
// O que o middleware derivou da URL fica como piso: handler que não chamou
// auditTarget gera exatamente a linha de antes. Tipo e id vazios não apagam o
// que já havia — um handler pode conhecer só o rótulo —, mas a unidade é sempre
// a que o handler afirmou, inclusive nula.
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

// auditWriter guarda o status da resposta para o middleware saber se a ação foi
// aceita, recusada ou quebrou.
//
// É um segundo envelope, e não uma extensão do limitedWriter, porque os dois
// resolvem problemas diferentes e nem sempre andam juntos: o teto de corpo
// TROCA a resposta, a auditoria só a OBSERVA. Fundir os dois faria o registro
// da ação depender do teto de corpo estar na cadeia.
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
	// Write sem WriteHeader é 200 implícito, que já é o valor inicial.
	a.written = true
	return a.ResponseWriter.Write(p)
}

// Flush repassa para o writer de baixo.
//
// As rotas de SSE não passam por este middleware hoje, mas startSSE decide se
// há streaming por type assertion para http.Flusher: um envelope que não a
// satisfaça faz o painel parar de receber dado em tempo real sem nenhum erro.
// O envelope não pode ser a razão pela qual mover uma rota de wrapper quebra
// em silêncio.
func (a *auditWriter) Flush() {
	if f, ok := a.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// auditResultFor traduz o status HTTP para o vocabulário de três valores da
// auditoria.
//
// 429 entra como recusa porque é isso que ele é: o limite de tentativa negando
// a ação. Os demais 4xx entram como erro — corpo inválido e método errado são
// defeito de quem chamou, não decisão de autorização, e misturá-los com denied
// sujaria justamente a consulta que procura quem tentou alcançar o que não
// podia.
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

// auditAction devolve o nome da ação no formato recurso.verbo.
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
		// Rota sem entrada na tabela ainda gera linha. Perder o registro de uma
		// escrita por causa de um nome que ninguém cadastrou é pior que gravar
		// um nome feio, e o "desconhecido" na consulta denuncia a omissão.
		resource = "desconhecido"
	}

	verb, ok := auditVerbs[r.Method]
	if !ok {
		verb = strings.ToLower(r.Method)
	}
	return resource + "." + verb
}

// auditTargetID identifica o alvo pelo que a rota expõe fora do corpo. O corpo
// não é lido aqui em nenhuma hipótese.
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

// auditDetail monta o detalhe da linha.
//
// O corpo da requisição NUNCA entra aqui. Copiá-lo transformaria a auditoria
// num depósito de senha em claro no primeiro POST /api/auth/login.
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

// siteScope é o recorte por unidade aplicado às consultas.
//
// Com várias unidades o painel não pode carregar tudo e filtrar no browser:
// o corte precisa acontecer no WHERE. "none" seleciona o que ainda não foi
// classificado — as VPS de infraestrutura caem aí de propósito.
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

// resolveScope cruza o que a requisição pediu com o que a sessão alcança.
// O filtro deixa de ser conveniência e vira permissão: usuário restrito a
// unidades nunca recebe dados de fora delas, peça o que pedir.
//
// Devolve o recorte efetivo e um status HTTP (0 = ok).
func resolveScope(sess auth.Session, r *http.Request) (siteScope, int) {
	requested, ok := parseSiteScope(r)
	if !ok {
		return siteScope{}, http.StatusBadRequest
	}

	// Acesso global: o pedido vale como veio (inclui "all" e "none").
	if auth.HasGlobal(sess.Accesses) {
		return requested, 0
	}

	allowed := auth.SiteIDs(sess.Accesses)

	// Sem filtro explícito, o "tudo" de um usuário restrito é a união das
	// unidades dele — não o parque inteiro.
	if !requested.filter {
		return siteScope{filter: true, ids: allowed}, 0
	}
	// "none" é o escopo sem unidade (VPS/Dev): exige concessão global.
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

// apply adiciona o recorte à consulta.
func (s siteScope) apply(tx *gorm.DB) *gorm.DB {
	switch {
	case !s.filter:
		return tx
	case s.includeNil && len(s.ids) == 0:
		return tx.Where("site_id IS NULL")
	case s.includeNil:
		return tx.Where("site_id IN ? OR site_id IS NULL", s.ids)
	case len(s.ids) == 0:
		// Usuário restrito sem nenhuma unidade: não vê nada.
		return tx.Where("1 = 0")
	default:
		return tx.Where("site_id IN ?", s.ids)
	}
}

// matches diz se um registro já carregado entra no recorte.
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

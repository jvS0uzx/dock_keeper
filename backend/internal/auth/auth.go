// Package auth cuida de usuários, sessões e papéis do painel.
//
// Convive com o API_TOKEN em vez de substituí-lo: o token continua servindo
// para tráfego máquina-a-máquina (agente, coletor, script), enquanto pessoas
// entram com usuário e senha e recebem um papel. Trocar tudo de uma vez
// derrubaria as integrações já instaladas.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/joaov/vd_stats/internal/database"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// Papéis, do menor para o maior privilégio.
const (
	RoleViewer   = "viewer"   // só leitura
	RoleOperator = "operator" // + ações em container, varredura, cadastro
	RoleAdmin    = "admin"    // + usuários e servidores
)

var roleRank = map[string]int{RoleViewer: 0, RoleOperator: 1, RoleAdmin: 2}

// ValidRole diz se o papel é conhecido.
func ValidRole(r string) bool {
	_, ok := roleRank[r]
	return ok
}

// Allows diz se quem tem `has` alcança o nível `needs`.
func Allows(has, needs string) bool {
	h, ok := roleRank[has]
	if !ok {
		return false
	}
	n, ok := roleRank[needs]
	return ok && h >= n
}

// Access é uma concessão de papel num escopo: SiteID nulo vale para todas as
// unidades e para o que não tem unidade (VPS, painel Dev); SiteID preenchido
// vale só para aquela unidade.
type Access struct {
	SiteID *uint  `json:"site_id"`
	Role   string `json:"role"`
}

// rankOf trata papel desconhecido (e o vazio) como abaixo de qualquer papel
// válido. Sem isso o viewer — rank zero — nunca venceria o vazio numa
// comparação de mapa, que também devolve zero para chave ausente.
func rankOf(role string) int {
	if r, ok := roleRank[role]; ok {
		return r
	}
	return -1
}

// MaxRole é o maior papel do usuário em qualquer escopo. Usado no gate grosso
// de rota; o recorte fino por unidade acontece nos handlers.
func MaxRole(accesses []Access) string {
	best := ""
	for _, a := range accesses {
		if rankOf(a.Role) > rankOf(best) {
			best = a.Role
		}
	}
	return best
}

// HasGlobal diz se o usuário tem alguma concessão sem unidade — quem não tem
// nunca enxerga o escopo "none" (VPS/Dev) nem o parque inteiro.
func HasGlobal(accesses []Access) bool {
	for _, a := range accesses {
		if a.SiteID == nil {
			return true
		}
	}
	return false
}

// GlobalRole é o maior papel entre as concessões globais ("" se não houver).
func GlobalRole(accesses []Access) string {
	best := ""
	for _, a := range accesses {
		if a.SiteID == nil && rankOf(a.Role) > rankOf(best) {
			best = a.Role
		}
	}
	return best
}

// RoleForSite resolve o papel efetivo num alvo. Alvo sem unidade (nil) só é
// alcançado por concessão global; alvo com unidade aceita a concessão global
// ou a da própria unidade — vence a maior.
func RoleForSite(accesses []Access, siteID *uint) string {
	best := GlobalRole(accesses)
	if siteID == nil {
		return best
	}
	for _, a := range accesses {
		if a.SiteID != nil && *a.SiteID == *siteID && rankOf(a.Role) > rankOf(best) {
			best = a.Role
		}
	}
	return best
}

// CanSeeSite diz se o alvo é visível — qualquer papel enxerga.
func CanSeeSite(accesses []Access, siteID *uint) bool {
	return RoleForSite(accesses, siteID) != ""
}

// SiteIDs lista as unidades citadas nas concessões (sem as globais).
func SiteIDs(accesses []Access) []uint {
	var ids []uint
	seen := map[uint]bool{}
	for _, a := range accesses {
		if a.SiteID != nil && !seen[*a.SiteID] {
			seen[*a.SiteID] = true
			ids = append(ids, *a.SiteID)
		}
	}
	return ids
}

// SessionTTL é a duração da sessão. Curta o bastante para uma aba esquecida
// não virar acesso permanente, longa o bastante para um turno de trabalho.
const SessionTTL = 12 * time.Hour

// Hash de descarte usado para gastar o mesmo tempo quando o usuário não
// existe. Sem isso o tempo de resposta revelaria quais nomes estão cadastrados.
const dummyHash = "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"

var (
	ErrInvalidCredentials = errors.New("usuário ou senha inválidos")
	ErrUserInactive       = errors.New("usuário desativado")
	ErrWeakPassword       = errors.New("a senha precisa de ao menos 10 caracteres")
	ErrInvalidRole        = errors.New("papel inválido: use viewer, operator ou admin")

	// ErrSessionStore aparece quando não há onde gravar a sessão. É erro de
	// infraestrutura, não de credencial: quem chama não deve traduzi-lo para
	// "usuário ou senha inválidos", que mandaria a pessoa procurar defeito na
	// própria senha.
	ErrSessionStore = errors.New("armazenamento de sessão indisponível")
)

// Session é um login ativo. Role é o papel MÁXIMO entre as concessões — o
// recorte por unidade vem de Accesses.
type Session struct {
	Token     string    `json:"token"`
	UserID    uint      `json:"user_id"`
	Username  string    `json:"username"`
	Role      string    `json:"role"`
	ExpiresAt time.Time `json:"expires_at"`
	Accesses  []Access  `json:"accesses"`
}

// HashPassword gera o hash bcrypt, validando o tamanho mínimo antes.
func HashPassword(plain string) (string, error) {
	if len([]rune(strings.TrimSpace(plain))) < 10 {
		return "", ErrWeakPassword
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	return string(hash), err
}

// Login confere as credenciais e abre uma sessão.
func Login(username, password string) (Session, error) {
	var user database.User
	err := database.DB.
		Where("username = ?", strings.ToLower(strings.TrimSpace(username))).
		First(&user).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		_ = bcrypt.CompareHashAndPassword([]byte(dummyHash), []byte(password))
		return Session{}, ErrInvalidCredentials
	}
	if err != nil {
		return Session{}, err
	}
	// O bcrypt vem antes da conferência de Active. Recusar a conta desativada
	// primeiro respondia em microssegundos enquanto senha errada e usuário
	// inexistente gastavam 60-100 ms, e esse intervalo entrega a quem está de
	// fora que a conta existe. A resposta HTTP dos dois erros já é a mesma.
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
		return Session{}, ErrInvalidCredentials
	}
	if !user.Active {
		return Session{}, ErrUserInactive
	}

	now := time.Now()
	database.DB.Model(&user).Update("last_login", &now)

	accesses, err := loadAccesses(user)
	if err != nil {
		return Session{}, err
	}
	return CreateSession(user.ID, user.Username, accesses)
}

// loadAccesses busca as concessões por unidade. Usuário sem nenhuma linha
// mantém o comportamento antigo: o papel da conta vale globalmente.
func loadAccesses(user database.User) ([]Access, error) {
	var rows []database.UserSiteAccess
	if err := database.DB.Where("user_id = ?", user.ID).Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return []Access{{SiteID: nil, Role: user.Role}}, nil
	}
	accesses := make([]Access, 0, len(rows))
	for _, r := range rows {
		accesses = append(accesses, Access{SiteID: r.SiteID, Role: r.Role})
	}
	return accesses, nil
}

// lastSeenRefresh evita uma escrita por requisição. Marcar o último uso a cada
// chamada transformaria toda leitura autenticada num UPDATE; o valor serve para
// o administrador saber se a sessão está viva, não para contabilidade.
const lastSeenRefresh = 5 * time.Minute

// tokenHash reduz o token à chave de busca.
//
// SHA-256 e não bcrypt de propósito: o token tem 256 bits vindos de
// crypto/rand, então não existe dicionário a testar, e a busca acontece em toda
// requisição autenticada — um KDF caro cobraria os 60-100 ms do bcrypt por
// requisição sem comprar segurança nenhuma.
func tokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// CreateSession abre uma sessão já com as concessões resolvidas. Exportada
// para o Login, para os testes e para um futuro SSO.
//
// O parâmetro accesses vale para a sessão devolvida agora; a autorização das
// requisições seguintes é relida do banco a cada Lookup. Quem chama isto com
// concessões que não existem em user_site_accesses recebe uma sessão que não se
// sustenta na próxima requisição — foi assim que os testes que fabricavam
// sessão para usuário inexistente pararam de funcionar.
func CreateSession(userID uint, username string, accesses []Access) (Session, error) {
	if database.DB == nil {
		return Session{}, ErrSessionStore
	}

	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return Session{}, err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)

	now := time.Now()
	session := Session{
		Token:     token,
		UserID:    userID,
		Username:  username,
		Role:      MaxRole(accesses),
		ExpiresAt: now.Add(SessionTTL),
		Accesses:  accesses,
	}

	// A poda acompanha a criação, no mesmo padrão do ticketStore. Sessão
	// vencida só se acumula enquanto gente entra, então quem entra paga a
	// conta e a tabela não precisa de rotina própria.
	purgeExpiredSessions(now)

	row := database.UserSession{
		TokenHash:  tokenHash(token),
		UserID:     userID,
		Role:       session.Role,
		Username:   username,
		ExpiresAt:  session.ExpiresAt,
		CreatedAt:  now,
		LastSeenAt: now,
	}
	if err := database.DB.Create(&row).Error; err != nil {
		return Session{}, err
	}
	return session, nil
}

// Lookup devolve a sessão do token, se ainda válida.
//
// A linha da tabela é só a credencial: quem a pessoa é e o que ela alcança sai
// do estado ATUAL do banco a cada chamada. Congelar as concessões no momento do
// login faria remover o acesso de alguém a uma unidade só valer no próximo
// login — e "revoguei e continuou entrando" é o pior defeito possível num
// controle de acesso.
//
// Falha fechado em toda dúvida: banco fora, linha ausente, conta apagada ou
// desativada devolvem sessão inválida. É o mesmo precedente de sessionFrom, que
// devolve sessão sem concessão em vez de conceder.
func Lookup(token string) (Session, bool) {
	if token == "" || database.DB == nil {
		return Session{}, false
	}

	var row database.UserSession
	if err := database.DB.First(&row, "token_hash = ?", tokenHash(token)).Error; err != nil {
		return Session{}, false
	}

	now := time.Now()
	if now.After(row.ExpiresAt) {
		deleteSession(row.TokenHash)
		return Session{}, false
	}

	var user database.User
	err := database.DB.First(&user, row.UserID).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		// A conta sumiu: a sessão não tem mais dono e morre junto.
		deleteSession(row.TokenHash)
		return Session{}, false
	case err != nil:
		// Banco indisponível é falha de autenticação, não sucesso — e não é
		// motivo para destruir a sessão de ninguém: a falha passa, a sessão
		// precisa continuar existindo quando o banco voltar.
		return Session{}, false
	}
	if !user.Active {
		deleteSession(row.TokenHash)
		return Session{}, false
	}

	accesses, err := loadAccesses(user)
	if err != nil || len(accesses) == 0 {
		return Session{}, false
	}

	touchSession(row, now)

	return Session{
		Token:     token,
		UserID:    user.ID,
		Username:  user.Username,
		Role:      MaxRole(accesses),
		ExpiresAt: row.ExpiresAt,
		Accesses:  accesses,
	}, true
}

// Logout invalida a sessão.
func Logout(token string) {
	if token == "" || database.DB == nil {
		return
	}
	deleteSession(tokenHash(token))
}

// RevokeUser derruba todas as sessões de um usuário — usado ao desativar ou
// trocar o papel de alguém, para a mudança valer na hora.
//
// Continua existindo mesmo com as concessões sendo relidas a cada requisição:
// reler cobre mudança de alcance, mas quem foi desativado ou teve o papel
// rebaixado não deve nem terminar a requisição em curso.
func RevokeUser(userID uint) {
	if database.DB == nil {
		return
	}
	if err := database.DB.Where("user_id = ?", userID).
		Delete(&database.UserSession{}).Error; err != nil {
		log.Printf("[Auth] erro ao revogar as sessões do usuário %d: %v", userID, err)
	}
}

func deleteSession(hash string) {
	if err := database.DB.Where("token_hash = ?", hash).
		Delete(&database.UserSession{}).Error; err != nil {
		log.Printf("[Auth] erro ao remover a sessão: %v", err)
	}
}

func purgeExpiredSessions(now time.Time) {
	if err := database.DB.Where("expires_at < ?", now).
		Delete(&database.UserSession{}).Error; err != nil {
		log.Printf("[Auth] erro ao podar sessões vencidas: %v", err)
	}
}

// touchSession registra o último uso, no máximo uma vez a cada lastSeenRefresh.
func touchSession(row database.UserSession, now time.Time) {
	if now.Sub(row.LastSeenAt) < lastSeenRefresh {
		return
	}
	if err := database.DB.Model(&database.UserSession{}).
		Where("token_hash = ?", row.TokenHash).
		Update("last_seen_at", now).Error; err != nil {
		log.Printf("[Auth] erro ao marcar o uso da sessão: %v", err)
	}
}

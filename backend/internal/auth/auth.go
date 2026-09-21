package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/database"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

const (
	RoleViewer   = "viewer"
	RoleOperator = "operator"
	RoleAdmin    = "admin"
)

var roleRank = map[string]int{RoleViewer: 0, RoleOperator: 1, RoleAdmin: 2}

func ValidRole(r string) bool {
	_, ok := roleRank[r]
	return ok
}

func Allows(has, needs string) bool {
	h, ok := roleRank[has]
	if !ok {
		return false
	}
	n, ok := roleRank[needs]
	return ok && h >= n
}

type Access struct {
	SiteID *uint  `json:"site_id"`
	Role   string `json:"role"`
}

func rankOf(role string) int {
	if r, ok := roleRank[role]; ok {
		return r
	}
	return -1
}

func MaxRole(accesses []Access) string {
	best := ""
	for _, a := range accesses {
		if rankOf(a.Role) > rankOf(best) {
			best = a.Role
		}
	}
	return best
}

func HasGlobal(accesses []Access) bool {
	for _, a := range accesses {
		if a.SiteID == nil {
			return true
		}
	}
	return false
}

func GlobalRole(accesses []Access) string {
	best := ""
	for _, a := range accesses {
		if a.SiteID == nil && rankOf(a.Role) > rankOf(best) {
			best = a.Role
		}
	}
	return best
}

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

func CanSeeSite(accesses []Access, siteID *uint) bool {
	return RoleForSite(accesses, siteID) != ""
}

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

const DefaultSessionTTL = 12 * time.Hour

var sessionTTL = DefaultSessionTTL

func Configure() {
	sessionTTL = database.EnvDuration("SESSION_TTL", DefaultSessionTTL)
}

const dummyHash = "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"

var (
	ErrInvalidCredentials = errors.New("usuário ou senha inválidos")
	ErrUserInactive       = errors.New("usuário desativado")
	ErrWeakPassword       = errors.New("a senha precisa de ao menos 10 caracteres")
	ErrInvalidRole        = errors.New("papel inválido: use viewer, operator ou admin")

	ErrSessionStore = errors.New("armazenamento de sessão indisponível")
)

type Session struct {
	Token     string    `json:"token"`
	UserID    uint      `json:"user_id"`
	Username  string    `json:"username"`
	Role      string    `json:"role"`
	ExpiresAt time.Time `json:"expires_at"`
	Accesses  []Access  `json:"accesses"`
}

func HashPassword(plain string) (string, error) {
	if len([]rune(strings.TrimSpace(plain))) < 10 {
		return "", ErrWeakPassword
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	return string(hash), err
}

func ContaDoLogin(identificador string) string {
	identificador = strings.ToLower(strings.TrimSpace(identificador))

	if database.DB == nil {
		return identificador
	}

	var ids []uint
	err := database.DB.Model(&database.User{}).
		Where("username = ? OR (email <> '' AND email = ?)", identificador, identificador).
		Limit(1).Pluck("id", &ids).Error
	if err != nil || len(ids) == 0 {
		return identificador
	}
	return "#" + strconv.FormatUint(uint64(ids[0]), 10)
}

func Login(username, password string) (Session, error) {
	identificador := strings.ToLower(strings.TrimSpace(username))

	var user database.User
	err := database.DB.
		Where("username = ? OR (email <> '' AND email = ?)", identificador, identificador).
		First(&user).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		_ = bcrypt.CompareHashAndPassword([]byte(dummyHash), []byte(password))
		return Session{}, ErrInvalidCredentials
	}
	if err != nil {
		return Session{}, err
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
		return Session{}, ErrInvalidCredentials
	}
	if !user.Active {
		return Session{}, ErrUserInactive
	}

	now := time.Now()
	if err := database.DB.Model(&user).Update("last_login", &now).Error; err != nil {
		log.Printf("[Auth] erro ao gravar o último login de %q: %v", user.Username, err)
	}

	accesses, err := loadAccesses(user)
	if err != nil {
		return Session{}, err
	}
	return CreateSession(user.ID, user.Username, accesses)
}

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

const lastSeenRefresh = 5 * time.Minute

func tokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

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
		ExpiresAt: now.Add(sessionTTL),
		Accesses:  accesses,
	}

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
		deleteSession(row.TokenHash)
		return Session{}, false
	case err != nil:
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

func Logout(token string) {
	if token == "" || database.DB == nil {
		return
	}
	deleteSession(tokenHash(token))
}

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

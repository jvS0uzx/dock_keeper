package api

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/jvS0uzx/dock_keeper/internal/auth"
	"github.com/jvS0uzx/dock_keeper/internal/database"
	"gorm.io/gorm"
)

func (c Config) loginHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "corpo inválido")
		return
	}

	ip := clientIP(r, c.TrustProxyHeaders)
	if !c.logins.allowed(ip, req.Username) {
		w.Header().Set("Retry-After", strconv.Itoa(int(c.logins.window.Seconds())))
		writeError(w, http.StatusTooManyRequests, "tentativas demais; aguarde alguns minutos")
		return
	}

	session, err := auth.Login(req.Username, req.Password)
	switch {
	case errors.Is(err, auth.ErrInvalidCredentials), errors.Is(err, auth.ErrUserInactive):
		c.logins.fail(ip, req.Username)
		writeError(w, http.StatusUnauthorized, "usuário ou senha inválidos")
		return
	case err != nil:
		log.Printf("[Auth] erro no login de %q: %v", req.Username, err)
		writeError(w, http.StatusInternalServerError, "falha no login")
		return
	}

	c.logins.succeed(req.Username)
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, session)
}

func logoutHandler(w http.ResponseWriter, r *http.Request) {
	auth.Logout(bearerToken(r))
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (c Config) meHandler(w http.ResponseWriter, r *http.Request) {
	if session, ok := auth.Lookup(bearerToken(r)); ok {
		var user database.User
		database.DB.Where("id = ?", session.UserID).Take(&user)
		writeJSON(w, http.StatusOK, map[string]any{
			"username": session.Username,
			"nome":     user.Nome,
			"email":    user.Email,
			"role":     session.Role,
			"kind":     "user",
			"accesses": session.Accesses,
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"username": "api-token",
		"nome":     "",
		"email":    "",
		"role":     auth.RoleAdmin,
		"kind":     "token",
		"accesses": []auth.Access{{SiteID: nil, Role: auth.RoleAdmin}},
	})
}

type userView struct {
	database.User
	Accesses []auth.Access `json:"accesses"`
}

type accessPayload []struct {
	SiteID *uint  `json:"site_id"`
	Role   string `json:"role"`
}

func validateAccesses(payload accessPayload) ([]database.UserSiteAccess, error) {
	rows := make([]database.UserSiteAccess, 0, len(payload))
	for _, a := range payload {
		if !auth.ValidRole(a.Role) {
			return nil, auth.ErrInvalidRole
		}
		if a.SiteID != nil {
			var count int64
			database.DB.Model(&database.Site{}).Where("id = ?", *a.SiteID).Count(&count)
			if count == 0 {
				return nil, errInvalidSite("unidade inexistente na concessão de acesso")
			}
		}
		rows = append(rows, database.UserSiteAccess{SiteID: a.SiteID, Role: a.Role})
	}
	return rows, nil
}

func criarUsuarioComAcessos(user *database.User, rows []database.UserSiteAccess) error {
	return database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(user).Error; err != nil {
			return err
		}
		return replaceAccessesTx(tx, user.ID, rows)
	})
}

func removerUsuario(user database.User) error {
	return database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ?", user.ID).Delete(&database.UserSiteAccess{}).Error; err != nil {
			return err
		}
		if err := tx.Where("owner_user_id = ?", user.ID).Delete(&database.Dashboard{}).Error; err != nil {
			return err
		}
		return tx.Delete(&user).Error
	})
}

func replaceAccesses(userID uint, rows []database.UserSiteAccess) error {
	return database.DB.Transaction(func(tx *gorm.DB) error {
		return replaceAccessesTx(tx, userID, rows)
	})
}

func replaceAccessesTx(tx *gorm.DB, userID uint, rows []database.UserSiteAccess) error {
	if err := tx.Where("user_id = ?", userID).Delete(&database.UserSiteAccess{}).Error; err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	for i := range rows {
		rows[i].UserID = userID
		rows[i].ID = 0
	}
	return tx.Create(&rows).Error
}

func usersHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		var users []database.User
		if err := database.DB.Order("username ASC").Find(&users).Error; err != nil {
			log.Printf("[Auth] erro ao listar usuários: %v", err)
			writeError(w, http.StatusInternalServerError, "falha ao listar usuários")
			return
		}

		var rows []database.UserSiteAccess
		if err := database.DB.Find(&rows).Error; err != nil {
			log.Printf("[Auth] erro ao listar acessos: %v", err)
			writeError(w, http.StatusInternalServerError, "falha ao listar usuários")
			return
		}
		byUser := make(map[uint][]auth.Access)
		for _, r := range rows {
			byUser[r.UserID] = append(byUser[r.UserID], auth.Access{SiteID: r.SiteID, Role: r.Role})
		}

		views := make([]userView, 0, len(users))
		for _, u := range users {
			accesses := byUser[u.ID]
			if accesses == nil {
				accesses = []auth.Access{}
			}
			views = append(views, userView{User: u, Accesses: accesses})
		}
		writeJSON(w, http.StatusOK, views)

	case http.MethodPost:
		createUser(w, r)

	case http.MethodPatch:
		updateUser(w, r)

	case http.MethodDelete:
		deleteUser(w, r)
	}
}

func createUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string        `json:"username"`
		Password string        `json:"password"`
		Nome     string        `json:"nome"`
		Email    string        `json:"email"`
		Role     string        `json:"role"`
		Active   *bool         `json:"active"`
		Accesses accessPayload `json:"accesses"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "corpo inválido")
		return
	}

	username := strings.ToLower(strings.TrimSpace(req.Username))
	if username == "" {
		writeError(w, http.StatusBadRequest, "username é obrigatório")
		return
	}
	nome, email, err := identidadeValida(req.Nome, req.Email)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if emailEmUso(email, 0) {
		writeError(w, http.StatusConflict, "este e-mail já está em uso")
		return
	}
	if req.Role == "" {
		req.Role = auth.RoleViewer
	}
	if !auth.ValidRole(req.Role) {
		writeError(w, http.StatusBadRequest, auth.ErrInvalidRole.Error())
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	accessRows, err := validateAccesses(req.Accesses)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	active := true
	if req.Active != nil {
		active = *req.Active
	}
	user := database.User{
		Username:     username,
		Nome:         nome,
		Email:        email,
		PasswordHash: hash,
		Role:         req.Role,
		Active:       active,
	}
	if err := criarUsuarioComAcessos(&user, accessRows); err != nil {
		log.Printf("[Auth] erro ao criar o usuário %q: %v", username, err)
		writeError(w, http.StatusConflict, "usuário ou e-mail já existe, ou os acessos são inválidos")
		return
	}
	accesses := make([]auth.Access, 0, len(accessRows))
	for _, row := range accessRows {
		accesses = append(accesses, auth.Access{SiteID: row.SiteID, Role: row.Role})
	}
	auditUserTarget(r, user)
	writeJSON(w, http.StatusCreated, userView{User: user, Accesses: accesses})
}

func updateUser(w http.ResponseWriter, r *http.Request) {
	user, ok := userFromQuery(w, r)
	if !ok {
		return
	}

	var req struct {
		Password *string        `json:"password"`
		Nome     *string        `json:"nome"`
		Email    *string        `json:"email"`
		Role     *string        `json:"role"`
		Active   *bool          `json:"active"`
		Accesses *accessPayload `json:"accesses"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "corpo inválido")
		return
	}

	updates := map[string]any{}
	if req.Password != nil {
		hash, err := auth.HashPassword(*req.Password)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		updates["password_hash"] = hash
	}
	if req.Nome != nil || req.Email != nil {
		nomeAtual, emailAtual := user.Nome, user.Email
		if req.Nome != nil {
			nomeAtual = *req.Nome
		}
		if req.Email != nil {
			emailAtual = *req.Email
		}
		nome, email, err := identidadeValida(nomeAtual, emailAtual)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if emailEmUso(email, user.ID) {
			writeError(w, http.StatusConflict, "este e-mail já está em uso")
			return
		}
		if req.Nome != nil {
			updates["nome"] = nome
		}
		if req.Email != nil {
			updates["email"] = email
		}
	}
	if req.Role != nil {
		if !auth.ValidRole(*req.Role) {
			writeError(w, http.StatusBadRequest, auth.ErrInvalidRole.Error())
			return
		}
		updates["role"] = *req.Role
	}
	if req.Active != nil {
		if !*req.Active && isLastAdmin(user) {
			writeError(w, http.StatusConflict, "não é possível desativar o último administrador")
			return
		}
		updates["active"] = *req.Active
	}
	if len(updates) == 0 && req.Accesses == nil {
		writeError(w, http.StatusBadRequest, "nenhum campo para atualizar")
		return
	}

	if req.Accesses != nil {
		rows, err := validateAccesses(*req.Accesses)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := replaceAccesses(user.ID, rows); err != nil {
			log.Printf("[Auth] erro ao trocar acessos do usuário %d: %v", user.ID, err)
			writeError(w, http.StatusInternalServerError, "falha ao gravar os acessos")
			return
		}
	}

	if len(updates) > 0 {
		if err := database.DB.Model(&user).Updates(updates).Error; err != nil {
			log.Printf("[Auth] erro ao atualizar o usuário %d: %v", user.ID, err)
			writeError(w, http.StatusInternalServerError, "falha ao atualizar o usuário")
			return
		}
	}

	auth.RevokeUser(user.ID)
	auditUserTarget(r, user)
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func deleteUser(w http.ResponseWriter, r *http.Request) {
	user, ok := userFromQuery(w, r)
	if !ok {
		return
	}
	if isLastAdmin(user) {
		writeError(w, http.StatusConflict, "não é possível remover o último administrador")
		return
	}

	if err := removerUsuario(user); err != nil {
		log.Printf("[Auth] erro ao remover o usuário %d: %v", user.ID, err)
		writeError(w, http.StatusInternalServerError, "falha ao remover o usuário")
		return
	}
	auth.RevokeUser(user.ID)
	auditUserTarget(r, user)
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func identidadeValida(nome, email string) (string, string, error) {
	nome = strings.TrimSpace(nome)
	if len([]rune(nome)) > 120 {
		return "", "", errors.New("nome passa de 120 caracteres")
	}

	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return nome, "", nil
	}
	if len([]rune(email)) > 160 {
		return "", "", errors.New("e-mail passa de 160 caracteres")
	}
	usuario, dominio, ok := strings.Cut(email, "@")
	if !ok || usuario == "" || !strings.Contains(dominio, ".") || strings.ContainsAny(email, " \t") {
		return "", "", errors.New("e-mail inválido")
	}
	return nome, email, nil
}

func emailEmUso(email string, exceto uint) bool {
	if email == "" {
		return false
	}
	var contagem int64
	database.DB.Model(&database.User{}).
		Where("email = ? AND id <> ?", email, exceto).
		Count(&contagem)
	return contagem > 0
}

func auditUserTarget(r *http.Request, user database.User) {
	auditTarget(r, "user", strconv.FormatUint(uint64(user.ID), 10), user.Username, nil)
}

func isLastAdmin(user database.User) bool {
	if user.Role != auth.RoleAdmin {
		return false
	}
	var admins int64
	database.DB.Model(&database.User{}).
		Where("role = ? AND active = ? AND id <> ?", auth.RoleAdmin, true, user.ID).
		Count(&admins)
	return admins == 0
}

func userFromQuery(w http.ResponseWriter, r *http.Request) (database.User, bool) {
	raw := strings.TrimSpace(r.URL.Query().Get("id"))
	id, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || id == 0 {
		writeError(w, http.StatusBadRequest, "id é obrigatório")
		return database.User{}, false
	}

	var user database.User
	if err := database.DB.First(&user, id).Error; err != nil {
		writeError(w, http.StatusNotFound, "usuário não encontrado")
		return database.User{}, false
	}
	return user, true
}

func bearerToken(r *http.Request) string {
	if v := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "); v != "" {
		return v
	}
	return r.Header.Get("X-API-Token")
}

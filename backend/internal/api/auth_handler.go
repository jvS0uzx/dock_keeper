package api

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/joaov/vd_stats/internal/auth"
	"github.com/joaov/vd_stats/internal/database"
	"gorm.io/gorm"
)

// loginHandler troca usuário e senha por um token de sessão.
// É a única rota, junto de /healthz, que não exige credencial prévia.
func (c Config) loginHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "corpo inválido")
		return
	}

	// O limite é conferido antes de auth.Login porque o custo que ele contém
	// está lá dentro: um bcrypt de 60 a 100 ms por tentativa, que sem teto
	// torna a rota uma negação de serviço não autenticada e barata.
	ip := clientIP(r, c.TrustProxyHeaders)
	if !c.logins.allowed(ip, req.Username) {
		w.Header().Set("Retry-After", strconv.Itoa(int(c.logins.window.Seconds())))
		writeError(w, http.StatusTooManyRequests, "tentativas demais; aguarde alguns minutos")
		return
	}

	session, err := auth.Login(req.Username, req.Password)
	switch {
	case errors.Is(err, auth.ErrInvalidCredentials), errors.Is(err, auth.ErrUserInactive):
		// Mesma resposta nos dois casos: dizer "usuário desativado" confirmaria
		// que o nome existe.
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

// logoutHandler encerra a sessão atual.
func logoutHandler(w http.ResponseWriter, r *http.Request) {
	auth.Logout(bearerToken(r))
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// meHandler devolve quem está autenticado e com qual papel, para o painel
// esconder o que a pessoa não pode fazer.
func (c Config) meHandler(w http.ResponseWriter, r *http.Request) {
	if session, ok := auth.Lookup(bearerToken(r)); ok {
		writeJSON(w, http.StatusOK, map[string]any{
			"username": session.Username,
			"role":     session.Role,
			"kind":     "user",
			"accesses": session.Accesses,
		})
		return
	}
	// Chegou aqui autenticado sem sessão: é o API_TOKEN de máquina.
	writeJSON(w, http.StatusOK, map[string]any{
		"username": "api-token",
		"role":     auth.RoleAdmin,
		"kind":     "token",
		"accesses": []auth.Access{{SiteID: nil, Role: auth.RoleAdmin}},
	})
}

// userView é o usuário com as concessões por unidade anexadas.
type userView struct {
	database.User
	Accesses []auth.Access `json:"accesses"`
}

// accessPayload é a lista de concessões enviada no create/update.
type accessPayload []struct {
	SiteID *uint  `json:"site_id"`
	Role   string `json:"role"`
}

// validateAccesses confere papéis e existência das unidades citadas.
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

// replaceAccesses troca o conjunto de concessões do usuário numa transação.
func replaceAccesses(userID uint, rows []database.UserSiteAccess) error {
	return database.DB.Transaction(func(tx *gorm.DB) error {
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
	})
}

// usersHandler faz o CRUD de usuários. Só admin chega aqui.
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
		Role     string        `json:"role"`
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

	user := database.User{Username: username, PasswordHash: hash, Role: req.Role, Active: true}
	if err := database.DB.Create(&user).Error; err != nil {
		log.Printf("[Auth] erro ao criar o usuário %q: %v", username, err)
		writeError(w, http.StatusConflict, "usuário já existe")
		return
	}
	if err := replaceAccesses(user.ID, accessRows); err != nil {
		log.Printf("[Auth] erro ao gravar acessos de %q: %v", username, err)
		writeError(w, http.StatusInternalServerError, "usuário criado, mas falhou ao gravar os acessos")
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

	// Troca de papel, desativação ou mudança de alcance precisa valer agora,
	// não no fim da sessão.
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

	if err := database.DB.Where("user_id = ?", user.ID).Delete(&database.UserSiteAccess{}).Error; err != nil {
		log.Printf("[Auth] erro ao remover acessos do usuário %d: %v", user.ID, err)
		writeError(w, http.StatusInternalServerError, "falha ao remover o usuário")
		return
	}
	if err := database.DB.Delete(&user).Error; err != nil {
		log.Printf("[Auth] erro ao remover o usuário %d: %v", user.ID, err)
		writeError(w, http.StatusInternalServerError, "falha ao remover o usuário")
		return
	}
	auth.RevokeUser(user.ID)
	// O usuário foi carregado por userFromQuery antes da exclusão, então o nome
	// ainda existe aqui — depois deste ponto sobraria só o id.
	auditUserTarget(r, user)
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// auditUserTarget nomeia o usuário afetado na linha de auditoria.
//
// A unidade fica nula de propósito: um usuário não pertence a uma unidade, ele
// tem concessões em várias. Escolher uma delas para o campo site_id faria a
// consulta por unidade mentir — mostraria a criação de um administrador global
// como se fosse evento de uma filial.
//
// Só o nome entra, nunca a senha nem o hash: esta linha vai para a mesma tabela
// que o administrador consulta.
func auditUserTarget(r *http.Request, user database.User) {
	auditTarget(r, "user", strconv.FormatUint(uint64(user.ID), 10), user.Username, nil)
}

// isLastAdmin evita que a instalação fique sem ninguém capaz de administrar.
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

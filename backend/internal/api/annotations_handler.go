package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/auth"
	"github.com/jvS0uzx/dock_keeper/internal/database"
)

const maxAnnotationRunes = 280

type annotationInput struct {
	ServerID *string `json:"server_id"`
	At       string  `json:"at"`
	Text     string  `json:"text"`
}

type annotationView struct {
	ID        uint      `json:"id"`
	ServerID  *string   `json:"server_id"`
	At        time.Time `json:"at"`
	Text      string    `json:"text"`
	Author    string    `json:"author"`
	CreatedAt time.Time `json:"created_at"`
}

type annotationRow struct {
	database.Annotation
	Author     string
	ServerSite *uint
	ServerSeen bool
}

func annotationsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		listAnnotations(w, r, sessionFrom(r))
		return
	}

	sess, ok := requireUserSession(w, r, "escrever anotação exige sessão de usuário")
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodPost:
		createAnnotation(w, r, sess)
	case http.MethodDelete:
		deleteAnnotation(w, r, sess)
	}
}

func annotationWindow(r *http.Request, now time.Time) (time.Time, time.Time, string) {
	q := r.URL.Query()
	start, end := now.Add(-24*time.Hour), now
	if v := strings.TrimSpace(q.Get("from")); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return start, end, "from inválido: use data RFC3339"
		}
		start = t
	}
	if v := strings.TrimSpace(q.Get("to")); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return start, end, "to inválido: use data RFC3339"
		}
		end = t
	}
	if !start.Before(end) {
		return start, end, "from precisa ser anterior a to"
	}
	return start, end, ""
}

func annotationQuery() string {
	return `SELECT a.*, COALESCE(u.username, '') AS author,
			s.site_id AS server_site, (s.id IS NOT NULL) AS server_seen
		FROM annotations a
		LEFT JOIN users u ON u.id = a.author_user_id
		LEFT JOIN servers s ON s.id = a.server_id AND s.deleted_at IS NULL`
}

func annotationVisible(sess auth.Session, row annotationRow) bool {
	if row.ServerID == nil {
		return true
	}
	if !row.ServerSeen {
		return auth.HasGlobal(sess.Accesses)
	}
	return auth.CanSeeSite(sess.Accesses, row.ServerSite)
}

func listAnnotations(w http.ResponseWriter, r *http.Request, sess auth.Session) {
	start, end, invalid := annotationWindow(r, time.Now().UTC())
	if invalid != "" {
		writeError(w, http.StatusBadRequest, invalid)
		return
	}

	sql := annotationQuery() + ` WHERE a.at >= ? AND a.at <= ?`
	args := []any{start, end}
	if serverID := strings.TrimSpace(r.URL.Query().Get("server_id")); serverID != "" {
		if _, found := lookupServer(w, sess, serverID); !found {
			return
		}
		sql += ` AND (a.server_id = ? OR a.server_id IS NULL)`
		args = append(args, serverID)
	}
	sql += ` ORDER BY a.at ASC, a.id ASC`

	var rows []annotationRow
	if err := database.DB.Raw(sql, args...).Scan(&rows).Error; err != nil {
		log.Printf("[Anotações] erro ao listar: %v", err)
		writeError(w, http.StatusInternalServerError, "falha ao listar as anotações")
		return
	}

	out := make([]annotationView, 0, len(rows))
	for _, row := range rows {
		if annotationVisible(sess, row) {
			out = append(out, viewOfAnnotation(row))
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func viewOfAnnotation(row annotationRow) annotationView {
	return annotationView{
		ID: row.ID, ServerID: row.ServerID, At: row.At, Text: row.Text,
		Author: row.Author, CreatedAt: row.CreatedAt,
	}
}

func createAnnotation(w http.ResponseWriter, r *http.Request, sess auth.Session) {
	var in annotationInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "corpo inválido")
		return
	}
	in.Text = strings.TrimSpace(in.Text)
	if in.Text == "" {
		writeError(w, http.StatusBadRequest, "text é obrigatório")
		return
	}
	if len([]rune(in.Text)) > maxAnnotationRunes {
		writeError(w, http.StatusBadRequest, "text passa de 280 caracteres")
		return
	}
	at := time.Now().UTC()
	if v := strings.TrimSpace(in.At); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "at inválido: use data RFC3339")
			return
		}
		at = t.UTC()
	}

	var siteID *uint
	if in.ServerID != nil && strings.TrimSpace(*in.ServerID) != "" {
		id := strings.TrimSpace(*in.ServerID)
		server, found := lookupServer(w, sess, id)
		if !found {
			return
		}
		in.ServerID, siteID = &id, server.SiteID
		if !auth.Allows(auth.RoleForSite(sess.Accesses, siteID), auth.RoleOperator) {
			writeError(w, http.StatusForbidden, "seu perfil não permite anotar neste servidor")
			return
		}
	} else {
		in.ServerID = nil
		if !auth.Allows(auth.GlobalRole(sess.Accesses), auth.RoleOperator) {
			writeError(w, http.StatusForbidden, "anotação global exige operador com acesso global")
			return
		}
	}

	note := database.Annotation{ServerID: in.ServerID, At: at, Text: in.Text, AuthorUserID: sess.UserID}
	if err := database.DB.Create(&note).Error; err != nil {
		log.Printf("[Anotações] erro ao gravar: %v", err)
		writeError(w, http.StatusInternalServerError, "falha ao gravar a anotação")
		return
	}
	auditTarget(r, "annotation", strconv.FormatUint(uint64(note.ID), 10), note.Text, siteID)
	writeJSON(w, http.StatusCreated, annotationView{
		ID: note.ID, ServerID: note.ServerID, At: note.At, Text: note.Text,
		Author: sess.Username, CreatedAt: note.CreatedAt,
	})
}

func deleteAnnotation(w http.ResponseWriter, r *http.Request, sess auth.Session) {
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "id é obrigatório")
		return
	}

	var rows []annotationRow
	if err := database.DB.Raw(annotationQuery()+` WHERE a.id = ?`, id).Scan(&rows).Error; err != nil || len(rows) == 0 {
		writeError(w, http.StatusNotFound, "anotação não encontrada")
		return
	}
	row := rows[0]
	if !annotationVisible(sess, row) {
		writeError(w, http.StatusNotFound, "anotação não encontrada")
		return
	}
	if row.AuthorUserID != sess.UserID && auth.GlobalRole(sess.Accesses) != auth.RoleAdmin {
		writeError(w, http.StatusForbidden, "só o autor ou um administrador global apaga esta anotação")
		return
	}

	if err := database.DB.Delete(&database.Annotation{}, row.ID).Error; err != nil {
		log.Printf("[Anotações] erro ao remover %d: %v", row.ID, err)
		writeError(w, http.StatusInternalServerError, "falha ao remover a anotação")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

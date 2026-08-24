package api

import (
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/joaov/vd_stats/internal/database"
)

const (
	defaultAuditLimit = 50
	maxAuditLimit     = 200
)

// auditListPage é a resposta paginada. O total vem junto porque a tela precisa
// saber se há próxima página, e contar no cliente exigiria trazer tudo — que é
// exatamente o que a paginação existe para evitar.
type auditListPage struct {
	Items  []database.AuditLog `json:"items"`
	Total  int64               `json:"total"`
	Limit  int                 `json:"limit"`
	Offset int                 `json:"offset"`
}

// auditListHandler lista o log de auditoria, mais recente primeiro.
//
// GET /api/audit?actor=&action=&result=&site_id=&from=&to=&limit=&offset=
//
// Registrada no wrapper admin, que exige papel de administrador GLOBAL: a
// tabela mostra ação de todas as unidades, e recortá-la por unidade daria a um
// administrador de filial a lista de quem o administra.
//
// A paginação não é conforto. A retenção é de um ano, e a primeira consulta sem
// LIMIT numa base madura carrega a tabela inteira para a memória do processo.
func auditListHandler(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	limit := defaultAuditLimit
	if raw := q.Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}
	limit = min(limit, maxAuditLimit)

	offset := 0
	if raw := q.Get("offset"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			offset = n
		}
	}

	tx := database.DB.Model(&database.AuditLog{})

	if v := strings.TrimSpace(q.Get("actor")); v != "" {
		tx = tx.Where("actor_username = ?", v)
	}
	// O prefixo casa a família inteira: "container" traz container.action sem
	// o operador precisar saber o nome exato do verbo.
	if v := strings.TrimSpace(q.Get("action")); v != "" {
		tx = tx.Where("action = ? OR action LIKE ?", v, v+".%")
	}
	if v := strings.TrimSpace(q.Get("result")); v != "" {
		tx = tx.Where("result = ?", v)
	}
	if v := strings.TrimSpace(q.Get("site_id")); v != "" {
		id, err := strconv.ParseUint(v, 10, 32)
		if err != nil {
			writeError(w, http.StatusBadRequest, "site_id inválido")
			return
		}
		tx = tx.Where("site_id = ?", uint(id))
	}

	from, ok := auditTime(w, q.Get("from"), "from")
	if !ok {
		return
	}
	if from != nil {
		tx = tx.Where("at >= ?", *from)
	}
	to, ok := auditTime(w, q.Get("to"), "to")
	if !ok {
		return
	}
	if to != nil {
		tx = tx.Where("at <= ?", *to)
	}

	var total int64
	if err := tx.Count(&total).Error; err != nil {
		log.Printf("[Auditoria] erro ao contar: %v", err)
		writeError(w, http.StatusInternalServerError, "falha ao consultar a auditoria")
		return
	}

	var items []database.AuditLog
	// O desempate por id existe porque várias linhas cabem no mesmo instante:
	// sem ele a paginação repete e pula registros entre uma página e outra.
	if err := tx.Order("at desc, id desc").Limit(limit).Offset(offset).Find(&items).Error; err != nil {
		log.Printf("[Auditoria] erro na consulta: %v", err)
		writeError(w, http.StatusInternalServerError, "falha ao consultar a auditoria")
		return
	}
	if items == nil {
		items = []database.AuditLog{}
	}

	writeJSON(w, http.StatusOK, auditListPage{
		Items: items, Total: total, Limit: limit, Offset: offset,
	})
}

// auditTime lê um instante em RFC3339. Devolve false quando o valor é inválido,
// e nesse caso a resposta de erro já foi escrita: filtro de tempo silenciosamente
// ignorado faria a tela mostrar o período errado sem avisar ninguém.
func auditTime(w http.ResponseWriter, raw, campo string) (*time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, true
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		writeError(w, http.StatusBadRequest, campo+" precisa estar no formato RFC3339")
		return nil, false
	}
	return &t, true
}

package api

import (
	"context"
	"net/http"

	"github.com/jvS0uzx/dock_keeper/internal/audit"
	"github.com/jvS0uzx/dock_keeper/internal/auth"
)

func (c Config) auditActor(r *http.Request) audit.Entry {
	return c.auditActorOf(sessionFrom(r), r)
}

func (c Config) auditActorOf(sess auth.Session, r *http.Request) audit.Entry {
	e := audit.Entry{
		ActorUsername: sess.Username,
		ActorRole:     sess.Role,
		SourceIP:      clientIP(r, c.TrustProxyHeaders),
		UserAgent:     r.UserAgent(),
	}
	if sess.UserID != 0 {
		id := sess.UserID
		e.ActorUserID = &id
	}
	return e
}

type auditTargetInfo struct {
	typ    string
	id     string
	label  string
	siteID *uint

	sess    auth.Session
	sessSet bool

	set     bool
	handled bool
}

type auditTargetCtxKey struct{}

func withAuditTarget(r *http.Request) (*http.Request, *auditTargetInfo) {
	info := &auditTargetInfo{}
	return r.WithContext(context.WithValue(r.Context(), auditTargetCtxKey{}, info)), info
}

func auditHandledByHandler(r *http.Request) {
	if info, ok := r.Context().Value(auditTargetCtxKey{}).(*auditTargetInfo); ok {
		info.handled = true
	}
}

func auditTarget(r *http.Request, targetType, id, label string, siteID *uint) {
	info, ok := r.Context().Value(auditTargetCtxKey{}).(*auditTargetInfo)
	if !ok {
		return
	}
	info.typ = targetType
	info.id = id
	info.label = label
	info.siteID = siteID
	info.set = true
}

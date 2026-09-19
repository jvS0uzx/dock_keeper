package api

import (
	"net/http"

	"github.com/jvS0uzx/dock_keeper/internal/audit"
	"github.com/jvS0uzx/dock_keeper/internal/database"
)

const (
	actionAuthLogOpen       = "auth-log.open"
	actionContainerLogsOpen = "container-logs.open"
)

func (c Config) auditStreamOpen(r *http.Request, action string, server database.Server, detail map[string]any) {
	entry := c.auditActor(r)
	entry.Action = action
	entry.TargetType = "server"
	entry.TargetID = server.ID
	entry.TargetLabel = server.Name
	entry.SiteID = server.SiteID
	entry.Result = audit.ResultOK
	entry.Detail = detail
	audit.Record(entry)
}

func (c Config) auditStreamDenied(r *http.Request, action, serverID string) {
	entry := c.auditActor(r)
	entry.Action = action
	entry.TargetType = "server"
	entry.TargetID = serverID
	entry.Result = audit.ResultDenied
	audit.Record(entry)
}

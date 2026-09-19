package api

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/jvS0uzx/dock_keeper/internal/audit"
	"github.com/jvS0uzx/dock_keeper/internal/ssh"
)

func startSSE(w http.ResponseWriter) (http.Flusher, bool) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming não suportado")
		return nil, false
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()
	return flusher, true
}

func (c Config) containerLogsStreamHandler(w http.ResponseWriter, r *http.Request) {
	containerName := r.URL.Query().Get("container_name")
	if containerName == "" {
		writeError(w, http.StatusBadRequest, "container_name é obrigatório")
		return
	}
	serverID := r.URL.Query().Get("server_id")
	server, ok := lookupServer(w, sessionFrom(r), serverID)
	if !ok {
		c.auditStreamDenied(r, actionContainerLogsOpen, serverID)
		return
	}
	target := c.sshTarget(server)
	release, ok := holdSSHSession(w, target)
	if !ok {
		return
	}
	defer release()

	flusher, ok := startSSE(w)
	if !ok {
		return
	}
	c.auditStreamOpen(r, actionContainerLogsOpen, server,
		map[string]any{"container": containerName, "host": server.HostIP})

	err := ssh.StreamDockerLogs(r.Context(), target, containerName, w, flusher)
	if err != nil && r.Context().Err() == nil {
		log.Printf("[API] erro no stream de logs de %s: %v", containerName, err)
	}
}

func (c Config) auditContainerDenial(r *http.Request, action, serverID string, detail map[string]any) {
	entry := c.auditActor(r)
	entry.Action = action
	entry.TargetType = "server"
	entry.TargetID = serverID
	entry.Result = audit.ResultDenied
	entry.Detail = detail
	audit.Record(entry)
}

func (c Config) containerActionHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ServerID      string `json:"server_id"`
		ContainerName string `json:"container_name"`
		Action        string `json:"action"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "corpo inválido")
		return
	}

	if !ssh.IsAllowedAction(req.Action) || !ssh.IsValidContainerName(req.ContainerName) {
		c.auditContainerDenial(r, "container.invalid", req.ServerID, map[string]any{
			"motivo": "ação ou nome de container fora do permitido",
		})
		writeError(w, http.StatusBadRequest, "ação ou nome de container inválido")
		return
	}

	server, ok := lookupServer(w, sessionFrom(r), req.ServerID)
	if !ok {
		c.auditContainerDenial(r, "container."+req.Action, req.ServerID, map[string]any{
			"container": req.ContainerName,
			"motivo":    "servidor inexistente ou fora do alcance da sessão",
		})
		return
	}

	entry := c.auditActor(r)
	entry.Action = "container." + req.Action
	entry.TargetType = "server"
	entry.TargetID = server.ID
	entry.TargetLabel = server.Name
	entry.SiteID = server.SiteID
	entry.Result = audit.ResultPending
	entry.Detail = map[string]any{"container": req.ContainerName, "host": server.HostIP}
	auditID := audit.Record(entry)

	target := c.sshTarget(server)
	release, err := ssh.AcquireSession(target)
	if err != nil {
		audit.Complete(auditID, audit.ResultError, map[string]any{"erro": err.Error()})
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	defer release()

	out, err := ssh.RunContainerAction(target, req.Action, req.ContainerName)
	if err != nil {
		audit.Complete(auditID, audit.ResultError, map[string]any{"erro": err.Error()})
		log.Printf("[API] ação %q em %s falhou: %v", req.Action, req.ContainerName, err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	audit.Complete(auditID, audit.ResultOK, map[string]any{"saida_bytes": len(out)})
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "output": out})
}

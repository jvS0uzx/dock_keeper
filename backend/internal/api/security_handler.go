package api

import (
	"log"
	"net/http"

	"github.com/jvS0uzx/dock_keeper/internal/ssh"
)

func (c Config) securityRadarHandler(w http.ResponseWriter, r *http.Request) {
	server, ok := lookupServer(w, sessionFrom(r), r.URL.Query().Get("server_id"))
	if !ok {
		return
	}

	target := c.sshTarget(server)
	release, ok := holdSSHSession(w, target)
	if !ok {
		return
	}
	defer release()

	ports, err := ssh.GetRadarPorts(target)
	if err != nil {
		log.Printf("[API] radar de portas em %s falhou: %v", server.HostIP, err)
		writeError(w, http.StatusBadGateway, "falha ao consultar as portas do host")
		return
	}
	if ports == nil {
		ports = []ssh.PortInfo{}
	}
	writeJSON(w, http.StatusOK, ports)
}

func (c Config) authLogStreamHandler(w http.ResponseWriter, r *http.Request) {
	serverID := r.URL.Query().Get("server_id")
	server, ok := lookupServer(w, sessionFrom(r), serverID)
	if !ok {
		c.auditStreamDenied(r, actionAuthLogOpen, serverID)
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
	c.auditStreamOpen(r, actionAuthLogOpen, server, map[string]any{"host": server.HostIP})

	err := ssh.StreamAuthLogs(r.Context(), target, w, flusher)
	if err != nil && r.Context().Err() == nil {
		log.Printf("[API] erro no stream de auth.log de %s: %v", server.HostIP, err)
	}
}

func holdSSHSession(w http.ResponseWriter, t ssh.Target) (func(), bool) {
	release, err := ssh.AcquireSession(t)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return nil, false
	}
	return release, true
}

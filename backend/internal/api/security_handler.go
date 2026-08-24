package api

import (
	"log"
	"net/http"

	"github.com/joaov/vd_stats/internal/ssh"
)

// securityRadarHandler lista as portas em LISTEN do host (ss -tulnp).
func (c Config) securityRadarHandler(w http.ResponseWriter, r *http.Request) {
	server, ok := lookupServer(w, sessionFrom(r), r.URL.Query().Get("server_id"))
	if !ok {
		return
	}

	ports, err := ssh.GetRadarPorts(c.sshTarget(server))
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

// authLogStreamHandler transmite /var/log/auth.log do host por SSE.
func (c Config) authLogStreamHandler(w http.ResponseWriter, r *http.Request) {
	serverID := r.URL.Query().Get("server_id")
	server, ok := lookupServer(w, sessionFrom(r), serverID)
	if !ok {
		c.auditStreamDenied(r, actionAuthLogOpen, serverID)
		return
	}

	flusher, ok := startSSE(w)
	if !ok {
		return
	}
	c.auditStreamOpen(r, actionAuthLogOpen, server, map[string]any{"host": server.HostIP})

	err := ssh.StreamAuthLogs(r.Context(), c.sshTarget(server), w, flusher)
	if err != nil && r.Context().Err() == nil {
		log.Printf("[API] erro no stream de auth.log de %s: %v", server.HostIP, err)
	}
}

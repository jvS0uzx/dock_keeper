package api

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/joaov/vd_stats/internal/audit"
	"github.com/joaov/vd_stats/internal/ssh"
)

// startSSE prepara a resposta para Server-Sent Events e devolve o flusher.
// Sem flusher explícito o Go bufferiza e o painel não recebe nada em tempo real.
func startSSE(w http.ResponseWriter) (http.Flusher, bool) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming não suportado")
		return nil, false
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // impede o Nginx de bufferizar o SSE
	w.WriteHeader(http.StatusOK)
	flusher.Flush()
	return flusher, true
}

// containerLogsStreamHandler transmite `docker logs -f` do container por SSE.
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

	flusher, ok := startSSE(w)
	if !ok {
		return
	}
	c.auditStreamOpen(r, actionContainerLogsOpen, server,
		map[string]any{"container": containerName, "host": server.HostIP})

	err := ssh.StreamDockerLogs(r.Context(), c.sshTarget(server), containerName, w, flusher)
	if err != nil && r.Context().Err() == nil {
		log.Printf("[API] erro no stream de logs de %s: %v", containerName, err)
	}
}

// auditContainerDenial registra a recusa de uma ação de container.
//
// Fica separado porque as duas recusas — argumento fora do permitido e servidor
// fora do alcance — precisam da mesma linha, e ela é o registro que mais
// interessa depois: é assim que uma tentativa de operar unidade alheia aparece.
func (c Config) auditContainerDenial(r *http.Request, action, serverID string, detail map[string]any) {
	entry := c.auditActor(r)
	entry.Action = action
	entry.TargetType = "server"
	entry.TargetID = serverID
	entry.Result = audit.ResultDenied
	entry.Detail = detail
	audit.Record(entry)
}

// containerActionHandler roda docker start/stop/restart no host remoto.
//
// A linha de auditoria nasce com ResultPending ANTES de o comando sair e é
// fechada depois. Gravar só no fim perderia justamente o caso que mais importa:
// o comando que travou a máquina e nunca retornou.
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

	// A ação e o nome do container entram na linha, que é gravada antes de
	// RunContainerAction ter chance de recusá-los. Conferir aqui, contra a mesma
	// allowlist e a mesma regex do pacote ssh, é o que impede o corpo da
	// requisição de escrever texto arbitrário na tabela de auditoria.
	if !ssh.IsAllowedAction(req.Action) || !ssh.IsValidContainerName(req.ContainerName) {
		c.auditContainerDenial(r, "container.invalid", req.ServerID, map[string]any{
			"motivo": "ação ou nome de container fora do permitido",
		})
		writeError(w, http.StatusBadRequest, "ação ou nome de container inválido")
		return
	}

	server, ok := lookupServer(w, sessionFrom(r), req.ServerID)
	if !ok {
		// lookupServer responde 404 tanto para servidor inexistente quanto para
		// servidor fora do alcance da sessão, de propósito (item C2): a resposta
		// não confirma existência. A linha herda a mesma ambiguidade, e basta —
		// o que interessa registrar é que alguém tentou.
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

	out, err := ssh.RunContainerAction(c.sshTarget(server), req.Action, req.ContainerName)
	if err != nil {
		audit.Complete(auditID, audit.ResultError, map[string]any{"erro": err.Error()})
		log.Printf("[API] ação %q em %s falhou: %v", req.Action, req.ContainerName, err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Só o tamanho da saída, nunca o conteúdo: o que um comando imprime no host
	// pode carregar segredo da aplicação monitorada.
	audit.Complete(auditID, audit.ResultOK, map[string]any{"saida_bytes": len(out)})
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "output": out})
}

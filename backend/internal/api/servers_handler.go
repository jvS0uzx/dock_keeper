package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/joaov/vd_stats/internal/auth"
	"github.com/joaov/vd_stats/internal/database"
	"github.com/joaov/vd_stats/internal/ssh"
)

type ServerCreateRequest struct {
	HostIP       string `json:"host_ip"`
	Name         string `json:"name"`
	User         string `json:"user"`
	Port         int    `json:"port"`
	CollectNginx bool   `json:"collect_nginx"`
	SiteID       *uint  `json:"site_id"`
}

// serversHandler faz o CRUD de Server. GET lista, POST cadastra e liga o
// stream SSH na hora, DELETE para o stream e remove.
func (c Config) serversHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		var servers []database.Server
		if err := database.DB.Order("name ASC").Find(&servers).Error; err != nil {
			log.Printf("[API] erro ao listar servidores: %v", err)
			writeError(w, http.StatusInternalServerError, "falha ao listar servidores")
			return
		}
		if servers == nil {
			servers = []database.Server{}
		}
		writeJSON(w, http.StatusOK, servers)

	case http.MethodPost:
		var req ServerCreateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "corpo inválido")
			return
		}
		req.HostIP = strings.TrimSpace(req.HostIP)
		req.Name = strings.TrimSpace(req.Name)
		if req.HostIP == "" || req.Name == "" {
			writeError(w, http.StatusBadRequest, "host_ip e name são obrigatórios")
			return
		}
		if req.User == "" {
			req.User = "root"
		}
		if req.Port <= 0 || req.Port > 65535 {
			req.Port = ssh.DefaultPort
		}

		var server database.Server
		if err := database.DB.Where("host_ip = ?", req.HostIP).
			Assign(database.Server{
				Name: req.Name, User: req.User, Port: req.Port,
				CollectNginx: req.CollectNginx, SiteID: req.SiteID,
			}).
			FirstOrCreate(&server, database.Server{HostIP: req.HostIP}).Error; err != nil {
			log.Printf("[API] erro ao cadastrar servidor %s: %v", req.HostIP, err)
			writeError(w, http.StatusInternalServerError, "falha ao cadastrar servidor")
			return
		}

		ssh.Manager.Start(c.sshTarget(server))
		auditTarget(r, "server", server.ID, server.Name, server.SiteID)
		writeJSON(w, http.StatusCreated, server)

	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if id == "" {
			writeError(w, http.StatusBadRequest, "id é obrigatório")
			return
		}
		// O rótulo é lido antes da exclusão: depois dela sobra o uuid, e
		// "removeu o servidor 3f2a..." não diz nada seis meses adiante.
		var doomed database.Server
		found := database.DB.Where("id = ?", id).First(&doomed).Error == nil

		ssh.Manager.Stop(id)
		if err := database.DB.Where("id = ?", id).Delete(&database.Server{}).Error; err != nil {
			log.Printf("[API] erro ao remover servidor %s: %v", id, err)
			writeError(w, http.StatusInternalServerError, "falha ao remover servidor")
			return
		}
		if found {
			auditTarget(r, "server", doomed.ID, doomed.Name, doomed.SiteID)
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	}
}

// lookupServer resolve um server_id e já responde o erro HTTP quando falta,
// não existe ou está fora do alcance da sessão. Todos os endpoints que abrem
// SSH passam por aqui — é onde o recorte por unidade encosta no acesso remoto.
//
// Servidor fora do alcance responde 404 e não 403: 403 confirmaria que aquele
// servidor existe, e a lista do parque é justamente o que o recorte esconde.
// Servidor sem unidade (VPS de infraestrutura) só é alcançado por concessão
// global, como o escopo "none" das demais consultas.
func lookupServer(w http.ResponseWriter, sess auth.Session, id string) (database.Server, bool) {
	if id == "" {
		writeError(w, http.StatusBadRequest, "server_id é obrigatório")
		return database.Server{}, false
	}
	var server database.Server
	if err := database.DB.Where("id = ?", id).First(&server).Error; err != nil {
		writeError(w, http.StatusNotFound, "servidor não encontrado")
		return database.Server{}, false
	}
	if !auth.CanSeeSite(sess.Accesses, server.SiteID) {
		writeError(w, http.StatusNotFound, "servidor não encontrado")
		return database.Server{}, false
	}
	return server, true
}

// sshTarget monta o alvo SSH a partir do registro do banco mais a chave da
// configuração — a chave é do processo, não do servidor cadastrado.
func (c Config) sshTarget(s database.Server) ssh.Target {
	return ssh.Target{
		ID:           s.ID,
		Name:         s.Name,
		Host:         s.HostIP,
		User:         s.User,
		Port:         s.Port,
		KeyPath:      c.SSHKeyPath,
		CollectNginx: s.CollectNginx,
	}
}

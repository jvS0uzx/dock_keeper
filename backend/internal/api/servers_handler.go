package api

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/jvS0uzx/dock_keeper/internal/auth"
	"github.com/jvS0uzx/dock_keeper/internal/database"
	"github.com/jvS0uzx/dock_keeper/internal/ssh"
)

type ServerPatchRequest struct {
	Name     *string         `json:"name"`
	User     *string         `json:"user"`
	Port     *int            `json:"port"`
	Aliases  *[]string       `json:"aliases"`
	BehindLB json.RawMessage `json:"behind_lb"`
}

type ServerCreateRequest struct {
	HostIP       string `json:"host_ip"`
	Name         string `json:"name"`
	User         string `json:"user"`
	Port         int    `json:"port"`
	CollectNginx bool   `json:"collect_nginx"`
	SiteID       *uint  `json:"site_id"`
}

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

		if dono, existe := database.DonoDoEndereco(req.HostIP, req.SiteID, ""); existe {
			writeError(w, http.StatusConflict,
				"o endereço "+req.HostIP+" já pertence ao servidor "+dono.Descricao()+
					"; renomeie o servidor existente em vez de cadastrar outro")
			return
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

	case http.MethodPatch:
		c.patchServer(w, r)

	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if id == "" {
			writeError(w, http.StatusBadRequest, "id é obrigatório")
			return
		}
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

func (c Config) patchServer(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "id é obrigatório")
		return
	}

	var req ServerPatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "corpo inválido")
		return
	}
	if req.Name == nil && req.User == nil && req.Port == nil && req.Aliases == nil && req.BehindLB == nil {
		writeError(w, http.StatusBadRequest, "nenhum campo para atualizar")
		return
	}

	behindLB, limparBehindLB, err := lerBehindLB(req.BehindLB)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	var aliases []string
	if req.Aliases != nil {
		for _, bruto := range *req.Aliases {
			endereco, ok := database.EnderecoValido(bruto)
			if !ok {
				writeError(w, http.StatusBadRequest, "endereço inválido: "+bruto)
				return
			}
			aliases = append(aliases, endereco)
		}
	}

	var server database.Server
	if err := database.DB.Where("id = ?", id).First(&server).Error; err != nil {
		writeError(w, http.StatusNotFound, "servidor não encontrado")
		return
	}

	for _, alias := range aliases {
		if dono, existe := database.DonoDoEndereco(alias, server.SiteID, server.ID); existe {
			writeError(w, http.StatusConflict,
				"o endereço "+alias+" já pertence ao servidor "+dono.Descricao())
			return
		}
	}

	updates := map[string]any{}
	if req.Name != nil {
		nome := strings.TrimSpace(*req.Name)
		if nome == "" {
			writeError(w, http.StatusBadRequest, "name é obrigatório")
			return
		}
		if len([]rune(nome)) > 64 {
			writeError(w, http.StatusBadRequest, "name passa de 64 caracteres")
			return
		}
		if nomeEmUsoNaUnidade(nome, server.SiteID, server.ID) {
			writeError(w, http.StatusConflict, "já existe um servidor com esse nome nesta unidade")
			return
		}
		updates["name"] = nome
	}
	if req.User != nil {
		usuario := strings.TrimSpace(*req.User)
		if usuario == "" {
			writeError(w, http.StatusBadRequest, "user não pode ficar vazio")
			return
		}
		updates["user"] = usuario
	}
	if req.Port != nil {
		if *req.Port <= 0 || *req.Port > 65535 {
			writeError(w, http.StatusBadRequest, "port fora da faixa 1-65535")
			return
		}
		updates["port"] = *req.Port
	}

	if limparBehindLB {
		updates["behind_lb"] = nil
	} else if behindLB != nil {
		updates["behind_lb"] = *behindLB
	}

	if len(updates) > 0 {
		if err := database.DB.Model(&server).Updates(updates).Error; err != nil {
			log.Printf("[API] erro ao atualizar servidor %s: %v", id, err)
			writeError(w, http.StatusInternalServerError, "falha ao atualizar servidor")
			return
		}
	}
	if len(aliases) > 0 {
		database.RegistrarAliases(server.ID, aliases)
	}

	if err := database.DB.Where("id = ?", id).First(&server).Error; err != nil {
		log.Printf("[API] erro ao reler servidor %s: %v", id, err)
		writeError(w, http.StatusInternalServerError, "falha ao atualizar servidor")
		return
	}

	auditTarget(r, "server", server.ID, server.Name, server.SiteID)
	writeJSON(w, http.StatusOK, server)
}

func lerBehindLB(bruto json.RawMessage) (*bool, bool, error) {
	if bruto == nil {
		return nil, false, nil
	}
	if string(bruto) == "null" {
		return nil, true, nil
	}
	var valor bool
	if err := json.Unmarshal(bruto, &valor); err != nil {
		return nil, false, errors.New("behind_lb aceita true, false ou null")
	}
	return &valor, false, nil
}

func nomeEmUsoNaUnidade(nome string, siteID *uint, exceto string) bool {
	q := database.DB.Model(&database.Server{}).Where("name = ? AND id <> ?", nome, exceto)
	if siteID == nil {
		q = q.Where("site_id IS NULL")
	} else {
		q = q.Where("site_id = ?", *siteID)
	}
	var contagem int64
	q.Count(&contagem)
	return contagem > 0
}

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

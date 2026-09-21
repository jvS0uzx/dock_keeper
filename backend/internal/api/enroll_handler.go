package api

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/jvS0uzx/dock_keeper/internal/audit"
	"github.com/jvS0uzx/dock_keeper/internal/config"
	"github.com/jvS0uzx/dock_keeper/internal/database"
)

const (
	enrollTokenTTL = 24 * time.Hour

	kindAgent     = "agent"
	kindCollector = "collector"
)

type enrollTokenRequest struct {
	SiteID uint   `json:"site_id"`
	Kind   string `json:"kind"`
}

type enrollRequest struct {
	Token     string `json:"enrollment_token"`
	MachineID string `json:"machine_id"`
	Hostname  string `json:"hostname"`
	Kind      string `json:"kind"`
}

func (c Config) enrollTokensHandler(w http.ResponseWriter, r *http.Request) {
	var req enrollTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "corpo inválido")
		return
	}

	req.Kind = strings.ToLower(strings.TrimSpace(req.Kind))
	if req.Kind != kindAgent && req.Kind != kindCollector {
		writeError(w, http.StatusBadRequest, "kind precisa ser agent ou collector")
		return
	}

	var site database.Site
	if err := database.DB.First(&site, req.SiteID).Error; err != nil {
		writeError(w, http.StatusBadRequest, "unidade inexistente")
		return
	}

	valor, err := newSecret()
	if err != nil {
		log.Printf("[Enroll] erro ao gerar o convite: %v", err)
		writeError(w, http.StatusInternalServerError, "falha ao gerar o convite")
		return
	}

	token := database.EnrollmentToken{
		TokenHash: hashSecret(valor),
		SiteID:    site.ID,
		Kind:      req.Kind,
		ExpiresAt: time.Now().UTC().Add(enrollTokenTTL),
		CreatedBy: sessionFrom(r).UserID,
	}
	if err := database.DB.Create(&token).Error; err != nil {
		log.Printf("[Enroll] erro ao gravar o convite: %v", err)
		writeError(w, http.StatusInternalServerError, "falha ao gravar o convite")
		return
	}

	auditTarget(r, "site", strconv.FormatUint(uint64(site.ID), 10), site.Name, &site.ID)

	writeJSON(w, http.StatusCreated, map[string]any{
		"enrollment_token": valor,
		"site_id":          site.ID,
		"kind":             req.Kind,
		"expires_at":       token.ExpiresAt,
	})
}

func (c Config) enrollHandler(w http.ResponseWriter, r *http.Request) {
	if !limitarTaxa(w, r, "enroll-ip:"+clientIP(r, config.Booleano("TRUST_PROXY_HEADERS", false)), tetoDeEnroll()) {
		return
	}

	var req enrollRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "corpo inválido")
		return
	}

	req.MachineID = strings.TrimSpace(req.MachineID)
	if req.MachineID == "" {
		writeError(w, http.StatusBadRequest, "machine_id é obrigatório")
		return
	}

	entrada := c.auditActor(r)
	entrada.Action = "device.enroll"
	entrada.TargetType = "device"
	entrada.Detail = map[string]any{
		"machine_id": req.MachineID,
		"hostname":   strings.TrimSpace(req.Hostname),
	}

	cred, err := trocarConvitePorCredencial(req)
	if err != nil {
		entrada.Result = audit.ResultDenied
		entrada.Detail["motivo"] = err.Error()
		audit.Record(entrada)

		if errors.Is(err, errEnrollKindMismatch) {
			writeError(w, http.StatusConflict, "o tipo do dispositivo não confere com o do convite; o convite continua válido")
			return
		}
		writeError(w, http.StatusUnauthorized, "convite inválido, expirado ou já utilizado")
		return
	}

	entrada.Result = audit.ResultOK
	entrada.TargetID = cred.credencial.DeviceID
	entrada.SiteID = &cred.credencial.SiteID
	audit.Record(entrada)

	writeJSON(w, http.StatusCreated, map[string]any{
		"device_id":    cred.credencial.DeviceID,
		"device_token": cred.segredo,
		"site_id":      cred.credencial.SiteID,
		"kind":         cred.credencial.Kind,
	})
}

var errEnrollKindMismatch = errors.New("kind declarado não confere com o do convite")

var errConviteJaUsado = errors.New("convite consumido por outro dispositivo durante a troca")

type credencialEmitida struct {
	credencial database.DeviceCredential
	segredo    string
}

func trocarConvitePorCredencial(req enrollRequest) (credencialEmitida, error) {
	var saida credencialEmitida

	err := database.DB.Transaction(func(tx *gorm.DB) error {
		var token database.EnrollmentToken
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("token_hash = ? AND used_at IS NULL AND expires_at > ?",
				hashSecret(strings.TrimSpace(req.Token)), time.Now().UTC()).
			First(&token).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("convite inexistente, expirado ou já usado")
		}
		if err != nil {
			return err
		}
		if kind := strings.ToLower(strings.TrimSpace(req.Kind)); kind != "" && kind != token.Kind {
			return errEnrollKindMismatch
		}

		deviceID, err := newDeviceID()
		if err != nil {
			return err
		}
		segredo, err := newSecret()
		if err != nil {
			return err
		}

		cred := database.DeviceCredential{
			DeviceID:   deviceID,
			SiteID:     token.SiteID,
			Kind:       token.Kind,
			SecretHash: hashSecret(segredo),
			MachineID:  req.MachineID,
			Hostname:   strings.TrimSpace(req.Hostname),
		}
		if err := tx.Create(&cred).Error; err != nil {
			return err
		}

		agora := time.Now().UTC()
		baixa := tx.Model(&database.EnrollmentToken{}).
			Where("id = ? AND used_at IS NULL", token.ID).
			Update("used_at", agora)
		if baixa.Error != nil {
			return baixa.Error
		}
		if baixa.RowsAffected != 1 {
			return errConviteJaUsado
		}

		saida = credencialEmitida{credencial: cred, segredo: segredo}
		return nil
	})

	return saida, err
}

func (c Config) devicesHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		var creds []database.DeviceCredential
		if err := database.DB.Order("created_at desc").Find(&creds).Error; err != nil {
			writeError(w, http.StatusInternalServerError, "falha ao listar dispositivos")
			return
		}
		writeJSON(w, http.StatusOK, creds)

	case http.MethodDelete:
		id := strings.TrimSpace(r.URL.Query().Get("device_id"))
		if id == "" {
			writeError(w, http.StatusBadRequest, "device_id é obrigatório")
			return
		}

		var cred database.DeviceCredential
		if err := database.DB.Where("device_id = ?", id).First(&cred).Error; err != nil {
			writeError(w, http.StatusNotFound, "dispositivo não encontrado")
			return
		}

		agora := time.Now().UTC()
		if err := database.DB.Model(&database.DeviceCredential{}).
			Where("device_id = ?", id).
			Update("revoked_at", agora).Error; err != nil {
			writeError(w, http.StatusInternalServerError, "falha ao revogar")
			return
		}

		auditTarget(r, "device", cred.DeviceID, cred.Hostname, &cred.SiteID)
		writeJSON(w, http.StatusOK, map[string]string{"status": "revogado"})

	default:
		w.Header().Set("Allow", "GET, DELETE")
		writeError(w, http.StatusMethodNotAllowed, "método não permitido")
	}
}

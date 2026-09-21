package api

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"github.com/jvS0uzx/dock_keeper/internal/config"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/jvS0uzx/dock_keeper/internal/audit"
	"github.com/jvS0uzx/dock_keeper/internal/database"
)

const (
	headerDeviceID    = "X-Device-Id"
	headerDeviceToken = "X-Device-Token"

	headerLegacyToken = "X-Agent-Token"
)

type deviceAuth struct {
	SiteID *uint

	DeviceID string
	Kind     string

	Legacy bool
}

var (
	errDeviceUnauthorized  = errors.New("credencial de dispositivo inválida")
	errLegacyTokenDisabled = errors.New("token compartilhado desligado")
)

func authenticateDevice(r *http.Request) (deviceAuth, error) {
	deviceID := strings.TrimSpace(r.Header.Get(headerDeviceID))
	secret := strings.TrimSpace(r.Header.Get(headerDeviceToken))

	if deviceID != "" && secret != "" {
		return authenticateCredential(deviceID, secret)
	}

	presented := r.Header.Get(headerLegacyToken)
	if presented != "" && !legacyIngestAllowed() {
		return deviceAuth{}, errLegacyTokenDisabled
	}
	expected := os.Getenv("AGENT_INGEST_TOKEN")
	if expected == "" {
		return deviceAuth{}, errDeviceUnauthorized
	}
	if subtle.ConstantTimeCompare([]byte(presented), []byte(expected)) != 1 {
		return deviceAuth{}, errDeviceUnauthorized
	}

	log.Printf("[Ingest] AVISO: envio autenticado pelo AGENT_INGEST_TOKEN compartilhado, " +
		"que não amarra o dispositivo a uma unidade. Migre para credencial própria (POST /api/enroll).")
	return deviceAuth{Legacy: true}, nil
}

func authenticateCredential(deviceID, secret string) (deviceAuth, error) {
	var cred database.DeviceCredential
	err := database.DB.Where("device_id = ?", deviceID).First(&cred).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		_ = subtle.ConstantTimeCompare([]byte(hashSecret(secret)), []byte(strings.Repeat("0", 64)))
		return deviceAuth{}, errDeviceUnauthorized
	}
	if err != nil {
		return deviceAuth{}, err
	}

	if subtle.ConstantTimeCompare([]byte(hashSecret(secret)), []byte(cred.SecretHash)) != 1 {
		return deviceAuth{}, errDeviceUnauthorized
	}
	if cred.RevokedAt != nil {
		return deviceAuth{}, errDeviceUnauthorized
	}

	agora := time.Now().UTC()
	database.DB.Model(&database.DeviceCredential{}).
		Where("device_id = ?", cred.DeviceID).
		Update("last_seen_at", agora)

	site := cred.SiteID
	return deviceAuth{SiteID: &site, DeviceID: cred.DeviceID, Kind: cred.Kind}, nil
}

func (d deviceAuth) allowsKind(kind string) bool {
	return d.Legacy || d.Kind == kind
}

func refuseDeviceKind(w http.ResponseWriter, r *http.Request, cred deviceAuth, action, expected, rota string) {
	auditHandledByHandler(r)
	audit.Record(audit.Entry{
		Action:     action,
		TargetType: "device",
		TargetID:   cred.DeviceID,
		SiteID:     cred.SiteID,
		Result:     audit.ResultDenied,
		Detail: map[string]any{
			"kind_da_credencial": cred.Kind,
			"kind_exigido":       expected,
			"rota":               r.URL.Path,
		},
	})
	writeError(w, http.StatusForbidden, "credencial do tipo "+cred.Kind+" não envia "+rota)
}

func (d deviceAuth) siteMatches(declared *uint) bool {
	if d.Legacy || d.SiteID == nil {
		return true
	}
	if declared == nil {
		return true
	}
	return *declared == *d.SiteID
}

func hashSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

func newSecret() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func newDeviceID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func auditIngestSiteMismatch(cred deviceAuth, p ingestPayload, declarada *uint) {
	detalhe := map[string]any{
		"hostname":           p.Hostname,
		"machine_id":         p.MachineID,
		"site_code_no_envio": p.SiteCode,
	}
	if declarada != nil {
		detalhe["site_id_declarado"] = *declarada
	}

	audit.Record(audit.Entry{
		Action:     "ingest.site_mismatch",
		TargetType: "device",
		TargetID:   cred.DeviceID,
		SiteID:     cred.SiteID,
		Result:     audit.ResultDenied,
		Detail:     detalhe,
	})
}

func legacyIngestAllowed() bool {
	on, _ := strconv.ParseBool(strings.TrimSpace(os.Getenv("ALLOW_LEGACY_INGEST_TOKEN")))
	return on
}

func refuseDeviceAuth(w http.ResponseWriter, r *http.Request, err error, action string) {
	if !errors.Is(err, errLegacyTokenDisabled) {
		writeError(w, http.StatusUnauthorized, "credencial de dispositivo inválida")
		return
	}
	auditHandledByHandler(r)
	audit.Record(audit.Entry{
		SourceIP:   clientIP(r, config.Booleano("TRUST_PROXY_HEADERS", false)),
		UserAgent:  r.UserAgent(),
		Action:     action,
		TargetType: "device",
		Result:     audit.ResultDenied,
		Detail: map[string]any{
			"rota":   r.URL.Path,
			"motivo": "token compartilhado desligado (ALLOW_LEGACY_INGEST_TOKEN=false)",
		},
	})
	writeError(w, http.StatusUnauthorized, "o token compartilhado (X-Agent-Token) está desligado neste painel: "+
		"troque o convite por uma credencial própria em POST /api/enroll")
}

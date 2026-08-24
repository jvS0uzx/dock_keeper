package api

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/joaov/vd_stats/internal/audit"
	"github.com/joaov/vd_stats/internal/database"
)

// Cabeçalhos da credencial por dispositivo.
const (
	headerDeviceID    = "X-Device-Id"
	headerDeviceToken = "X-Device-Token"

	// headerLegacyToken é o AGENT_INGEST_TOKEN único, aceito durante a
	// transição. Continua funcionando porque derrubá-lo no dia do deploy
	// silenciaria todo agente e coletor já instalado — e um painel de
	// monitoramento que emudece é pior que um painel inseguro, porque ninguém
	// percebe.
	headerLegacyToken = "X-Agent-Token"
)

// deviceAuth é o resultado da autenticação de um envio máquina-a-máquina.
type deviceAuth struct {
	// SiteID é a unidade AUTORITATIVA do envio. Vem da credencial, nunca do
	// corpo. É a inversão que resolve o item S7: com o token único, qualquer
	// portador declarava a unidade que quisesse e forjava inventário de outra
	// filial.
	SiteID *uint

	DeviceID string
	Kind     string

	// Legacy marca envio autenticado pelo token compartilhado. Nesse modo a
	// unidade continua vindo do corpo, porque o token único não sabe dizer de
	// onde o envio veio — é exatamente a fraqueza que o enrollment corrige.
	Legacy bool
}

var errDeviceUnauthorized = errors.New("credencial de dispositivo inválida")

// authenticateDevice resolve a credencial de um envio de agente ou coletor.
//
// Ordem deliberada: a credencial própria vence o token compartilhado. Um
// dispositivo que já fez enrollment e ainda carrega o token antigo na
// configuração é autenticado pela credencial, com a unidade conferida — o
// contrário deixaria a migração sem efeito enquanto alguém não limpasse os
// arquivos de configuração um a um.
func authenticateDevice(r *http.Request) (deviceAuth, error) {
	deviceID := strings.TrimSpace(r.Header.Get(headerDeviceID))
	secret := strings.TrimSpace(r.Header.Get(headerDeviceToken))

	if deviceID != "" && secret != "" {
		return authenticateCredential(deviceID, secret)
	}

	expected := os.Getenv("AGENT_INGEST_TOKEN")
	if expected == "" {
		// Sem token compartilhado configurado E sem credencial apresentada, a
		// ingestão fica desligada — o fail-closed que já existia.
		return deviceAuth{}, errDeviceUnauthorized
	}
	presented := r.Header.Get(headerLegacyToken)
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
		// Custo simétrico: sem isto, dispositivo inexistente responde antes de
		// dispositivo com segredo errado, e o relógio diz quais device_id
		// existem.
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

	// last_seen_at é escrita solta, sem transação e sem conferir erro: é dado de
	// conveniência para a tela de dispositivos, e falhar aqui não pode recusar
	// uma métrica legítima.
	agora := time.Now().UTC()
	database.DB.Model(&database.DeviceCredential{}).
		Where("device_id = ?", cred.DeviceID).
		Update("last_seen_at", agora)

	site := cred.SiteID
	return deviceAuth{SiteID: &site, DeviceID: cred.DeviceID, Kind: cred.Kind}, nil
}

// siteMatches confere a unidade declarada no corpo contra a da credencial.
//
// Divergência é o sinal mais direto de dispositivo comprometido que o sistema
// consegue produzir, e por isso o envio é descartado inteiro: aceitar
// parcialmente é aceitar o que o atacante escolheu.
//
// No modo legado não há o que conferir — o token compartilhado não carrega
// unidade — e a função devolve a unidade do corpo, que é o comportamento antigo.
func (d deviceAuth) siteMatches(declared *uint) bool {
	if d.Legacy || d.SiteID == nil {
		return true
	}
	if declared == nil {
		// Corpo sem unidade sob credencial válida: a credencial decide, e isso
		// é o caminho normal depois da migração — o dispositivo deixa de
		// precisar declarar de onde é.
		return true
	}
	return *declared == *d.SiteID
}

// hashSecret é o SHA-256 hexadecimal usado nas duas tabelas de identidade.
//
// SHA-256 e não bcrypt de propósito: o segredo tem 32 bytes de crypto/rand, e
// contra entropia dessa ordem o ataque de dicionário que o bcrypt encarece não
// existe. O que existe é o custo por requisição, e a ingestão roda a cada poucos
// segundos por dispositivo — um bcrypt aqui seria negação de serviço embutida.
func hashSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

// newSecret gera um segredo de 32 bytes. Erro de crypto/rand é fatal e não deve
// ser tratado como recuperável: continuar produziria credencial previsível.
func newSecret() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// newDeviceID gera o identificador público da credencial, que viaja em claro.
func newDeviceID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// auditIngestSiteMismatch registra a tentativa de um dispositivo declarar
// unidade diferente da sua.
//
// É o sinal mais direto de dispositivo comprometido que o sistema consegue
// produzir: a credencial é legítima, mas o envio afirma pertencer a outra
// filial. Merece linha própria, e não o registro genérico da rota, porque o
// administrador vai procurar exatamente por isto.
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

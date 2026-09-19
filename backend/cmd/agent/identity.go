package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type credential struct {
	DeviceID string `json:"device_id"`
	Token    string `json:"device_token"`
	SiteID   uint   `json:"site_id"`
	Kind     string `json:"kind"`
}

func credentialPath() string {
	if v := strings.TrimSpace(os.Getenv("AGENT_CREDENTIAL_PATH")); v != "" {
		return v
	}
	if runtime.GOOS == "windows" {
		return filepath.Join(os.Getenv("ProgramData"), "dockkeeper-agent", "credential.json")
	}
	return "/var/lib/dockkeeper-agent/credential.json"
}

func machineID() string {
	if v := strings.TrimSpace(os.Getenv("AGENT_MACHINE_ID")); v != "" {
		return v
	}

	for _, caminho := range []string{"/etc/machine-id", "/var/lib/dbus/machine-id"} {
		if b, err := os.ReadFile(caminho); err == nil {
			if id := strings.TrimSpace(string(b)); id != "" {
				return id
			}
		}
	}

	return persistedFallbackID()
}

func persistedFallbackID() string {
	caminho := filepath.Join(filepath.Dir(credentialPath()), "machine-id")

	if b, err := os.ReadFile(caminho); err == nil {
		if id := strings.TrimSpace(string(b)); id != "" {
			return id
		}
	}

	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		log.Printf("[Agent] AVISO: nao foi possivel gerar identificador de maquina: %v", err)
		return ""
	}
	id := hex.EncodeToString(buf)

	if err := os.MkdirAll(filepath.Dir(caminho), 0o755); err == nil {
		if err := os.WriteFile(caminho, []byte(id+"\n"), 0o600); err != nil {
			log.Printf("[Agent] AVISO: identificador de maquina nao foi persistido: %v", err)
		}
	}
	return id
}

func loadCredential() (credential, bool) {
	b, err := os.ReadFile(credentialPath())
	if err != nil {
		return credential{}, false
	}

	var c credential
	if err := json.Unmarshal(b, &c); err != nil {
		log.Printf("[Agent] AVISO: credencial ilegivel em %s: %v", credentialPath(), err)
		return credential{}, false
	}
	if c.DeviceID == "" || c.Token == "" {
		return credential{}, false
	}
	return c, true
}

func saveCredential(c credential) error {
	caminho := credentialPath()
	if err := os.MkdirAll(filepath.Dir(caminho), 0o755); err != nil {
		return err
	}

	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(caminho, append(b, '\n'), 0o600)
}

func enroll(client *http.Client, serverURL, conviteToken, hostname string) (credential, error) {
	corpo, err := json.Marshal(map[string]string{
		"enrollment_token": conviteToken,
		"machine_id":       machineID(),
		"hostname":         hostname,
		"kind":             "agent",
	})
	if err != nil {
		return credential{}, err
	}

	req, err := http.NewRequest(http.MethodPost, serverURL+"/api/enroll", bytes.NewReader(corpo))
	if err != nil {
		return credential{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return credential{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return credential{}, fmt.Errorf("enrollment recusado (%d): %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}

	var c credential
	if err := json.NewDecoder(resp.Body).Decode(&c); err != nil {
		return credential{}, err
	}
	if c.DeviceID == "" || c.Token == "" {
		return credential{}, errors.New("painel devolveu credencial incompleta")
	}
	return c, nil
}

func resolverIdentidade(client *http.Client, serverURL, hostname string) (credential, string) {
	if c, ok := loadCredential(); ok {
		log.Printf("[Agent] credencial propria em uso (device=%s unidade=%d)", c.DeviceID, c.SiteID)
		return c, ""
	}

	if convite := strings.TrimSpace(os.Getenv("AGENT_ENROLL_TOKEN")); convite != "" {
		c, err := enroll(client, serverURL, convite, hostname)
		if err != nil {
			log.Fatalf("[Agent] enrollment falhou: %v", err)
		}
		if err := saveCredential(c); err != nil {
			log.Printf("[Agent] AVISO GRAVE: credencial obtida mas NAO gravada em %s (%v). "+
				"O convite ja foi consumido; emita outro antes de reiniciar este agente.",
				credentialPath(), err)
		} else {
			log.Printf("[Agent] enrollment concluido (device=%s unidade=%d). "+
				"Remova AGENT_ENROLL_TOKEN da configuracao.", c.DeviceID, c.SiteID)
		}
		return c, ""
	}

	legado := strings.TrimSpace(os.Getenv("AGENT_TOKEN"))
	if legado == "" {
		log.Fatal("[Agent] sem identidade: defina AGENT_ENROLL_TOKEN para se cadastrar, " +
			"ou AGENT_TOKEN para o modo compartilhado (em descontinuacao)")
	}
	log.Printf("[Agent] AVISO: usando AGENT_TOKEN compartilhado, descontinuado. O painel so aceita " +
		"esse token com ALLOW_LEGACY_INGEST_TOKEN=true no .env dele; sem isso todo envio volta 401. " +
		"Migre para credencial propria com AGENT_ENROLL_TOKEN.")
	return credential{}, legado
}

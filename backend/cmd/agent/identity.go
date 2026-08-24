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

// credential é a identidade própria deste agente, obtida uma vez no enrollment
// e persistida em disco.
//
// Substitui o AGENT_TOKEN compartilhado, que era o mesmo em todos os agentes de
// todas as unidades: qualquer estação comprometida declarava a filial que
// quisesse e injetava métrica falsa nela.
type credential struct {
	DeviceID string `json:"device_id"`
	Token    string `json:"device_token"`
	SiteID   uint   `json:"site_id"`
	Kind     string `json:"kind"`
}

// credentialPath é onde a credencial vive. Fica ao lado da configuração do
// serviço, não no diretório do usuário: o agente roda como serviço de sistema e
// não tem HOME confiável.
func credentialPath() string {
	if v := strings.TrimSpace(os.Getenv("AGENT_CREDENTIAL_PATH")); v != "" {
		return v
	}
	if runtime.GOOS == "windows" {
		return filepath.Join(os.Getenv("ProgramData"), "vd-agent", "credential.json")
	}
	// /var/lib e não /etc: a credencial é obtida em tempo de execução, então é
	// estado e não configuração. E o serviço systemd roda com ProtectSystem=strict,
	// que deixa /etc somente-leitura — gravar ali falharia DEPOIS de o convite já
	// ter sido queimado no painel, que é a pior hora possível para falhar.
	return "/var/lib/vd-agent/credential.json"
}

// machineIDPath é o identificador estável da máquina, fornecido pelo sistema.
//
// Preferido ao hostname porque hostname muda: renomear a estação partiria o
// histórico dela em duas séries no painel.
func machineID() string {
	if v := strings.TrimSpace(os.Getenv("AGENT_MACHINE_ID")); v != "" {
		return v
	}

	// /etc/machine-id é padrão em qualquer Linux com systemd; o dbus é o
	// fallback das distribuições que não o criam.
	for _, caminho := range []string{"/etc/machine-id", "/var/lib/dbus/machine-id"} {
		if b, err := os.ReadFile(caminho); err == nil {
			if id := strings.TrimSpace(string(b)); id != "" {
				return id
			}
		}
	}

	// Sem identificador do sistema — Windows sem o registro lido, container sem
	// /etc/machine-id — geramos um e o guardamos junto da credencial. Vale menos
	// que o do sistema (reinstalar o agente gera outro), mas vale mais que
	// hostname, que muda sozinho.
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

	// 0600: o segredo vale por si só. Um arquivo legivel por todos entrega a
	// identidade do dispositivo a qualquer processo da maquina.
	return os.WriteFile(caminho, append(b, '\n'), 0o600)
}

// enroll troca o convite de uso unico pela credencial propria deste agente.
//
// Roda uma vez, na instalacao. Depois disso o convite ja foi queimado no painel
// e nao serve para mais nada, nem para quem o interceptar.
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
		// O corpo do erro é lido com teto: um painel mal configurado pode
		// devolver uma página inteira, e despejá-la no log do serviço não ajuda
		// ninguém.
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

// resolverIdentidade decide como este agente vai se autenticar.
//
// Ordem: credencial já persistida vence tudo; senão, um convite de enrollment é
// trocado por uma; senão, o AGENT_TOKEN compartilhado, que continua aceito
// durante a transição. Sem nenhum dos três o agente não sobe — um agente que
// roda sem conseguir enviar é pior que um que não roda, porque a máquina some
// do painel sem ninguém perceber.
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
			// Não é fatal: o agente já tem a credencial em memória e vai
			// reportar. Mas o convite foi queimado, então o próximo reinício
			// não conseguirá outra — e isso precisa gritar.
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
	log.Printf("[Agent] AVISO: usando AGENT_TOKEN compartilhado, que nao amarra este agente " +
		"a uma unidade. Migre para credencial propria com AGENT_ENROLL_TOKEN.")
	return credential{}, legado
}

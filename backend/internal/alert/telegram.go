package alert

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/database"
)

const defaultCooldown = 30 * time.Minute

var (
	cooldown = defaultCooldown
	agora    = time.Now
)

func configure() {
	cooldown = database.EnvDuration("ALERT_COOLDOWN", defaultCooldown)
}

var client = &http.Client{
	Timeout: 10 * time.Second,
	Transport: &http.Transport{
		TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
	},
	CheckRedirect: func(*http.Request, []*http.Request) error {
		return errors.New("redirecionamento recusado")
	},
}

var apiBase = "https://api.telegram.org"

var (
	mu       sync.Mutex
	lastSent = make(map[string]time.Time)
)

var (
	enabled bool
	token   string
	chatID  string
)

type telegramResponse struct {
	OK          bool            `json:"ok"`
	Description string          `json:"description"`
	Result      json.RawMessage `json:"result"`
}

func Init() {
	configure()
	token = os.Getenv("TELEGRAM_BOT_TOKEN")
	chatID = os.Getenv("TELEGRAM_CHAT_ID")

	switch {
	case token == "" && chatID == "":
		enabled = false
		marcarDesligado("Telegram não configurado")
		log.Println("[Alert] Telegram não configurado: alertas só no log")
		return
	case token == "":
		enabled = false
		marcarDesligado("TELEGRAM_CHAT_ID definido sem TELEGRAM_BOT_TOKEN")
		log.Println("[Alert] Telegram desligado: TELEGRAM_CHAT_ID definido sem TELEGRAM_BOT_TOKEN")
		return
	case chatID == "":
		enabled = false
		marcarDesligado("falta TELEGRAM_CHAT_ID")
		log.Println("[Alert] Telegram desligado: falta TELEGRAM_CHAT_ID (id numérico do chat, não o nome do bot)")
		return
	}

	enabled = true

	botName, err := verificarCanal()
	if err != nil {
		log.Printf("[Alert] Telegram configurado, mas a verificação falhou: %v. O envio continua sendo tentado e a verificação se repete em segundo plano", err)
		return
	}
	log.Printf("[Alert] Telegram ativo: bot @%s, chat %s", botName, chatID)
}

func verifyBot() (string, error) {
	var payload struct {
		Username string `json:"username"`
	}
	if err := call("getMe", nil, &payload); err != nil {
		return "", fmt.Errorf("TELEGRAM_BOT_TOKEN recusado (%w); pegue um novo com /mytoken no BotFather", err)
	}
	return payload.Username, nil
}

func verifyChat() error {
	if err := call("getChat", url.Values{"chat_id": {chatID}}, nil); err != nil {
		return fmt.Errorf("TELEGRAM_CHAT_ID %q inválido (%w); use o id numérico devolvido por getUpdates", chatID, err)
	}
	return nil
}

func redact(err error) error {
	if err == nil || token == "" {
		return err
	}
	return errors.New(strings.ReplaceAll(err.Error(), token, "<TELEGRAM_BOT_TOKEN>"))
}

func call(method string, form url.Values, out any) error {
	endpoint := fmt.Sprintf("%s/bot%s/%s", apiBase, token, method)

	resp, err := client.PostForm(endpoint, form)
	if err != nil {
		return redact(err)
	}
	defer resp.Body.Close()

	var body telegramResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return fmt.Errorf("resposta ilegível (status %d)", resp.StatusCode)
	}
	if !body.OK {
		return fmt.Errorf("%s", body.Description)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(body.Result, out)
}

func claimSlot(key string) bool {
	mu.Lock()
	defer mu.Unlock()

	now := agora()
	if t, ok := lastSent[key]; ok && now.Sub(t) < cooldown {
		return false
	}

	for k, t := range lastSent {
		if now.Sub(t) >= cooldown {
			delete(lastSent, k)
		}
	}

	lastSent[key] = now
	return true
}

var ErrSemCanal = errors.New("canal de alerta não configurado")

func Deliver(msg string) error {
	if !enabled {
		log.Printf("[Alert] (Telegram desligado) %s", msg)
		return ErrSemCanal
	}

	err := call("sendMessage", url.Values{
		"chat_id": {chatID},
		"text":    {msg},
	}, nil)
	if err != nil {
		marcarDegradado(err)
		return err
	}

	marcarOK()
	return nil
}

func Send(msg string) {
	if err := Deliver(msg); err != nil {
		log.Printf("[Alert] falha ao enviar Telegram: %v", err)
	}
}

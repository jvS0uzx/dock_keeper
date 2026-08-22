package alert

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"
)

// cooldown entre alertas repetidos da mesma chave, para não inundar o Telegram
// quando um container fica minutos fora do ar ou um domínio segue vencido.
const cooldown = 30 * time.Minute

var (
	mu       sync.Mutex
	lastSent = make(map[string]time.Time)
)

// Notify envia msg só se a mesma key não foi disparada dentro do cooldown.
func Notify(key, msg string) {
	mu.Lock()
	if t, ok := lastSent[key]; ok && time.Since(t) < cooldown {
		mu.Unlock()
		return
	}
	lastSent[key] = time.Now()
	mu.Unlock()

	Send(msg)
}

// Send dispara direto (sem cooldown). No-op se as env do bot não estão setadas.
func Send(msg string) {
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	chatID := os.Getenv("TELEGRAM_CHAT_ID")
	if token == "" || chatID == "" {
		log.Printf("[Alert] (Telegram desligado) %s", msg)
		return
	}

	api := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", token)
	resp, err := http.PostForm(api, url.Values{
		"chat_id":    {chatID},
		"text":       {msg},
		"parse_mode": {"Markdown"},
	})
	if err != nil {
		log.Printf("[Alert] falha ao enviar Telegram: %v", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Printf("[Alert] Telegram respondeu status %d", resp.StatusCode)
	}
}

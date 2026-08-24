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

	"gorm.io/gorm/clause"

	"github.com/joaov/vd_stats/internal/database"
)

// cooldown entre alertas repetidos da mesma chave, para não inundar o Telegram
// quando um container fica minutos fora do ar ou um domínio segue vencido.
const cooldown = 30 * time.Minute

// Cliente dedicado à API do Telegram.
//   - Timeout: o http.DefaultClient não tem nenhum, e uma resposta que nunca
//     chega seguraria a goroutine de coleta para sempre.
//   - MinVersion TLS 1.2: o token viaja no caminho da URL, então uma sessão
//     negociada em TLS 1.0/1.1 exporia o segredo a downgrade.
//   - CheckRedirect: recusa qualquer redirecionamento. Sem isso um 3xx (DNS
//     sequestrado, proxy hostil) faria o Go reenviar a URL — token incluso —
//     para o host de destino.
var client = &http.Client{
	Timeout: 10 * time.Second,
	Transport: &http.Transport{
		TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
	},
	CheckRedirect: func(*http.Request, []*http.Request) error {
		return errors.New("redirecionamento recusado")
	},
}

// Base da API. Variável para os testes apontarem para um servidor local.
var apiBase = "https://api.telegram.org"

// lastSent é o primeiro nível do cooldown, à frente da tabela.
//
// A verdade mora em database.AlertState — é o que faz o cooldown sobreviver a
// reinício e valer entre réplicas. O mapa existe só para não consultar o banco a
// cada tick: o motor de regras avalia todo alvo violando a cada rodada, e sem
// cache seriam centenas de SELECT por minuto para responder "ainda no cooldown".
// Depois da primeira consulta de cada chave, o processo responde de memória.
var (
	mu       sync.Mutex
	lastSent = make(map[string]time.Time)
)

// Credenciais validadas no boot. Enquanto enabled for falso todo alerta sai
// apenas no log — sem tentar HTTP a cada disparo.
var (
	enabled bool
	token   string
	chatID  string
)

// telegramResponse é o envelope padrão da API do Telegram.
type telegramResponse struct {
	OK          bool            `json:"ok"`
	Description string          `json:"description"`
	Result      json.RawMessage `json:"result"`
}

// Init valida as credenciais do bot uma única vez, no boot. Sem isso um token
// errado só aparece como falha solta no meio do log, um alerta por vez, e cada
// alerta ainda paga uma chamada HTTP que já se sabe que vai falhar.
func Init() {
	token = os.Getenv("TELEGRAM_BOT_TOKEN")
	chatID = os.Getenv("TELEGRAM_CHAT_ID")

	switch {
	case token == "" && chatID == "":
		log.Println("[Alert] Telegram não configurado: alertas só no log")
		return
	case token == "":
		log.Println("[Alert] Telegram desligado: TELEGRAM_CHAT_ID definido sem TELEGRAM_BOT_TOKEN")
		return
	case chatID == "":
		log.Println("[Alert] Telegram desligado: falta TELEGRAM_CHAT_ID (id numérico do chat, não o nome do bot)")
		return
	}

	botName, err := verifyBot()
	if err != nil {
		log.Printf("[Alert] Telegram desligado: %v", err)
		return
	}
	if err := verifyChat(); err != nil {
		log.Printf("[Alert] Telegram desligado: %v", err)
		return
	}

	enabled = true
	log.Printf("[Alert] Telegram ativo: bot @%s, chat %s", botName, chatID)
}

// verifyBot confirma o token em getMe e devolve o username do bot.
func verifyBot() (string, error) {
	var payload struct {
		Username string `json:"username"`
	}
	if err := call("getMe", nil, &payload); err != nil {
		return "", fmt.Errorf("TELEGRAM_BOT_TOKEN recusado (%w); pegue um novo com /mytoken no BotFather", err)
	}
	return payload.Username, nil
}

// verifyChat confirma que o chat_id existe e o bot alcança ele.
func verifyChat() error {
	if err := call("getChat", url.Values{"chat_id": {chatID}}, nil); err != nil {
		return fmt.Errorf("TELEGRAM_CHAT_ID %q inválido (%w); use o id numérico devolvido por getUpdates", chatID, err)
	}
	return nil
}

// redact troca o token por um marcador. O Telegram exige o token no caminho da
// URL, e o *url.Error do Go embute a URL inteira na mensagem — sem isso
// qualquer falha de rede grava o segredo no log do processo e no journald.
func redact(err error) error {
	if err == nil || token == "" {
		return err
	}
	return errors.New(strings.ReplaceAll(err.Error(), token, "<TELEGRAM_BOT_TOKEN>"))
}

// call executa um método da API do Telegram e decodifica o result em out.
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

// Notify envia msg só se a mesma key não foi disparada dentro do cooldown.
//
// Devolve se o aviso realmente saiu. Quem chama precisa saber: o motor de regras
// só marca o alerta como anunciado — e portanto só promete um aviso de
// recuperação depois — quando o operador de fato foi avisado.
func Notify(key, msg string) bool {
	if !claimSlot(key) {
		return false
	}
	Send(msg)
	return true
}

// claimSlot registra o disparo da chave e diz se ele pode acontecer agora.
//
// Consulta a memória primeiro e o banco só quando a memória não sabe. É o que
// faz o cooldown sobreviver a um reinício: antes o mapa zerava junto com o
// processo, e um painel que reiniciava — deploy, atualização, queda — recomeçava
// notificando tudo de novo, ensinando o operador a ignorar o canal.
func claimSlot(key string) bool {
	mu.Lock()
	defer mu.Unlock()

	now := time.Now()
	if t, ok := lastSent[key]; ok {
		if now.Sub(t) < cooldown {
			return false
		}
	} else if last, ok := lastNotifiedInDB(key); ok && now.Sub(last) < cooldown {
		// Guarda na memória para o próximo tick não voltar ao banco.
		lastSent[key] = last
		return false
	}

	// Aproveita a passada para descartar chaves já fora do cooldown; sem isso o
	// mapa cresce indefinidamente, uma entrada por container e domínio já visto.
	for k, t := range lastSent {
		if now.Sub(t) >= cooldown {
			delete(lastSent, k)
		}
	}

	lastSent[key] = now
	recordNotified(key, now)
	return true
}

// lastNotifiedInDB devolve quando a chave foi avisada pela última vez.
//
// O segundo retorno é falso tanto para "nunca avisada" quanto para "não deu para
// consultar", e os dois liberam o disparo de propósito: alerta duplicado
// incomoda, alerta perdido mata. Banco fora do ar não pode virar silêncio.
func lastNotifiedInDB(key string) (time.Time, bool) {
	if database.DB == nil {
		return time.Time{}, false
	}

	var state database.AlertState
	err := database.DB.Select("last_notified_at").
		Where("key = ?", key).Take(&state).Error
	if err != nil || state.LastNotifiedAt == nil {
		return time.Time{}, false
	}
	return *state.LastNotifiedAt, true
}

// recordNotified grava o disparo.
//
// Só last_notified_at entra no DoUpdates: as demais colunas de AlertState são
// escritas pelo motor de regras, que sabe quando a violação começou e se o
// alerta segue ativo. Duas metades da mesma linha, cada uma com o seu dono.
func recordNotified(key string, at time.Time) {
	if database.DB == nil {
		return
	}

	row := database.AlertState{Key: key, LastNotifiedAt: &at, UpdatedAt: at}
	err := database.DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"last_notified_at", "updated_at"}),
	}).Create(&row).Error
	if err != nil {
		log.Printf("[Alert] AVISO: cooldown de %q não foi persistido: %v", key, err)
	}
}

// Send dispara direto, sem cooldown. Cai para o log se o Telegram não passou
// na validação do boot.
func Send(msg string) {
	if !enabled {
		log.Printf("[Alert] (Telegram desligado) %s", msg)
		return
	}

	// Texto puro, sem parse_mode. A mensagem interpola nome de container, de
	// host e de domínio, que vêm do Docker, do DHCP e do DNS: são texto
	// arbitrário. Com Markdown, um nome contendo "_" ou "*" — `app_web_1` é o
	// caso comum — faz a API responder 400 e o alerta morre como linha de log,
	// justamente o alerta que mais importa. MarkdownV2 só trocaria o problema
	// de lugar: exigiria escapar corretamente todo ponto de interpolação, para
	// sempre, e um esquecido volta a falhar de forma intermitente.
	err := call("sendMessage", url.Values{
		"chat_id": {chatID},
		"text":    {msg},
	}, nil)
	if err != nil {
		log.Printf("[Alert] falha ao enviar Telegram: %v", err)
	}
}

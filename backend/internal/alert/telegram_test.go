package alert

import (
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/jvS0uzx/dock_keeper/internal/database"
)

// fakeTelegram sobe um servidor que responde como a API do Telegram e registra
// as chamadas recebidas, para o teste conferir o que o bot mandou.
func fakeTelegram(t *testing.T, replies map[string]string) *[]*http.Request {
	t.Helper()

	var got []*http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		got = append(got, r)

		method := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
		body, ok := replies[method]
		if !ok {
			body = `{"ok":false,"description":"metodo inesperado: ` + method + `"}`
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	base := apiBase
	apiBase = srv.URL
	t.Cleanup(func() { apiBase = base })

	return &got
}

func resetState() {
	enabled = false
	clear(lastSent)

	// O cooldown deixou de ser só memória: sem limpar a tabela, a segunda
	// execução da suíte contra o mesmo banco encontraria o disparo gravado pela
	// primeira e recusaria o primeiro claimSlot do teste.
	if database.DB != nil {
		database.DB.Where("key LIKE ? OR key LIKE ?", "container_down:%", "e12-teste:%").
			Delete(&database.AlertState{})
	}
}

func TestInitAtivaComCredenciaisValidas(t *testing.T) {
	resetState()
	calls := fakeTelegram(t, map[string]string{
		"getMe":   `{"ok":true,"result":{"username":"vd_stats_bot"}}`,
		"getChat": `{"ok":true,"result":{"id":123456789}}`,
	})
	t.Setenv("TELEGRAM_BOT_TOKEN", "123:abc")
	t.Setenv("TELEGRAM_CHAT_ID", "123456789")

	Init()

	if !enabled {
		t.Fatal("Init não habilitou o Telegram com credenciais válidas")
	}
	if len(*calls) != 2 {
		t.Fatalf("chamadas na validação = %d, esperado 2 (getMe + getChat)", len(*calls))
	}
}

func TestInitRecusaTokenInvalido(t *testing.T) {
	resetState()
	fakeTelegram(t, map[string]string{
		"getMe": `{"ok":false,"description":"Unauthorized"}`,
	})
	t.Setenv("TELEGRAM_BOT_TOKEN", "123:token-errado")
	t.Setenv("TELEGRAM_CHAT_ID", "123456789")

	Init()

	if enabled {
		t.Fatal("Init habilitou o Telegram com token recusado")
	}
}

// É exatamente o caso do TELEGRAM_CHAT_ID="botVdSats": token válido, chat que
// não existe. Sem a checagem de boot isso só apareceria alerta a alerta.
func TestInitRecusaChatIDInvalido(t *testing.T) {
	resetState()
	fakeTelegram(t, map[string]string{
		"getMe":   `{"ok":true,"result":{"username":"vd_stats_bot"}}`,
		"getChat": `{"ok":false,"description":"Bad Request: chat not found"}`,
	})
	t.Setenv("TELEGRAM_BOT_TOKEN", "123:abc")
	t.Setenv("TELEGRAM_CHAT_ID", "botVdSats")

	Init()

	if enabled {
		t.Fatal("Init habilitou o Telegram com chat_id inexistente")
	}
}

func TestInitSemCredenciaisNaoChamaAPI(t *testing.T) {
	resetState()
	calls := fakeTelegram(t, nil)
	t.Setenv("TELEGRAM_BOT_TOKEN", "")
	t.Setenv("TELEGRAM_CHAT_ID", "")

	Init()

	if enabled {
		t.Fatal("Init habilitou o Telegram sem credenciais")
	}
	if len(*calls) != 0 {
		t.Fatalf("chamou a API %d vezes sem credenciais", len(*calls))
	}
}

func TestSendMandaMensagemQuandoAtivo(t *testing.T) {
	resetState()
	calls := fakeTelegram(t, map[string]string{
		"getMe":       `{"ok":true,"result":{"username":"vd_stats_bot"}}`,
		"getChat":     `{"ok":true,"result":{"id":123456789}}`,
		"sendMessage": `{"ok":true,"result":{"message_id":1}}`,
	})
	t.Setenv("TELEGRAM_BOT_TOKEN", "123:abc")
	t.Setenv("TELEGRAM_CHAT_ID", "123456789")
	Init()

	Send("[ALERTA] teste")

	last := (*calls)[len(*calls)-1]
	if !strings.HasSuffix(last.URL.Path, "/sendMessage") {
		t.Fatalf("último método chamado = %s", last.URL.Path)
	}
	if got := last.PostForm.Get("text"); got != "[ALERTA] teste" {
		t.Errorf("text = %q", got)
	}
	if got := last.PostForm.Get("chat_id"); got != "123456789" {
		t.Errorf("chat_id = %q", got)
	}
}

// ativaTelegram valida credenciais contra o fake e deixa o envio habilitado.
func ativaTelegram(t *testing.T, sendMessage http.HandlerFunc) *[]*http.Request {
	t.Helper()

	var got []*http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		got = append(got, r)

		w.Header().Set("Content-Type", "application/json")
		switch method := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]; method {
		case "getMe":
			_, _ = w.Write([]byte(`{"ok":true,"result":{"username":"vd_stats_bot"}}`))
		case "getChat":
			_, _ = w.Write([]byte(`{"ok":true,"result":{"id":123456789}}`))
		case "sendMessage":
			sendMessage(w, r)
		default:
			_, _ = w.Write([]byte(`{"ok":false,"description":"metodo inesperado"}`))
		}
	}))
	t.Cleanup(srv.Close)

	base := apiBase
	apiBase = srv.URL
	t.Cleanup(func() { apiBase = base })

	resetState()
	t.Setenv("TELEGRAM_BOT_TOKEN", "123:abc")
	t.Setenv("TELEGRAM_CHAT_ID", "123456789")
	Init()

	return &got
}

// telegramComMarkdown imita o comportamento real da API: com parse_mode e
// entidade desbalanceada no texto, responde 400 e a mensagem não é entregue.
func telegramComMarkdown(w http.ResponseWriter, r *http.Request) {
	texto := r.PostForm.Get("text")
	if modo := r.PostForm.Get("parse_mode"); modo != "" && desbalanceado(texto) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"ok":false,"description":"Bad Request: can't parse entities"}`))
		return
	}
	_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1}}`))
}

func desbalanceado(texto string) bool {
	return strings.Count(texto, "_")%2 != 0 || strings.Count(texto, "*")%2 != 0
}

// O caso que motivou o E10, na forma exata que internal/ssh/client.go monta:
// nome de container entre asteriscos, e o nome trazendo um "_". O Markdown fica
// com entidade de itálico sem fechamento, a API responde 400 e o alerta some.
func TestSendEntregaNomeComUnderline(t *testing.T) {
	calls := ativaTelegram(t, telegramComMarkdown)

	var registro strings.Builder
	log.SetOutput(&registro)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	Send("[ALERTA] Container *nginx_proxy* está *exited* em 10.0.0.5")

	last := (*calls)[len(*calls)-1]
	if !strings.HasSuffix(last.URL.Path, "/sendMessage") {
		t.Fatalf("último método chamado = %s", last.URL.Path)
	}
	if saida := registro.String(); strings.Contains(saida, "falha ao enviar") {
		t.Fatalf("alerta com underline no nome virou linha de log: %s", saida)
	}
}

// Nome com asterisco tem a mesma origem: vem do Docker e do DNS, é texto
// arbitrário, e nenhum deles pode derrubar o envio.
func TestSendEntregaNomeComAsterisco(t *testing.T) {
	calls := ativaTelegram(t, telegramComMarkdown)

	var registro strings.Builder
	log.SetOutput(&registro)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	Send("[CRITICO] Certificado de *.grupoveloci.com.br inválido")

	last := (*calls)[len(*calls)-1]
	if got := last.PostForm.Get("text"); !strings.Contains(got, "*.grupoveloci.com.br") {
		t.Errorf("text = %q", got)
	}
	if saida := registro.String(); strings.Contains(saida, "falha ao enviar") {
		t.Fatalf("alerta com asterisco no nome virou linha de log: %s", saida)
	}
}

// Texto puro é a decisão, não um efeito colateral: qualquer parse_mode traz de
// volta a classe inteira de falha, porque o texto interpola nome de container,
// de host e de domínio, que ninguém controla.
func TestSendNaoUsaParseMode(t *testing.T) {
	calls := ativaTelegram(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1}}`))
	})

	Send("[AVISO] teste")

	last := (*calls)[len(*calls)-1]
	if modo := last.PostForm.Get("parse_mode"); modo != "" {
		t.Fatalf("parse_mode = %q; nome de container com _ ou * volta a derrubar o alerta", modo)
	}
}

// Com o Telegram desligado nenhum alerta pode gerar tráfego HTTP: seriam
// milhares de chamadas condenadas ao longo do dia.
func TestSendNaoChamaAPIQuandoDesligado(t *testing.T) {
	resetState()
	calls := fakeTelegram(t, nil)

	Send("[ALERTA] teste")

	if len(*calls) != 0 {
		t.Fatalf("chamou a API %d vezes com o Telegram desligado", len(*calls))
	}
}

func TestNotifyRespeitaCooldown(t *testing.T) {
	resetState()

	if !claimSlot("container_down:x") {
		t.Fatal("primeiro disparo foi bloqueado")
	}
	if claimSlot("container_down:x") {
		t.Fatal("disparo repetido passou pelo cooldown")
	}
	if !claimSlot("container_down:y") {
		t.Fatal("chave diferente foi bloqueada")
	}
}

// O Telegram exige o token no caminho da URL e o *url.Error do Go embute a URL
// inteira. Sem redação, uma falha de rede grava o segredo no log.
func TestErroDeRedeNaoVazaToken(t *testing.T) {
	resetState()
	base := apiBase
	apiBase = "https://127.0.0.1:1"
	t.Cleanup(func() { apiBase = base })

	token = "8804077626:SEGREDO-DO-BOT"
	t.Cleanup(func() { token = "" })

	err := call("getMe", nil, nil)
	if err == nil {
		t.Fatal("esperava erro de rede")
	}
	if strings.Contains(err.Error(), "SEGREDO-DO-BOT") {
		t.Fatalf("token vazou na mensagem de erro: %v", err)
	}
	if !strings.Contains(err.Error(), "<TELEGRAM_BOT_TOKEN>") {
		t.Fatalf("token não foi substituído pelo marcador: %v", err)
	}
}

// Um 3xx faria o Go reenviar a URL — com o token — para o host de destino.
func TestNaoSegueRedirecionamento(t *testing.T) {
	resetState()

	destino := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("token foi reenviado para o host de destino: %s", r.URL.Path)
	}))
	t.Cleanup(destino.Close)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destino.URL+r.URL.Path, http.StatusTemporaryRedirect)
	}))
	t.Cleanup(srv.Close)

	base := apiBase
	apiBase = srv.URL
	t.Cleanup(func() { apiBase = base })
	token = "123:abc"
	t.Cleanup(func() { token = "" })

	if err := call("getMe", nil, nil); err == nil {
		t.Fatal("redirecionamento foi seguido")
	}
}

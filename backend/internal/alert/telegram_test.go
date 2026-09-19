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
	resetSaude()
	clear(lastSent)

	if database.DB != nil {
		database.DB.Where("key LIKE ? OR key LIKE ?", "container_down:%", "e12-teste:%").
			Delete(&database.AlertState{})
		database.DB.Where("key LIKE ? OR key LIKE ?", "container_down:%", "e12-teste:%").
			Delete(&database.Alert{})
	}
}

func TestInitAtivaComCredenciaisValidas(t *testing.T) {
	resetState()
	calls := fakeTelegram(t, map[string]string{
		"getMe":   `{"ok":true,"result":{"username":"dockkeeper_bot"}}`,
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

func TestInitComTokenRecusadoFicaDegradado(t *testing.T) {
	resetState()
	fakeTelegram(t, map[string]string{
		"getMe": `{"ok":false,"description":"Unauthorized"}`,
	})
	t.Setenv("TELEGRAM_BOT_TOKEN", "123:token-errado")
	t.Setenv("TELEGRAM_CHAT_ID", "123456789")

	Init()

	if !enabled {
		t.Fatal("token recusado desligou o canal: configurado significa ligado, com estado degradado")
	}
	if e := Status().Estado; e != EstadoDegradado {
		t.Fatalf("estado = %q, esperado %q", e, EstadoDegradado)
	}
}

func TestInitComChatIDRecusadoFicaDegradado(t *testing.T) {
	resetState()
	fakeTelegram(t, map[string]string{
		"getMe":   `{"ok":true,"result":{"username":"dockkeeper_bot"}}`,
		"getChat": `{"ok":false,"description":"Bad Request: chat not found"}`,
	})
	t.Setenv("TELEGRAM_BOT_TOKEN", "123:abc")
	t.Setenv("TELEGRAM_CHAT_ID", "botVdSats")

	Init()

	if !enabled {
		t.Fatal("chat_id inexistente desligou o canal: configurado significa ligado, com estado degradado")
	}
	if e := Status().Estado; e != EstadoDegradado {
		t.Fatalf("estado = %q, esperado %q", e, EstadoDegradado)
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
		"getMe":       `{"ok":true,"result":{"username":"dockkeeper_bot"}}`,
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

func ativaTelegram(t *testing.T, sendMessage http.HandlerFunc) *[]*http.Request {
	t.Helper()

	var got []*http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		got = append(got, r)

		w.Header().Set("Content-Type", "application/json")
		switch method := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]; method {
		case "getMe":
			_, _ = w.Write([]byte(`{"ok":true,"result":{"username":"dockkeeper_bot"}}`))
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

func TestSendEntregaNomeComAsterisco(t *testing.T) {
	calls := ativaTelegram(t, telegramComMarkdown)

	var registro strings.Builder
	log.SetOutput(&registro)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	Send("[CRITICO] Certificado de *.example.com inválido")

	last := (*calls)[len(*calls)-1]
	if got := last.PostForm.Get("text"); !strings.Contains(got, "*.example.com") {
		t.Errorf("text = %q", got)
	}
	if saida := registro.String(); strings.Contains(saida, "falha ao enviar") {
		t.Fatalf("alerta com asterisco no nome virou linha de log: %s", saida)
	}
}

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

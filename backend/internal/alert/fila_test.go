package alert

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/database"
)

const prefixoFila = "fr02-teste:"

type telegramFalso struct {
	falhar atomic.Bool
	demora atomic.Int64

	mu       sync.Mutex
	porTexto map[string]int
}

func (f *telegramFalso) enviosCom(texto string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.porTexto[texto]
}

func (f *telegramFalso) responder(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.porTexto[r.FormValue("text")]++
	f.mu.Unlock()
	if d := f.demora.Load(); d > 0 {
		time.Sleep(time.Duration(d))
	}
	w.Header().Set("Content-Type", "application/json")
	if f.falhar.Load() {
		_, _ = w.Write([]byte(`{"ok":false,"description":"Bad Gateway"}`))
		return
	}
	_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1}}`))
}

func setupFila(t *testing.T) *telegramFalso {
	t.Helper()

	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("DATABASE_URL não definido; pulando teste da fila de alertas")
	}
	if database.DB == nil {
		if err := database.Connect(); err != nil {
			t.Skipf("banco indisponível: %v", err)
		}
	}

	falso := &telegramFalso{porTexto: map[string]int{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if strings.HasSuffix(r.URL.Path, "/sendMessage") {
			falso.responder(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{"username":"dockkeeper_bot"}}`))
	}))
	t.Cleanup(srv.Close)

	baseAPI, baseCooldown, baseAgora := apiBase, cooldown, agora
	apiBase = srv.URL
	token, chatID, enabled = "123:abc", "123456789", true
	marcarOK()

	limpar := func() {
		database.DB.Where("key LIKE ?", prefixoFila+"%").Delete(&database.Alert{})
	}
	limpar()
	t.Cleanup(func() {
		limpar()
		apiBase, cooldown, agora = baseAPI, baseCooldown, baseAgora
		resetState()
	})
	return falso
}

func Notify(key, msg string) bool {
	return Enqueue(Entrada{Key: key, Text: msg})
}

func alertasDaChave(t *testing.T, key string) []database.Alert {
	t.Helper()

	var linhas []database.Alert
	if err := database.DB.Where("key = ?", key).Order("id asc").Find(&linhas).Error; err != nil {
		t.Fatalf("ler alertas de %q: %v", key, err)
	}
	return linhas
}

func TestNotifyEnfileiraSemBloquearNoEnvio(t *testing.T) {
	falso := setupFila(t)
	falso.demora.Store(int64(2 * time.Second))
	chave := prefixoFila + "nao-bloqueia"

	inicio := time.Now()
	if !Notify(chave, "[CRITICO] disco cheio") {
		t.Fatal("Notify recusou o primeiro alerta da chave")
	}
	gasto := time.Since(inicio)

	if gasto > 300*time.Millisecond {
		t.Errorf("Notify levou %s: o envio continua no caminho de quem alerta", gasto)
	}
	linhas := alertasDaChave(t, chave)
	if len(linhas) != 1 {
		t.Fatalf("%d alerta(s) gravados, esperado 1", len(linhas))
	}
	if linhas[0].Status != database.AlertStatusOpen || linhas[0].Delivery != database.AlertDeliveryPendente {
		t.Errorf("alerta gravado como %s/%s, esperado open/pendente", linhas[0].Status, linhas[0].Delivery)
	}
	if n := falso.enviosCom("[CRITICO] disco cheio"); n != 0 {
		t.Errorf("o Telegram foi chamado %d vez(es) dentro do Notify", n)
	}
}

func TestCooldownOlhaAUltimaEntregaComSucesso(t *testing.T) {
	setupFila(t)
	chave := prefixoFila + "cooldown"
	cooldown = 30 * time.Minute

	base := time.Now().UTC()
	agora = func() time.Time { return base }
	if !Notify(chave, "[ALERTA] primeiro") {
		t.Fatal("primeiro alerta recusado")
	}
	dispatchPending(base)
	if linhas := alertasDaChave(t, chave); linhas[0].Delivery != database.AlertDeliveryEnviado {
		t.Fatalf("entrega = %q depois do despacho, esperado enviado", linhas[0].Delivery)
	}

	agora = func() time.Time { return base.Add(29 * time.Minute) }
	if Notify(chave, "[ALERTA] dentro do cooldown") {
		t.Error("alerta repetido dentro do cooldown virou linha nova")
	}
	if n := len(alertasDaChave(t, chave)); n != 1 {
		t.Errorf("%d linhas para a chave, esperado 1: suprimido não grava", n)
	}

	agora = func() time.Time { return base.Add(31 * time.Minute) }
	if !Notify(chave, "[ALERTA] depois do cooldown") {
		t.Error("alerta recusado depois do cooldown vencido")
	}
}

func TestTelegramForaNaoPerdeAlerta(t *testing.T) {
	falso := setupFila(t)
	falso.falhar.Store(true)
	chave := prefixoFila + "telegram-fora"

	base := time.Now().UTC()
	agora = func() time.Time { return base }
	Notify(chave, "[CRITICO] VPS inalcançável")
	dispatchPending(base)

	linhas := alertasDaChave(t, chave)
	if len(linhas) != 1 {
		t.Fatalf("%d alertas, esperado 1", len(linhas))
	}
	if linhas[0].Delivery != database.AlertDeliveryPendente {
		t.Errorf("entrega = %q, esperado pendente: a falha não pode perder o alerta", linhas[0].Delivery)
	}
	if linhas[0].Attempts != 1 || linhas[0].LastError == "" {
		t.Errorf("tentativas = %d, último erro = %q, esperado 1 e motivo gravado", linhas[0].Attempts, linhas[0].LastError)
	}
	if linhas[0].NextAttemptAt == nil || !linhas[0].NextAttemptAt.After(base) {
		t.Errorf("próxima tentativa = %v, esperado no futuro", linhas[0].NextAttemptAt)
	}

	falso.falhar.Store(false)
	depois := base.Add(2 * time.Minute)
	agora = func() time.Time { return depois }
	dispatchPending(depois)

	linhas = alertasDaChave(t, chave)
	if linhas[0].Delivery != database.AlertDeliveryEnviado {
		t.Errorf("entrega = %q depois do Telegram voltar, esperado enviado", linhas[0].Delivery)
	}
	if n := falso.enviosCom("[CRITICO] VPS inalcançável"); n != 2 {
		t.Errorf("envios = %d, esperado 2 (a falha e a entrega)", n)
	}
}

func TestBackoffCresceAteOTeto(t *testing.T) {
	esperado := []time.Duration{
		time.Minute, 2 * time.Minute, 4 * time.Minute, 8 * time.Minute,
		16 * time.Minute, 32 * time.Minute, time.Hour, time.Hour,
	}
	for i, quer := range esperado {
		if got := retryDelay(i + 1); got != quer {
			t.Errorf("espera da tentativa %d = %s, esperado %s", i+1, got, quer)
		}
	}
}

func TestAcimaDoTetoDeTentativasFalhaMasSegueAberto(t *testing.T) {
	falso := setupFila(t)
	falso.falhar.Store(true)
	t.Setenv("ALERT_MAX_ATTEMPTS", "2")
	chave := prefixoFila + "desiste"

	base := time.Now().UTC()
	agora = func() time.Time { return base }
	Notify(chave, "[CRITICO] sem entrega")

	for i := range 2 {
		quando := base.Add(time.Duration(i) * time.Hour)
		agora = func() time.Time { return quando }
		dispatchPending(quando)
	}

	linhas := alertasDaChave(t, chave)
	if linhas[0].Delivery != database.AlertDeliveryFalhou {
		t.Errorf("entrega = %q depois de 2 tentativas, esperado falhou", linhas[0].Delivery)
	}
	if linhas[0].Status != database.AlertStatusOpen {
		t.Errorf("status = %q, esperado open: desistir da entrega não fecha o alerta", linhas[0].Status)
	}
}

func TestDoisDespachantesNaoEnviamOMesmoAlerta(t *testing.T) {
	falso := setupFila(t)
	chave := prefixoFila + "concorrente"

	base := time.Now().UTC()
	agora = func() time.Time { return base }
	Notify(chave, "[ALERTA] concorrência")

	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			dispatchPending(base)
		}()
	}
	wg.Wait()

	if n := falso.enviosCom("[ALERTA] concorrência"); n != 1 {
		t.Errorf("o mesmo alerta foi enviado %d vezes, esperado 1", n)
	}
}

func TestRecuperacaoResolveEEnfileiraOK(t *testing.T) {
	setupFila(t)
	chave := prefixoFila + "recuperacao"

	base := time.Now().UTC()
	agora = func() time.Time { return base }
	Notify(chave, "[CRITICO] cpu acima do limite")
	dispatchPending(base)

	Recovered(Entrada{Key: chave, Text: "[INFO] Recuperado - cpu voltou ao limite", Severity: "info"})

	abertos := alertasDaChave(t, chave)
	if len(abertos) != 1 || abertos[0].Status != database.AlertStatusResolved {
		t.Fatalf("alerta original ficou como %v, esperado resolved", abertos)
	}
	if abertos[0].ResolvedAt == nil {
		t.Error("resolved_at não foi gravado")
	}

	ok := alertasDaChave(t, chave+":recuperacao")
	if len(ok) != 1 {
		t.Fatalf("%d mensagens de recuperação, esperado 1", len(ok))
	}
	if ok[0].Delivery != database.AlertDeliveryPendente {
		t.Errorf("recuperação entrou como %q, esperado pendente", ok[0].Delivery)
	}
}

func TestRecuperacaoDesligadaNaoEnfileiraOK(t *testing.T) {
	setupFila(t)
	t.Setenv("ALERT_NOTIFY_RECOVERY", "false")
	chave := prefixoFila + "sem-ok"

	base := time.Now().UTC()
	agora = func() time.Time { return base }
	Notify(chave, "[CRITICO] cpu acima do limite")
	Recovered(Entrada{Key: chave, Text: "[INFO] Recuperado", Severity: "info"})

	if n := len(alertasDaChave(t, chave+":recuperacao")); n != 0 {
		t.Errorf("%d mensagens de recuperação com ALERT_NOTIFY_RECOVERY=false, esperado 0", n)
	}
	if linhas := alertasDaChave(t, chave); linhas[0].Status != database.AlertStatusResolved {
		t.Errorf("status = %q, esperado resolved mesmo sem mensagem de recuperação", linhas[0].Status)
	}
}

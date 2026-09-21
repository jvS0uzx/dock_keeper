package alert

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/database"
)

type canalFalso struct {
	nome   string
	ativo  bool
	falha  error
	mu     sync.Mutex
	textos []string
}

func (c *canalFalso) Nome() string { return c.nome }

func (c *canalFalso) Ativo() bool { return c.ativo }

func (c *canalFalso) Entregar(a database.Alert) error {
	c.mu.Lock()
	c.textos = append(c.textos, textoDoAlerta(a))
	c.mu.Unlock()
	return c.falha
}

func (c *canalFalso) entregasCom(texto string) int {
	c.mu.Lock()
	defer c.mu.Unlock()

	n := 0
	for _, t := range c.textos {
		if t == texto {
			n++
		}
	}
	return n
}

func trocarRegistro(t *testing.T, canais ...Canal) {
	t.Helper()

	anterior := registro
	registro = canais
	t.Cleanup(func() { registro = anterior })
}

func linhasDeEntrega(t *testing.T, chave string) map[string]database.AlertDelivery {
	t.Helper()

	alerta := alertaDaChave(t, chave)
	var linhas []database.AlertDelivery
	if err := database.DB.Where("alert_id = ?", alerta.ID).Find(&linhas).Error; err != nil {
		t.Fatalf("ler a entrega por canal de %q: %v", chave, err)
	}

	porCanal := make(map[string]database.AlertDelivery, len(linhas))
	for _, l := range linhas {
		porCanal[l.Canal] = l
	}
	return porCanal
}

func TestUmCanalFalhaEOOutroEntregaNoMesmoAlerta(t *testing.T) {
	setupFila(t)
	bom := &canalFalso{nome: "bom", ativo: true}
	ruim := &canalFalso{nome: "ruim", ativo: true, falha: errors.New("recusado")}
	trocarRegistro(t, bom, ruim)

	chave := prefixoFila + "dois-canais"
	base := time.Now().UTC()
	agora = func() time.Time { return base }

	const texto = "[CRITICO] disco cheio no alerta de dois canais"
	Notify(chave, texto)
	dispatchPending(base)

	linhas := linhasDeEntrega(t, chave)
	if linhas["bom"].Status != database.AlertDeliveryEnviado {
		t.Errorf("canal bom ficou em %q, esperado enviado", linhas["bom"].Status)
	}
	if linhas["ruim"].Status != database.AlertDeliveryPendente {
		t.Errorf("canal ruim ficou em %q, esperado pendente para nova tentativa", linhas["ruim"].Status)
	}
	if linhas["ruim"].LastError == "" {
		t.Error("o canal que falhou não gravou o motivo")
	}
	if a := alertaDaChave(t, chave); a.Delivery != database.AlertDeliveryPendente {
		t.Errorf("alerta consolidado em %q, esperado pendente enquanto um canal não entregou", a.Delivery)
	}
	if n, m := bom.entregasCom(texto), ruim.entregasCom(texto); n != 1 || m != 1 {
		t.Errorf("entregas: bom=%d ruim=%d, esperado 1 em cada", n, m)
	}
}

func TestBackoffDeUmCanalNaoContaminaOVizinho(t *testing.T) {
	setupFila(t)
	bom := &canalFalso{nome: "bom", ativo: true}
	ruim := &canalFalso{nome: "ruim", ativo: true, falha: errors.New("recusado")}
	trocarRegistro(t, bom, ruim)

	chave := prefixoFila + "backoff-por-canal"
	base := time.Now().UTC()
	agora = func() time.Time { return base }

	const texto = "[CRITICO] disco cheio no alerta de backoff"
	Notify(chave, texto)
	dispatchPending(base)

	linhas := linhasDeEntrega(t, chave)
	if linhas["bom"].Attempts != 1 || linhas["ruim"].Attempts != 1 {
		t.Fatalf("tentativas: bom=%d ruim=%d, esperado 1 em cada", linhas["bom"].Attempts, linhas["ruim"].Attempts)
	}
	if linhas["bom"].NextAttemptAt != nil {
		t.Error("o canal que entregou ficou com nova tentativa marcada")
	}
	if linhas["ruim"].NextAttemptAt == nil || !linhas["ruim"].NextAttemptAt.After(base) {
		t.Fatalf("o canal que falhou ficou com próxima tentativa %v, esperado no futuro", linhas["ruim"].NextAttemptAt)
	}

	depois := base.Add(2 * time.Minute)
	agora = func() time.Time { return depois }
	dispatchPending(depois)

	linhas = linhasDeEntrega(t, chave)
	if linhas["bom"].Attempts != 1 {
		t.Errorf("tentativas do canal que já entregou = %d, esperado 1: o retry do vizinho reenviou", linhas["bom"].Attempts)
	}
	if linhas["ruim"].Attempts != 2 {
		t.Errorf("tentativas do canal que falha = %d, esperado 2", linhas["ruim"].Attempts)
	}
	if n := bom.entregasCom(texto); n != 1 {
		t.Errorf("o canal que já tinha entregue recebeu %d entregas, esperado 1", n)
	}
}

func TestTodosOsCanaisEntregamConsolidaEnviado(t *testing.T) {
	setupFila(t)
	um := &canalFalso{nome: "um", ativo: true}
	dois := &canalFalso{nome: "dois", ativo: true}
	trocarRegistro(t, um, dois)

	chave := prefixoFila + "todos-entregam"
	base := time.Now().UTC()
	agora = func() time.Time { return base }

	Notify(chave, "[ALERTA] cpu alta")
	dispatchPending(base)

	a := alertaDaChave(t, chave)
	if a.Delivery != database.AlertDeliveryEnviado {
		t.Errorf("delivery = %q, esperado enviado quando todos os canais entregaram", a.Delivery)
	}
	if a.LastNotifiedAt == nil {
		t.Error("last_notified_at não foi gravado com a entrega")
	}
}

func TestNenhumCanalConfiguradoNaoDizQueEntregou(t *testing.T) {
	setupFila(t)
	trocarRegistro(t, &canalFalso{nome: "desligado", ativo: false})

	chave := prefixoFila + "registro-vazio"
	base := time.Now().UTC()
	agora = func() time.Time { return base }

	Notify(chave, "[CRITICO] sem canal nenhum")
	dispatchPending(base)

	a := alertaDaChave(t, chave)
	if a.Delivery != database.AlertDeliverySemCanal {
		t.Errorf("delivery = %q, esperado sem_canal", a.Delivery)
	}
	if a.Status != database.AlertStatusOpen {
		t.Errorf("status = %q, o alerta precisa continuar aberto e visível", a.Status)
	}
	if len(linhasDeEntrega(t, chave)) != 0 {
		t.Error("sem canal ativo não pode nascer linha de entrega")
	}
}

func TestWebhookSoEAtivoComURL(t *testing.T) {
	t.Setenv("ALERT_WEBHOOK_URL", "")
	if (canalWebhook{}).Ativo() {
		t.Error("webhook ativo sem ALERT_WEBHOOK_URL")
	}
	t.Setenv("ALERT_WEBHOOK_URL", "https://exemplo.invalid/hook")
	if !(canalWebhook{}).Ativo() {
		t.Error("webhook inativo com ALERT_WEBHOOK_URL definido")
	}
}

func TestWebhookMandaOAlvoEstruturado(t *testing.T) {
	var recebido webhookCorpo
	var autorizacao, tipo string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		autorizacao = r.Header.Get("Authorization")
		tipo = r.Header.Get("Content-Type")
		_ = json.NewDecoder(r.Body).Decode(&recebido)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)

	t.Setenv("ALERT_WEBHOOK_URL", srv.URL)
	t.Setenv("ALERT_WEBHOOK_TOKEN", "segredo-de-teste")

	valor, limiar := 93.5, 90.0
	alerta := database.Alert{
		Key: "interface:sw-core:Gi0/1:erros", Severity: "high", Status: database.AlertStatusOpen,
		AlvoTipo: ponteiroDeTexto(database.AlvoTipoInterface),
		AlvoID:   ponteiroDeTexto("Gi0/1"),
		AlvoNome: ponteiroDeTexto("Gi0/1 em sw-core"),
		Metrica:  ponteiroDeTexto("uso"),
		Valor:    &valor, Limiar: &limiar,
		Unidade: ponteiroDeTexto("%"),
	}
	if err := (canalWebhook{}).Entregar(alerta); err != nil {
		t.Fatalf("entregar no webhook: %v", err)
	}

	if tipo != "application/json" {
		t.Errorf("content-type = %q", tipo)
	}
	if autorizacao != "Bearer segredo-de-teste" {
		t.Errorf("authorization = %q", autorizacao)
	}
	if recebido.Alvo.Tipo != database.AlvoTipoInterface || recebido.Alvo.ID != "Gi0/1" {
		t.Errorf("alvo = %+v", recebido.Alvo)
	}
	if recebido.Medida.Valor == nil || *recebido.Medida.Valor != valor {
		t.Errorf("valor = %v, esperado %v", recebido.Medida.Valor, valor)
	}
	if !strings.Contains(recebido.Texto, "interface Gi0/1 em sw-core") {
		t.Errorf("texto = %q, esperado descrever o alvo", recebido.Texto)
	}
}

func TestWebhookDevolveErroQuandoORemotoRecusa(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	t.Cleanup(srv.Close)

	t.Setenv("ALERT_WEBHOOK_URL", srv.URL)
	err := (canalWebhook{}).Entregar(database.Alert{Key: "x", Text: "[CRITICO] teste"})
	if err == nil {
		t.Fatal("webhook aceitou uma resposta 502 como entrega")
	}
}

func TestNtfyMandaTextoTituloEPrioridade(t *testing.T) {
	var caminho, titulo, prioridade, corpo string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		caminho = r.URL.Path
		titulo = r.Header.Get("Title")
		prioridade = r.Header.Get("Priority")
		b := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(b)
		corpo = string(b)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	t.Setenv("ALERT_NTFY_URL", srv.URL)
	t.Setenv("ALERT_NTFY_TOPIC", "dockkeeper-teste")

	alerta := database.Alert{
		Key: "host:vps-1:cpu", Severity: "critical", Text: "[CRITICO] cpu no talo",
		AlvoTipo: ponteiroDeTexto(database.AlvoTipoHost),
		AlvoNome: ponteiroDeTexto("vps-1"),
	}
	if err := (canalNtfy{}).Entregar(alerta); err != nil {
		t.Fatalf("entregar no ntfy: %v", err)
	}

	if caminho != "/dockkeeper-teste" {
		t.Errorf("caminho = %q, esperado o tópico", caminho)
	}
	if prioridade != "urgent" {
		t.Errorf("priority = %q, esperado urgent para severidade critical", prioridade)
	}
	if !strings.Contains(titulo, "vps-1") {
		t.Errorf("title = %q, esperado citar o alvo", titulo)
	}
	if corpo != "[CRITICO] cpu no talo" {
		t.Errorf("corpo = %q", corpo)
	}
}

func TestNtfySoEAtivoComTopico(t *testing.T) {
	t.Setenv("ALERT_NTFY_TOPIC", "")
	if (canalNtfy{}).Ativo() {
		t.Error("ntfy ativo sem ALERT_NTFY_TOPIC")
	}
	t.Setenv("ALERT_NTFY_TOPIC", "dockkeeper")
	if !(canalNtfy{}).Ativo() {
		t.Error("ntfy inativo com ALERT_NTFY_TOPIC definido")
	}
}

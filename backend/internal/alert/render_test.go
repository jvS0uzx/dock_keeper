package alert

import (
	"testing"

	"github.com/jvS0uzx/dock_keeper/internal/database"
)

func TestTextoPreservaOQueQuemAlertouEscreveu(t *testing.T) {
	alerta := database.Alert{
		Severity: "high", Text: "[ALERTA] texto pronto",
		AlvoTipo: ponteiroDeTexto(database.AlvoTipoHost),
		AlvoNome: ponteiroDeTexto("vps-1"),
	}
	if got := textoDoAlerta(alerta); got != "[ALERTA] texto pronto" {
		t.Errorf("texto = %q, esperado o que veio pronto", got)
	}
}

func TestTextoNasceDoAlvoEstruturado(t *testing.T) {
	valor, limiar := 93.5, 90.0
	casos := []struct {
		nome     string
		alerta   database.Alert
		esperado string
	}{
		{
			nome: "interface com medida",
			alerta: database.Alert{
				Severity: "high",
				AlvoTipo: ponteiroDeTexto(database.AlvoTipoInterface),
				AlvoNome: ponteiroDeTexto("Gi0/1 em sw-core"),
				Metrica:  ponteiroDeTexto("uso do uplink"),
				Valor:    &valor, Limiar: &limiar,
				Unidade: ponteiroDeTexto("%"),
			},
			esperado: "[ALERTA] interface Gi0/1 em sw-core: uso do uplink em 93.5 %, limite 90 %",
		},
		{
			nome: "container sem medida",
			alerta: database.Alert{
				Severity: "critical",
				AlvoTipo: ponteiroDeTexto(database.AlvoTipoContainer),
				AlvoNome: ponteiroDeTexto("api"),
			},
			esperado: "[CRITICO] container api",
		},
		{
			nome: "sem alvo cai na chave",
			alerta: database.Alert{
				Severity: "warning",
				Key:      "host:vps-1:disco",
			},
			esperado: "[AVISO] host:vps-1:disco",
		},
		{
			nome: "severidade desconhecida vira info",
			alerta: database.Alert{
				Severity: "inventada",
				AlvoTipo: ponteiroDeTexto(database.AlvoTipoHost),
				AlvoNome: ponteiroDeTexto("vps-9"),
			},
			esperado: "[INFO] host vps-9",
		},
	}

	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			if got := textoDoAlerta(caso.alerta); got != caso.esperado {
				t.Errorf("texto = %q, esperado %q", got, caso.esperado)
			}
		})
	}
}

func TestEnqueueSemTextoUsaOAlvoEstruturado(t *testing.T) {
	setupFila(t)
	chave := prefixoFila + "so-estruturado"
	valor := 97.0

	ok := Enqueue(Entrada{
		Key: chave, Severity: "critical",
		AlvoTipo: database.AlvoTipoHost, AlvoNome: "vps-3",
		Metrica: "disco", Valor: &valor, Unidade: "%",
	})
	if !ok {
		t.Fatal("alerta estruturado sem texto foi recusado")
	}

	a := alertaDaChave(t, chave)
	if a.Text != "[CRITICO] host vps-3: disco em 97 %" {
		t.Errorf("texto gravado = %q", a.Text)
	}
	if a.AlvoTipo == nil || *a.AlvoTipo != database.AlvoTipoHost {
		t.Errorf("alvo_tipo = %v, esperado host", a.AlvoTipo)
	}
	if a.Valor == nil || *a.Valor != valor {
		t.Errorf("valor = %v, esperado %v", a.Valor, valor)
	}
}

func TestEnqueueSemTextoENemAlvoERecusado(t *testing.T) {
	setupFila(t)

	if Enqueue(Entrada{Key: "", Severity: "critical"}) {
		t.Error("alerta sem chave foi aceito")
	}
}

package alert

import (
	"strconv"
	"strings"

	"github.com/jvS0uzx/dock_keeper/internal/database"
)

var prefixoDaSeveridade = map[string]string{
	"critical": "[CRITICO]",
	"high":     "[ALERTA]",
	"warning":  "[AVISO]",
	"info":     "[INFO]",
}

var rotuloDoTipoDeAlvo = map[string]string{
	database.AlvoTipoHost:      "host",
	database.AlvoTipoContainer: "container",
	database.AlvoTipoServico:   "serviço",
	database.AlvoTipoInterface: "interface",
}

func textoDoAlerta(a database.Alert) string {
	if texto := strings.TrimSpace(a.Text); texto != "" {
		return texto
	}
	return textoDoAlvo(a)
}

func textoDoAlvo(a database.Alert) string {
	prefixo, ok := prefixoDaSeveridade[strings.ToLower(strings.TrimSpace(a.Severity))]
	if !ok {
		prefixo = prefixoDaSeveridade["info"]
	}

	alvo := descricaoDoAlvo(a)
	medida := descricaoDaMedida(a)

	switch {
	case alvo != "" && medida != "":
		return prefixo + " " + alvo + ": " + medida
	case alvo != "":
		return prefixo + " " + alvo
	case medida != "":
		return prefixo + " " + medida
	case strings.TrimSpace(a.Key) != "":
		return prefixo + " " + strings.TrimSpace(a.Key)
	default:
		return ""
	}
}

func descricaoDoAlvo(a database.Alert) string {
	nome := textoDoPonteiro(a.AlvoNome)
	if nome == "" {
		nome = textoDoPonteiro(a.AlvoID)
	}
	tipo := rotuloDoTipoDeAlvo[textoDoPonteiro(a.AlvoTipo)]

	switch {
	case tipo != "" && nome != "":
		return tipo + " " + nome
	case nome != "":
		return nome
	default:
		return tipo
	}
}

func descricaoDaMedida(a database.Alert) string {
	metrica := textoDoPonteiro(a.Metrica)
	if metrica == "" {
		return ""
	}

	unidade := textoDoPonteiro(a.Unidade)
	medida := metrica
	if a.Valor != nil {
		medida += " em " + comUnidade(*a.Valor, unidade)
	}
	if a.Limiar != nil {
		medida += ", limite " + comUnidade(*a.Limiar, unidade)
	}
	return medida
}

func comUnidade(valor float64, unidade string) string {
	texto := strconv.FormatFloat(valor, 'f', -1, 64)
	if unidade == "" {
		return texto
	}
	return texto + " " + unidade
}

func textoDoPonteiro(v *string) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(*v)
}

func ponteiroDeTexto(v string) *string {
	limpo := strings.TrimSpace(v)
	if limpo == "" {
		return nil
	}
	return &limpo
}

func alertaDaEntrada(e Entrada) database.Alert {
	return database.Alert{
		Key:      e.Key,
		Severity: e.Severity,
		Text:     e.Text,
		AlvoTipo: ponteiroDeTexto(e.AlvoTipo),
		AlvoID:   ponteiroDeTexto(e.AlvoID),
		AlvoNome: ponteiroDeTexto(e.AlvoNome),
		Metrica:  ponteiroDeTexto(e.Metrica),
		Valor:    e.Valor,
		Limiar:   e.Limiar,
		Unidade:  ponteiroDeTexto(e.Unidade),
	}
}

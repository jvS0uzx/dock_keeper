package ssh

import (
	"strings"
	"testing"
	"time"
)

const linhaCombined = `203.0.113.9 - - [04/Oct/2026:10:00:00 +0000] "GET / HTTP/1.1" 200 612 "-" "curl/8.0"`
const linhaNoFormato = `203.0.113.9 - loja.exemplo to: 10.0.0.5:8080: GET / HTTP/1.1 200 upstream_response_time 0.010 msec 1759572000.000 request_time 0.010`

func observarLinhas(v *vigiaDeFormatoNginx, linhas []string, agora time.Time) []string {
	var avisos []string
	for _, l := range linhas {
		_, ok := parseNginxEntry(l)
		if aviso, emitir := v.observar(ok, agora); emitir {
			avisos = append(avisos, aviso)
		}
	}
	return avisos
}

func repetir(linha string, n int) []string {
	linhas := make([]string, n)
	for i := range linhas {
		linhas[i] = linha
	}
	return linhas
}

func TestVigiaDeFormatoAvisaUmaVezPorJanelaQuandoNadaEReconhecido(t *testing.T) {
	inicio := time.Date(2026, 10, 4, 10, 0, 0, 0, time.UTC)
	v := novoVigiaDeFormatoNginx("lb-01", inicio)

	if avisos := observarLinhas(v, repetir(linhaCombined, 500), inicio.Add(30*time.Second)); len(avisos) != 0 {
		t.Fatalf("avisou antes de fechar a janela: %v", avisos)
	}

	avisos := observarLinhas(v, repetir(linhaCombined, 500), inicio.Add(janelaDoFormatoNginx))
	if len(avisos) != 1 {
		t.Fatalf("esperado exatamente 1 aviso ao fechar a janela, vieram %d: %v", len(avisos), avisos)
	}
	for _, trecho := range []string{"[Nginx] lb-01:", "501 linhas recebidas, 0 reconhecidas", "docs/operacao.md#formato-do-access-log-do-nginx"} {
		if !strings.Contains(avisos[0], trecho) {
			t.Errorf("aviso %q não traz %q", avisos[0], trecho)
		}
	}

	proxima := inicio.Add(janelaDoFormatoNginx + time.Second)
	if avisos := observarLinhas(v, repetir(linhaCombined, 100), proxima); len(avisos) != 0 {
		t.Errorf("avisou de novo dentro da janela seguinte: %v", avisos)
	}
	if avisos := observarLinhas(v, repetir(linhaCombined, 1), proxima.Add(janelaDoFormatoNginx)); len(avisos) != 1 {
		t.Errorf("a janela seguinte, ainda sem nada reconhecido, deveria avisar uma vez; vieram %d", len(avisos))
	}
}

func TestVigiaDeFormatoNaoAvisaComTrafegoReconhecido(t *testing.T) {
	if _, ok := parseNginxEntry(linhaNoFormato); !ok {
		t.Fatalf("a linha de referência no formato exigido não foi reconhecida: %q", linhaNoFormato)
	}

	inicio := time.Date(2026, 10, 4, 10, 0, 0, 0, time.UTC)
	v := novoVigiaDeFormatoNginx("lb-01", inicio)

	linhas := append(repetir(linhaNoFormato, 50), repetir(linhaCombined, 50)...)
	if avisos := observarLinhas(v, linhas, inicio.Add(2*janelaDoFormatoNginx)); len(avisos) != 0 {
		t.Errorf("metade das linhas reconhecidas não é formato errado, mas veio aviso: %v", avisos)
	}
}

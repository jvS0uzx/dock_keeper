package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// decodeHandler imita o que as treze rotas de JSON fazem: decodifica o corpo e
// responde 400 quando ele não presta.
func decodeHandler(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "corpo inválido")
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"campos": len(body)})
}

func corpoJSON(bytes int) string {
	return `{"a":"` + strings.Repeat("x", bytes) + `"}`
}

func TestLimitBodyRecusaContentLengthAcimaDoTeto(t *testing.T) {
	h := limitBody(1024)(decodeHandler)

	req := httptest.NewRequest(http.MethodPost, "/api/qualquer", strings.NewReader(corpoJSON(4096)))
	rec := httptest.NewRecorder()
	h(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, esperado %d", rec.Code, http.StatusRequestEntityTooLarge)
	}
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("resposta não é JSON: %v", err)
	}
	if body["error"] == "" {
		t.Errorf("resposta deveria trazer o campo error no formato das demais rotas, veio %v", body)
	}
}

// Sem Content-Length o tamanho só aparece lendo, e é aí que o handler devolveria
// 400 "corpo inválido" — resposta que manda procurar erro de sintaxe onde o
// problema é tamanho.
func TestLimitBodyRecusaCorpoSemContentLength(t *testing.T) {
	h := limitBody(1024)(decodeHandler)

	// io.NopCloser esconde o tipo concreto do leitor, então httptest deixa o
	// ContentLength em -1, como num envio chunked.
	req := httptest.NewRequest(http.MethodPost, "/api/qualquer",
		io.NopCloser(strings.NewReader(corpoJSON(4096))))
	rec := httptest.NewRecorder()
	h(rec, req)

	if req.ContentLength != -1 {
		t.Fatalf("o teste precisa de ContentLength desconhecido, veio %d", req.ContentLength)
	}
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, esperado %d", rec.Code, http.StatusRequestEntityTooLarge)
	}
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("resposta não é JSON: %v", err)
	}
}

func TestLimitBodyDeixaPassarCorpoDentroDoTeto(t *testing.T) {
	h := limitBody(1024)(decodeHandler)

	req := httptest.NewRequest(http.MethodPost, "/api/qualquer", strings.NewReader(`{"a":"b","c":"d"}`))
	rec := httptest.NewRecorder()
	h(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, esperado %d", rec.Code, http.StatusOK)
	}
	var body map[string]int
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("resposta não é JSON: %v", err)
	}
	if body["campos"] != 2 {
		t.Errorf("o handler deveria ter lido o corpo inteiro, veio %v", body)
	}
}

// O teto precisa valer na malha de rotas, não só quando o middleware é montado
// à mão: é a montagem que decide se alguma rota ficou de fora.
func TestRotasDoMuxTemTetoDeCorpo(t *testing.T) {
	handler := Routes(testConfig())

	// Rota pública e rota autenticada: no segundo caso o teto tem de agir antes
	// da checagem de credencial, senão o corpo já foi lido para nada.
	for _, caso := range []struct{ metodo, path string }{
		{http.MethodPost, "/api/auth/login"},
		{http.MethodPost, "/api/sites"},
		{http.MethodPost, "/api/ingest/metrics"},
	} {
		req := httptest.NewRequest(caso.metodo, caso.path, strings.NewReader(corpoJSON(maxFormBodyBytes+1)))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusRequestEntityTooLarge {
			t.Errorf("%s %s: status = %d, esperado %d",
				caso.metodo, caso.path, rec.Code, http.StatusRequestEntityTooLarge)
		}
	}
}

// A planta baixa é imagem e não cabe no teto de formulário; o inventário de uma
// unidade inteira também não. Os dois precisam do teto próprio.
func TestRotasComTetoProprioAceitamCorpoMaiorQueOFormulario(t *testing.T) {
	handler := Routes(testConfig())

	for _, caso := range []struct{ path, esperado string }{
		{"/api/floorplans", "upload"},
		{"/api/ingest/inventory", "ingestão"},
	} {
		req := httptest.NewRequest(http.MethodPost, caso.path, strings.NewReader(corpoJSON(maxFormBodyBytes+1)))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code == http.StatusRequestEntityTooLarge {
			t.Errorf("%s (%s): corpo de %d bytes foi barrado pelo teto de formulário",
				caso.path, caso.esperado, maxFormBodyBytes+1)
		}
	}
}

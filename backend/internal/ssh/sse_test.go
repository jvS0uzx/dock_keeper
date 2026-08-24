package ssh

import (
	"net/http/httptest"
	"testing"
)

func TestSSEWriterFormataCadaLinhaComoEvento(t *testing.T) {
	rec := httptest.NewRecorder()
	w := newSSEWriter(rec, rec)

	w.send("primeira")
	w.send("segunda")

	esperado := "data: primeira\n\ndata: segunda\n\n"
	if got := rec.Body.String(); got != esperado {
		t.Errorf("corpo = %q, esperado %q", got, esperado)
	}
	if !rec.Flushed {
		t.Error("SSE sem flush: o navegador só veria os eventos no fim da resposta")
	}
}

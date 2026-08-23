package api

import (
	"encoding/json"
	"testing"
)

func TestParseOptionalUint(t *testing.T) {
	casos := map[string]struct {
		valor *uint
		ok    bool
	}{
		"42":    {ptrUint(42), true},
		" 7 ":   {ptrUint(7), true},
		"null":  {nil, true},
		"":      {nil, true},
		"0":     {nil, false},
		`"abc"`: {nil, false},
		"-1":    {nil, false},
		"[1,2]": {nil, false},
		"1.5":   {nil, false},
	}

	for entrada, esperado := range casos {
		valor, ok := parseOptionalUint(json.RawMessage(entrada))
		if ok != esperado.ok {
			t.Errorf("parseOptionalUint(%s): ok = %v, esperado %v", entrada, ok, esperado.ok)
			continue
		}
		switch {
		case esperado.valor == nil && valor != nil:
			t.Errorf("parseOptionalUint(%s) = %d, esperado nil", entrada, *valor)
		case esperado.valor != nil && valor == nil:
			t.Errorf("parseOptionalUint(%s) = nil, esperado %d", entrada, *esperado.valor)
		case esperado.valor != nil && *valor != *esperado.valor:
			t.Errorf("parseOptionalUint(%s) = %d, esperado %d", entrada, *valor, *esperado.valor)
		}
	}
}

// Regressão: com *uint — ou *json.RawMessage — o encoding/json anula o próprio
// ponteiro ao encontrar null, e "campo ausente" vira indistinguível de "campo
// enviado como null". No PATCH do host os dois são opostos: ausente preserva a
// unidade, null devolve o host ao controle automático do coletor.
func TestSiteIDDistingueAusenteDeNull(t *testing.T) {
	type corpo struct {
		SiteID json.RawMessage `json:"site_id"`
		Owner  *string         `json:"owner"`
	}

	var ausente corpo
	if err := json.Unmarshal([]byte(`{"owner":"TI"}`), &ausente); err != nil {
		t.Fatalf("decode ausente: %v", err)
	}
	if ausente.SiteID != nil {
		t.Errorf("campo ausente deveria ficar nil, veio %q", ausente.SiteID)
	}

	var nulo corpo
	if err := json.Unmarshal([]byte(`{"site_id":null}`), &nulo); err != nil {
		t.Fatalf("decode null: %v", err)
	}
	if nulo.SiteID == nil {
		t.Fatal("null explícito virou nil: ausente e null ficaram indistinguíveis")
	}
	if valor, ok := parseOptionalUint(nulo.SiteID); !ok || valor != nil {
		t.Errorf("null: valor = %v, ok = %v; esperado (nil, true)", valor, ok)
	}

	var numero corpo
	if err := json.Unmarshal([]byte(`{"site_id":9}`), &numero); err != nil {
		t.Fatalf("decode número: %v", err)
	}
	valor, ok := parseOptionalUint(numero.SiteID)
	if !ok || valor == nil || *valor != 9 {
		t.Errorf("número: valor = %v, ok = %v; esperado (9, true)", valor, ok)
	}
}

func ptrUint(n uint) *uint { return &n }

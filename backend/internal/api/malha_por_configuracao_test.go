package api

import (
	"testing"

	"github.com/jvS0uzx/dock_keeper/internal/database"
)

func TestMembroDaMalhaUsaConfiguracaoQuandoNaoHaTrafego(t *testing.T) {
	servidor := database.Server{Name: "NODE 2"}
	enderecos := []string{"198.51.100.20", "203.0.113.20"}
	declarados := map[string]bool{"203.0.113.20": true}

	atras, origem := membroDaMalha(servidor, enderecos, map[string]bool{}, declarados)
	if !atras || origem != "configuracao" {
		t.Errorf("membroDaMalha = (%v, %q), esperado (true, \"configuracao\")", atras, origem)
	}
}

func TestMembroDaMalhaPrefereTrafegoAConfiguracao(t *testing.T) {
	servidor := database.Server{Name: "NODE 1"}
	enderecos := []string{"203.0.113.10"}
	upstreams := map[string]bool{"203.0.113.10": true}
	declarados := map[string]bool{"203.0.113.10": true}

	atras, origem := membroDaMalha(servidor, enderecos, upstreams, declarados)
	if !atras || origem != "trafego" {
		t.Errorf("membroDaMalha = (%v, %q), esperado (true, \"trafego\")", atras, origem)
	}
}

func TestMembroDaMalhaManualVenceADescoberta(t *testing.T) {
	fora := false
	servidor := database.Server{Name: "NODE 3", BehindLB: &fora}
	enderecos := []string{"203.0.113.30"}
	declarados := map[string]bool{"203.0.113.30": true}

	atras, origem := membroDaMalha(servidor, enderecos, map[string]bool{}, declarados)
	if atras || origem != "manual" {
		t.Errorf("membroDaMalha = (%v, %q), esperado (false, \"manual\")", atras, origem)
	}
}

func TestMembroDaMalhaSemEvidenciaFicaFora(t *testing.T) {
	servidor := database.Server{Name: "NODE 4"}
	atras, origem := membroDaMalha(servidor, []string{"203.0.113.40"}, map[string]bool{}, map[string]bool{})
	if atras || origem != "nenhum" {
		t.Errorf("membroDaMalha = (%v, %q), esperado (false, \"nenhum\")", atras, origem)
	}
}

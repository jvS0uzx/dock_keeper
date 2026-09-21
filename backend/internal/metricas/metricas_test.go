package metricas

import (
	"os"
	"regexp"
	"slices"
	"testing"

	"github.com/jvS0uzx/dock_keeper/internal/database"
)

func TestCheckDoBancoBateComORegistro(t *testing.T) {
	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("DATABASE_URL não definido; pulando a conferência do CHECK")
	}
	if database.DB == nil {
		if err := database.Connect(); err != nil {
			t.Fatalf("conectar: %v", err)
		}
	}

	var definicao string
	err := database.DB.Raw(`SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE conname = 'ck_alert_rules_metric'`).
		Scan(&definicao).Error
	if err != nil || definicao == "" {
		t.Fatalf("ler ck_alert_rules_metric: %v (%q)", err, definicao)
	}

	var noBanco []string
	for _, m := range regexp.MustCompile(`'([a-z_0-9]+)'`).FindAllStringSubmatch(definicao, -1) {
		noBanco = append(noBanco, m[1])
	}
	noRegistro := NomesAvaliaveis()
	slices.Sort(noBanco)
	slices.Sort(noRegistro)
	if !slices.Equal(noBanco, noRegistro) {
		t.Errorf("CHECK do banco = %v, registro = %v: métrica nova avaliável pelo motor pede uma migração que recrie ck_alert_rules_metric", noBanco, noRegistro)
	}
}

func TestRegistroNaoTemNomeRepetidoNemEntradaManca(t *testing.T) {
	vistos := map[string]bool{}
	for _, m := range Todas() {
		if vistos[m.Nome] {
			t.Errorf("métrica %q aparece duas vezes", m.Nome)
		}
		vistos[m.Nome] = true
		if m.Rotulo == "" || m.SerieDoServidor == "" && m.SerieDoContainer == "" {
			t.Errorf("métrica %q sem rótulo ou sem coluna na série bruta", m.Nome)
		}
		temServidor := m.Escopo == EscopoServidor || m.Escopo == EscopoAmbos
		temContainer := m.Escopo == EscopoContainer || m.Escopo == EscopoAmbos
		if temServidor != (m.SerieDoServidor != "") || temContainer != (m.SerieDoContainer != "") {
			t.Errorf("métrica %q: escopo %q não bate com as colunas declaradas", m.Nome, m.Escopo)
		}
		if m.TendenciaMaxima != "" && m.TendenciaMedia == "" {
			t.Errorf("métrica %q tem máxima sem média na tendência", m.Nome)
		}
	}
}

func TestColunasDoRegistroExistemNoBanco(t *testing.T) {
	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("DATABASE_URL não definido")
	}
	if database.DB == nil {
		if err := database.Connect(); err != nil {
			t.Fatalf("conectar: %v", err)
		}
	}
	for _, m := range Todas() {
		for tabela, expr := range map[string]string{
			"metric_servers": m.SerieDoServidor, "metric_containers": m.SerieDoContainer,
			"metric_server_trends": m.TendenciaMedia, "metric_server_trends ": m.TendenciaMaxima,
		} {
			if expr == "" {
				continue
			}
			if err := database.DB.Exec("SELECT " + expr + " FROM " + tabela + " LIMIT 0").Error; err != nil {
				t.Errorf("métrica %q: %q não existe em %s: %v", m.Nome, expr, tabela, err)
			}
		}
	}
}

func NomesAvaliaveis() []string {
	var nomes []string
	for _, m := range registro {
		if m.Avaliavel() {
			nomes = append(nomes, m.Nome)
		}
	}
	return nomes
}

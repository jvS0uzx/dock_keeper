package discovery

import (
	"testing"
	"time"

	"github.com/joaov/vd_stats/internal/database"
)

const codigoUnidadeColetor = "qa-coletor"

// A varredura local e um coletor remoto na mesma unidade são dois escritores no
// mesmo inventário: cada ciclo sobrescreve o do outro, e a diferença entre as
// listas de porta faz o tipo do equipamento alternar sozinho a cada quinze
// minutos. O coletor vence porque enxerga a rede da filial; o painel só enxerga
// a dele.
func TestVarreduraLocalDesligaQuandoHaColetorNaUnidade(t *testing.T) {
	setupDB(t)
	site := criarUnidade(t, codigoUnidadeColetor)

	cred := database.DeviceCredential{
		DeviceID:   "qa-coletor-device",
		SecretHash: "irrelevante-para-este-teste",
		SiteID:     site,
		Kind:       "collector",
		MachineID:  "qa-coletor-maquina",
	}
	if err := database.DB.Create(&cred).Error; err != nil {
		t.Fatalf("criar credencial de coletor: %v", err)
	}
	t.Cleanup(func() {
		database.DB.Where("device_id = ?", cred.DeviceID).Delete(&database.DeviceCredential{})
	})

	s := &Sweeper{
		cfg:      Config{CIDRs: []string{"192.168.250.0/24"}}.withDefaults(),
		interval: time.Minute,
		siteCode: codigoUnidadeColetor,
	}
	if !s.Enabled() {
		t.Fatal("a varredura deveria começar ligada; o teste não mediria nada")
	}

	s.resolveSite()

	if s.Enabled() {
		t.Error("a varredura local continuou ligada numa unidade que já tem coletor registrado")
	}
}

// O contraponto que impede a proteção de virar desligamento indevido: coletor
// revogado não conta, senão revogar um dispositivo deixaria a unidade sem
// nenhuma fonte de inventário.
func TestColetorRevogadoNaoDesligaAVarredura(t *testing.T) {
	setupDB(t)
	site := criarUnidade(t, codigoUnidadeColetor)

	revogado := time.Now().UTC()
	cred := database.DeviceCredential{
		DeviceID:   "qa-coletor-revogado",
		SecretHash: "irrelevante-para-este-teste",
		SiteID:     site,
		Kind:       "collector",
		RevokedAt:  &revogado,
	}
	if err := database.DB.Create(&cred).Error; err != nil {
		t.Fatalf("criar credencial revogada: %v", err)
	}
	t.Cleanup(func() {
		database.DB.Where("device_id = ?", cred.DeviceID).Delete(&database.DeviceCredential{})
	})

	s := &Sweeper{
		cfg:      Config{CIDRs: []string{"192.168.250.0/24"}}.withDefaults(),
		interval: time.Minute,
		siteCode: codigoUnidadeColetor,
	}
	s.resolveSite()

	if !s.Enabled() {
		t.Error("a varredura foi desligada por um coletor já revogado")
	}
}

// Toda porta que a tabela de classificação consulta precisa estar na lista
// sondada. Sem 631, impressora que só publica IPP caía como "web-device"; sem
// 5000, NAS virava estação Windows por causa do 445. O coletor remoto já sondava
// as doze, e o mesmo equipamento mudava de tipo conforme quem o encontrasse.
func TestPortasSondadasCobremATabelaDeClassificacao(t *testing.T) {
	sondadas := make(map[int]bool, len(DefaultPorts))
	for _, p := range DefaultPorts {
		sondadas[p] = true
	}

	for _, regra := range fingerprints {
		if !sondadas[regra.Port] {
			t.Errorf("a porta %d classifica como %q mas não é sondada por DefaultPorts",
				regra.Port, regra.Type)
		}
	}
}

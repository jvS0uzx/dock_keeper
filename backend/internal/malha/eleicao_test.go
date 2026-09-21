package malha

import (
	"testing"

	"github.com/jvS0uzx/dock_keeper/internal/database"
)

const (
	ciclosDeTeste = 2
	margemDeTeste = 1.25
)

type simulador struct {
	membros  []Membro
	pendente int
}

func novoSimulador(membros []Membro) *simulador {
	return &simulador{membros: membros}
}

func (s *simulador) rodar(voltas int) Decisao {
	var d Decisao
	for i := 0; i < voltas; i++ {
		d = decidir(s.membros, s.pendente, ciclosDeTeste, margemDeTeste)
		s.pendente = d.Desafios
		for j := range s.membros {
			s.membros[j].PapelAtual = d.Papeis[s.membros[j].ID]
		}
	}
	return d
}

func TestTrafegoClaroElegeOLiderDepoisDosCiclos(t *testing.T) {
	membros := []Membro{
		{ID: "a", Nome: "borda-1", Candidato: true, Requisicoes: 100},
		{ID: "b", Nome: "borda-2", Candidato: true, Requisicoes: 10},
	}

	sim := novoSimulador(membros)
	if d := sim.rodar(1); d.Principal != "" {
		t.Errorf("principal na primeira volta = %q, esperado nenhum: um ciclo só não sustenta a eleição", d.Principal)
	}
	d := sim.rodar(1)
	if d.Principal != "a" {
		t.Fatalf("principal = %q, esperado a depois de %d ciclos", d.Principal, ciclosDeTeste)
	}
	if d.Papeis["b"] != database.NginxPapelReserva {
		t.Errorf("papel de b = %q, esperado reserva", d.Papeis["b"])
	}
}

func TestEmpateNaoElegeNinguem(t *testing.T) {
	membros := []Membro{
		{ID: "a", Nome: "borda-1", Candidato: true, Requisicoes: 50},
		{ID: "b", Nome: "borda-2", Candidato: true, Requisicoes: 50},
	}

	d := novoSimulador(membros).rodar(5)
	if d.Principal != "" {
		t.Errorf("principal = %q, esperado nenhum: empate não pode virar sorteio", d.Principal)
	}
	for _, id := range []string{"a", "b"} {
		if d.Papeis[id] != database.NginxPapelReserva {
			t.Errorf("papel de %s = %q, esperado reserva", id, d.Papeis[id])
		}
	}
}

func TestTrafegoZeroPreservaOPrincipal(t *testing.T) {
	membros := []Membro{
		{ID: "a", Nome: "borda-1", Candidato: true, Requisicoes: 0, PapelAtual: database.NginxPapelPrincipal},
		{ID: "b", Nome: "borda-2", Candidato: true, Requisicoes: 0, PapelAtual: database.NginxPapelReserva},
	}

	d := novoSimulador(membros).rodar(10)
	if d.Principal != "a" {
		t.Errorf("principal = %q, esperado a: madrugada sem requisição não derruba o principal", d.Principal)
	}
	if d.Trocou() {
		t.Errorf("a decisão acusou troca sem tráfego nenhum")
	}
}

func TestPicoIsoladoNaoTrocaOPrincipal(t *testing.T) {
	membros := []Membro{
		{ID: "a", Nome: "borda-1", Candidato: true, Requisicoes: 100, PapelAtual: database.NginxPapelPrincipal},
		{ID: "b", Nome: "borda-2", Candidato: true, Requisicoes: 400, PapelAtual: database.NginxPapelReserva},
	}

	sim := novoSimulador(membros)
	if d := sim.rodar(1); d.Principal != "a" {
		t.Fatalf("principal no ciclo do pico = %q, esperado a", d.Principal)
	}

	membros[1].Requisicoes = 10
	d := sim.rodar(3)
	if d.Principal != "a" {
		t.Errorf("principal = %q, esperado a: um pico isolado não sustenta a troca", d.Principal)
	}
}

func TestMargemMinimaSeguraTrocaPorDiferencaPequena(t *testing.T) {
	membros := []Membro{
		{ID: "a", Nome: "borda-1", Candidato: true, Requisicoes: 100, PapelAtual: database.NginxPapelPrincipal},
		{ID: "b", Nome: "borda-2", Candidato: true, Requisicoes: 120, PapelAtual: database.NginxPapelReserva},
	}

	sim := novoSimulador(membros)
	if d := sim.rodar(6); d.Principal != "a" {
		t.Errorf("principal = %q, esperado a: 120 não supera 100 pela margem de 25%%", d.Principal)
	}

	membros[1].Requisicoes = 400
	if d := sim.rodar(ciclosDeTeste); d.Principal != "b" {
		t.Errorf("principal = %q, esperado b depois de superar a margem por %d ciclos", d.Principal, ciclosDeTeste)
	}
}

func TestFailoverElegeQuemPassouAReceberOTrafego(t *testing.T) {
	membros := []Membro{
		{ID: "a", Nome: "borda-1", Candidato: true, Requisicoes: 0, PapelAtual: database.NginxPapelPrincipal},
		{ID: "b", Nome: "borda-2", Candidato: true, Requisicoes: 500, PapelAtual: database.NginxPapelReserva},
	}

	d := novoSimulador(membros).rodar(ciclosDeTeste)
	if d.Principal != "b" {
		t.Fatalf("principal = %q, esperado b: o tráfego inteiro migrou para ele", d.Principal)
	}
	if d.Anterior != "a" || !d.Trocou() {
		t.Errorf("anterior = %q trocou = %v, esperado a e true", d.Anterior, d.Trocou())
	}
	if d.Papeis["a"] != database.NginxPapelReserva {
		t.Errorf("papel de a = %q, esperado reserva depois de perder o posto", d.Papeis["a"])
	}
}

func TestIncumbenteQueDeixouDeSerCandidatoPerdeOPapel(t *testing.T) {
	membros := []Membro{
		{ID: "a", Nome: "borda-1", Candidato: false, Requisicoes: 0, PapelAtual: database.NginxPapelPrincipal},
		{ID: "b", Nome: "borda-2", Candidato: true, Requisicoes: 0, PapelAtual: database.NginxPapelReserva},
	}

	sim := novoSimulador(membros)
	if d := sim.rodar(1); d.Principal != "a" {
		t.Fatalf("principal na primeira volta = %q, esperado a: um ciclo só pode ser tropeço da sonda", d.Principal)
	}
	d := sim.rodar(1)
	if d.Principal != "" {
		t.Errorf("principal = %q, esperado nenhum: sem Nginx e sem tráfego não há quem provar", d.Principal)
	}
	if d.Papeis["a"] != database.NginxPapelNenhum {
		t.Errorf("papel de a = %q, esperado nenhum", d.Papeis["a"])
	}
}

func TestSemCandidatoNinguemTemPapel(t *testing.T) {
	membros := []Membro{
		{ID: "a", Nome: "app-1", Candidato: false, Requisicoes: 0},
		{ID: "b", Nome: "app-2", Candidato: false, Requisicoes: 0},
	}

	d := novoSimulador(membros).rodar(3)
	if d.Principal != "" {
		t.Errorf("principal = %q, esperado nenhum", d.Principal)
	}
	for _, id := range []string{"a", "b"} {
		if d.Papeis[id] != database.NginxPapelNenhum {
			t.Errorf("papel de %s = %q, esperado nenhum", id, d.Papeis[id])
		}
	}
}

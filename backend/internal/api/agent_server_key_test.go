package api

import (
	"os"
	"testing"

	"github.com/joaov/vd_stats/internal/database"
)

// Testes de findOrCreateAgentServer (item N4 do checklist): a parte de
// armazenamento da chave (unidade, hostname) já era coberta em
// internal/database; a adoção e a precedência do machine_id, que vivem aqui,
// não eram.

func setupChaveAgente(t *testing.T) (uint, uint) {
	t.Helper()

	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("DATABASE_URL não definido; pulando teste de chave do agente")
	}
	if database.DB == nil {
		if err := database.Connect(); err != nil {
			t.Skipf("banco indisponível: %v", err)
		}
	}
	limparChaveAgente(t)
	t.Cleanup(func() { limparChaveAgente(t) })

	sedeA := database.Site{Name: "qa-n4-a", Code: "qa-n4-a"}
	sedeB := database.Site{Name: "qa-n4-b", Code: "qa-n4-b"}
	if err := database.DB.Create(&sedeA).Error; err != nil {
		t.Fatalf("criar unidade A: %v", err)
	}
	if err := database.DB.Create(&sedeB).Error; err != nil {
		t.Fatalf("criar unidade B: %v", err)
	}
	return sedeA.ID, sedeB.ID
}

func limparChaveAgente(t *testing.T) {
	t.Helper()
	database.DB.Unscoped().Where("name LIKE ?", "qa-n4-%").Delete(&database.Server{})
	database.DB.Where("code LIKE ?", "qa-n4-%").Delete(&database.Site{})
}

func contarServidores(t *testing.T, nome string) int64 {
	t.Helper()
	var n int64
	if err := database.DB.Model(&database.Server{}).Where("name = ?", nome).Count(&n).Error; err != nil {
		t.Fatalf("contar servidores %q: %v", nome, err)
	}
	return n
}

// Agente que não mandava site_code gravou site_id nulo. Quando ele passa a
// mandar, o registro antigo tem de ser adotado — abrir linha nova partiria o
// histórico da mesma máquina em duas séries.
func TestAgenteAdotaServidorSemUnidade(t *testing.T) {
	sedeA, _ := setupChaveAgente(t)

	orfao := database.Server{Name: "qa-n4-adotado", HostIP: "10.94.1.1", Kind: "agent"}
	if err := database.DB.Create(&orfao).Error; err != nil {
		t.Fatalf("semear servidor sem unidade: %v", err)
	}

	achado, err := findOrCreateAgentServer("qa-n4-adotado", "", "10.94.1.2", &sedeA)
	if err != nil {
		t.Fatalf("findOrCreateAgentServer: %v", err)
	}
	if achado.ID != orfao.ID {
		t.Errorf("adoção criou registro novo: achado %s, semeado %s", achado.ID, orfao.ID)
	}
	if n := contarServidores(t, "qa-n4-adotado"); n != 1 {
		t.Errorf("servidores com o nome = %d, esperado 1", n)
	}
}

// Dois DESKTOP-01 em filiais diferentes são dois equipamentos. Sob a chave
// antiga viravam um registro só, com as métricas das duas máquinas serradas na
// mesma série.
func TestMesmoHostnameCoexisteEmUnidadesDiferentes(t *testing.T) {
	sedeA, sedeB := setupChaveAgente(t)

	naA, err := findOrCreateAgentServer("qa-n4-dup", "", "10.94.2.1", &sedeA)
	if err != nil {
		t.Fatalf("criar na unidade A: %v", err)
	}
	naB, err := findOrCreateAgentServer("qa-n4-dup", "", "10.94.2.2", &sedeB)
	if err != nil {
		t.Fatalf("criar na unidade B: %v", err)
	}
	if naA.ID == naB.ID {
		t.Fatalf("o mesmo registro atendeu duas unidades: %s", naA.ID)
	}

	// Repetir o push da unidade A devolve o registro dela, não um terceiro.
	deNovo, err := findOrCreateAgentServer("qa-n4-dup", "", "10.94.2.1", &sedeA)
	if err != nil {
		t.Fatalf("repetir na unidade A: %v", err)
	}
	if deNovo.ID != naA.ID {
		t.Errorf("push repetido devolveu %s, esperado %s", deNovo.ID, naA.ID)
	}
	if n := contarServidores(t, "qa-n4-dup"); n != 2 {
		t.Errorf("servidores com o nome = %d, esperado 2", n)
	}
}

// Renomear a máquina não pode partir o histórico: quando o machine_id existe,
// ele vence o hostname.
func TestMachineIDVenceHostname(t *testing.T) {
	sedeA, _ := setupChaveAgente(t)

	existente := database.Server{
		Name: "qa-n4-nome-antigo", HostIP: "10.94.3.1", Kind: "agent",
		SiteID: &sedeA, MachineID: "qa-n4-mid-1",
	}
	if err := database.DB.Create(&existente).Error; err != nil {
		t.Fatalf("semear servidor com machine_id: %v", err)
	}

	achado, err := findOrCreateAgentServer("qa-n4-nome-novo", "qa-n4-mid-1", "10.94.3.1", &sedeA)
	if err != nil {
		t.Fatalf("findOrCreateAgentServer: %v", err)
	}
	if achado.ID != existente.ID {
		t.Errorf("máquina renomeada virou registro novo: achado %s, semeado %s", achado.ID, existente.ID)
	}
	if n := contarServidores(t, "qa-n4-nome-novo"); n != 0 {
		t.Errorf("registro novo criado para o nome novo: %d", n)
	}
}

// Quando nada casa, o registro nasce com a unidade, o machine_id e o tipo
// certos — é o que o upsert do push atualiza depois.
func TestHostNovoQuandoNadaCasa(t *testing.T) {
	sedeA, _ := setupChaveAgente(t)

	criado, err := findOrCreateAgentServer("qa-n4-novo", "qa-n4-mid-2", "10.94.4.1", &sedeA)
	if err != nil {
		t.Fatalf("findOrCreateAgentServer: %v", err)
	}
	if criado.ID == "" {
		t.Fatal("registro criado sem id")
	}

	var doBanco database.Server
	if err := database.DB.First(&doBanco, "id = ?", criado.ID).Error; err != nil {
		t.Fatalf("reler servidor criado: %v", err)
	}
	if doBanco.Kind != "agent" {
		t.Errorf("kind = %q, esperado agent", doBanco.Kind)
	}
	if doBanco.SiteID == nil || *doBanco.SiteID != sedeA {
		t.Errorf("site_id = %v, esperado %d", doBanco.SiteID, sedeA)
	}
	if doBanco.MachineID != "qa-n4-mid-2" {
		t.Errorf("machine_id = %q, esperado qa-n4-mid-2", doBanco.MachineID)
	}
}

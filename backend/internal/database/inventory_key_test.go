package database

import (
	"os"
	"testing"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// A unicidade do inventário é imposta pelo Postgres, num índice de expressão
// que nenhuma tag do GORM descreve. Verificar isso sem banco testaria a string
// do DDL, não o comportamento — por isso o teste é de integração e pula sem
// DATABASE_URL, como o de retenção.
func setupInventoryDB(t *testing.T) {
	t.Helper()

	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("DATABASE_URL não definido; pulando teste de chave do inventário")
	}
	if DB == nil {
		if err := Connect(); err != nil {
			t.Skipf("banco indisponível: %v", err)
		}
	}
	limpaUnidadesDeTeste(t)
	t.Cleanup(func() { limpaUnidadesDeTeste(t) })
}

const (
	codigoUnidadeA = "zz-teste-a"
	codigoUnidadeB = "zz-teste-b"
	ipCompartilado = "192.168.0.10"
	hostnameComum  = "ZZ-TESTE-DESKTOP-01"
)

func limpaUnidadesDeTeste(t *testing.T) {
	t.Helper()

	var sites []Site
	DB.Where("code IN ?", []string{codigoUnidadeA, codigoUnidadeB}).Find(&sites)
	for _, s := range sites {
		DB.Where("site_id = ?", s.ID).Delete(&NetworkHost{})
		DB.Unscoped().Where("site_id = ?", s.ID).Delete(&Server{})
	}
	DB.Where("ip = ?", ipCompartilado).Delete(&NetworkHost{})
	DB.Unscoped().Where("name = ?", hostnameComum).Delete(&Server{})
	DB.Where("code IN ?", []string{codigoUnidadeA, codigoUnidadeB}).Delete(&Site{})
}

func criaUnidade(t *testing.T, code string) uint {
	t.Helper()

	site := Site{Name: "Unidade de teste " + code, Code: code}
	if err := DB.Create(&site).Error; err != nil {
		t.Fatalf("criar unidade %s: %v", code, err)
	}
	return site.ID
}

func upsertHost(t *testing.T, siteID uint, hostname string) error {
	t.Helper()

	now := time.Now().UTC()
	host := NetworkHost{
		IP:        ipCompartilado,
		Hostname:  hostname,
		SiteID:    &siteID,
		FirstSeen: now,
		LastSeen:  now,
	}
	return DB.Clauses(clause.OnConflict{
		Columns: NetworkHostConflictTarget(),
		DoUpdates: clause.Assignments(map[string]any{
			"last_seen": now,
			"hostname":  gorm.Expr("EXCLUDED.hostname"),
		}),
	}).Create(&host).Error
}

// O achado E2: 192.168.0.0/24 existe em toda filial. Com o índice único global
// no IP, o mesmo endereço em duas unidades disputava uma linha só e cada
// coletor sobrescrevia o host do outro a cada ciclo.
func TestMesmoIPEmUnidadesDiferentesCoexiste(t *testing.T) {
	setupInventoryDB(t)

	unidadeA := criaUnidade(t, codigoUnidadeA)
	unidadeB := criaUnidade(t, codigoUnidadeB)

	if err := upsertHost(t, unidadeA, "impressora-a"); err != nil {
		t.Fatalf("gravar host da unidade A: %v", err)
	}
	if err := upsertHost(t, unidadeB, "impressora-b"); err != nil {
		t.Fatalf("gravar host da unidade B: %v", err)
	}

	var hosts []NetworkHost
	DB.Where("ip = ?", ipCompartilado).Order("site_id").Find(&hosts)
	if len(hosts) != 2 {
		t.Fatalf("hosts gravados = %d, esperado 2 (um por unidade)", len(hosts))
	}
	if hosts[0].Hostname == hosts[1].Hostname {
		t.Error("um coletor sobrescreveu o hostname do outro")
	}
}

// O ciclo seguinte do mesmo coletor atualiza a linha da unidade dele, sem criar
// duplicata e sem tocar na outra unidade.
func TestSegundoCicloAtualizaSoAPropriaUnidade(t *testing.T) {
	setupInventoryDB(t)

	unidadeA := criaUnidade(t, codigoUnidadeA)
	unidadeB := criaUnidade(t, codigoUnidadeB)

	if err := upsertHost(t, unidadeA, "impressora-a"); err != nil {
		t.Fatalf("primeiro ciclo da unidade A: %v", err)
	}
	if err := upsertHost(t, unidadeB, "impressora-b"); err != nil {
		t.Fatalf("primeiro ciclo da unidade B: %v", err)
	}
	if err := upsertHost(t, unidadeA, "impressora-a-renomeada"); err != nil {
		t.Fatalf("segundo ciclo da unidade A: %v", err)
	}

	var hosts []NetworkHost
	DB.Where("ip = ?", ipCompartilado).Find(&hosts)
	if len(hosts) != 2 {
		t.Fatalf("hosts gravados = %d, esperado 2", len(hosts))
	}

	for _, h := range hosts {
		if *h.SiteID == unidadeB && h.Hostname != "impressora-b" {
			t.Errorf("a unidade B foi alterada por um ciclo da unidade A: %q", h.Hostname)
		}
		if *h.SiteID == unidadeA && h.Hostname != "impressora-a-renomeada" {
			t.Errorf("o segundo ciclo da unidade A não atualizou: %q", h.Hostname)
		}
	}
}

// Host sem unidade continua único pelo IP: o COALESCE do índice existe
// justamente porque o Postgres trataria cada NULL como um valor distinto e
// deixaria o coletor inserir uma linha nova a cada ciclo.
func TestHostSemUnidadeNaoDuplica(t *testing.T) {
	setupInventoryDB(t)

	now := time.Now().UTC()
	for i := 0; i < 2; i++ {
		host := NetworkHost{IP: ipCompartilado, FirstSeen: now, LastSeen: now}
		err := DB.Clauses(clause.OnConflict{
			Columns:   NetworkHostConflictTarget(),
			DoUpdates: clause.Assignments(map[string]any{"last_seen": now}),
		}).Create(&host).Error
		if err != nil {
			t.Fatalf("ciclo %d do host sem unidade: %v", i, err)
		}
	}

	var n int64
	DB.Model(&NetworkHost{}).Where("ip = ? AND site_id IS NULL", ipCompartilado).Count(&n)
	if n != 1 {
		t.Errorf("linhas do host sem unidade = %d, esperado 1", n)
	}
}

// O achado E3: dois DESKTOP-01 em filiais diferentes são duas máquinas. A
// separação depende de a coluna de unidade participar da busca do upsert — o
// que este teste cobre do lado do armazenamento; o caminho HTTP fica com
// findOrCreateAgentServer, em internal/api.
func TestMesmoHostnameEmUnidadesDiferentesCoexiste(t *testing.T) {
	setupInventoryDB(t)

	unidadeA := criaUnidade(t, codigoUnidadeA)
	unidadeB := criaUnidade(t, codigoUnidadeB)

	for _, siteID := range []uint{unidadeA, unidadeB} {
		id := siteID
		srv := Server{Name: hostnameComum, HostIP: "10.0.0.1", Kind: "agent", SiteID: &id}
		if err := DB.Create(&srv).Error; err != nil {
			t.Fatalf("criar servidor da unidade %d: %v", siteID, err)
		}
	}

	var n int64
	DB.Model(&Server{}).Where("name = ?", hostnameComum).Count(&n)
	if n != 2 {
		t.Errorf("servidores gravados = %d, esperado 2 (um por unidade)", n)
	}
}

package database

import (
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
)

type colunaDoInventario struct {
	ColumnName string
	IsNullable string
	DataType   string
}

func bancoComInventario(t *testing.T) *gorm.DB {
	t.Helper()

	db := bancoVazio(t)
	if err := Migrate(db); err != nil {
		t.Fatalf("migrar até o inventário de bancos: %v", err)
	}
	return db
}

func servidorDoInventario(t *testing.T, db *gorm.DB, nome, ip string) string {
	t.Helper()

	s := Server{Name: nome, HostIP: ip, User: "root", Port: 22}
	if err := db.Create(&s).Error; err != nil {
		t.Fatalf("criar servidor: %v", err)
	}
	return s.ID
}

func instanciaDoInventario(t *testing.T, db *gorm.DB, serverID string, porta int) uint {
	t.Helper()

	i := PostgresInstancia{
		ServerID:    serverID,
		Porta:       porta,
		Papel:       "primario",
		Estado:      "ativo",
		ObservadoEm: time.Now().UTC(),
	}
	if err := db.Create(&i).Error; err != nil {
		t.Fatalf("criar instância na porta %d: %v", porta, err)
	}
	return i.ID
}

func baseDoInventario(t *testing.T, db *gorm.DB, instanciaID uint, nome string) uint {
	t.Helper()

	b := PostgresBase{InstanciaID: instanciaID, Nome: nome, ObservadoEm: time.Now().UTC()}
	if err := db.Create(&b).Error; err != nil {
		t.Fatalf("criar base %q: %v", nome, err)
	}
	return b.ID
}

func colunasDe(t *testing.T, db *gorm.DB, tabela string) map[string]colunaDoInventario {
	t.Helper()

	var linhas []colunaDoInventario
	err := db.Raw(`
		SELECT column_name, is_nullable, data_type
		  FROM information_schema.columns
		 WHERE table_schema = 'public' AND table_name = ?`, tabela).Scan(&linhas).Error
	if err != nil {
		t.Fatalf("ler as colunas de %s: %v", tabela, err)
	}

	fora := make(map[string]colunaDoInventario, len(linhas))
	for _, l := range linhas {
		fora[l.ColumnName] = l
	}
	return fora
}

func conferirColunas(t *testing.T, db *gorm.DB, tabela string, esperado map[string][2]string) {
	t.Helper()

	achadas := colunasDe(t, db, tabela)
	if len(achadas) == 0 {
		t.Fatalf("tabela %s não existe depois da migração", tabela)
	}
	for nome, quero := range esperado {
		tem, ok := achadas[nome]
		if !ok {
			t.Errorf("%s.%s não existe", tabela, nome)
			continue
		}
		if tem.DataType != quero[0] {
			t.Errorf("%s.%s é %s, esperado %s", tabela, nome, tem.DataType, quero[0])
		}
		if tem.IsNullable != quero[1] {
			t.Errorf("%s.%s tem is_nullable = %s, esperado %s", tabela, nome, tem.IsNullable, quero[1])
		}
	}
	for nome := range achadas {
		if _, ok := esperado[nome]; !ok {
			t.Errorf("%s tem a coluna inesperada %s", tabela, nome)
		}
	}
}

func TestInventarioDeBancosSobeNaVersao016(t *testing.T) {
	db := bancoComInventario(t)

	if v := versaoAplicada(t, db); v < 16 {
		t.Fatalf("versão aplicada = %d, esperado ao menos 16", v)
	}

	conferirColunas(t, db, "postgres_instancias", map[string][2]string{
		"id":              {"bigint", "NO"},
		"server_id":       {"uuid", "NO"},
		"porta":           {"integer", "NO"},
		"em_container":    {"boolean", "NO"},
		"container_nome":  {"character varying", "NO"},
		"motor":           {"character varying", "NO"},
		"versao":          {"character varying", "NO"},
		"papel":           {"character varying", "NO"},
		"wal_level":       {"character varying", "NO"},
		"max_wal_senders": {"integer", "YES"},
		"archive_mode":    {"character varying", "NO"},
		"estado":          {"character varying", "NO"},
		"motivo":          {"text", "NO"},
		"observado_em":    {"timestamp with time zone", "NO"},
		"created_at":      {"timestamp with time zone", "NO"},
		"updated_at":      {"timestamp with time zone", "NO"},
	})

	conferirColunas(t, db, "postgres_bases", map[string][2]string{
		"id":            {"bigint", "NO"},
		"instancia_id":  {"bigint", "NO"},
		"nome":          {"character varying", "NO"},
		"dono":          {"character varying", "NO"},
		"encoding":      {"character varying", "NO"},
		"tamanho_bytes": {"bigint", "YES"},
		"conexoes":      {"integer", "YES"},
		"observado_em":  {"timestamp with time zone", "NO"},
		"created_at":    {"timestamp with time zone", "NO"},
		"updated_at":    {"timestamp with time zone", "NO"},
	})

	coluna, ok := colunasDe(t, db, "servers")["collect_postgres"]
	if !ok {
		t.Fatal("servers.collect_postgres não existe")
	}
	if coluna.DataType != "boolean" || coluna.IsNullable != "NO" {
		t.Errorf("servers.collect_postgres é %s/%s, esperado boolean/NO", coluna.DataType, coluna.IsNullable)
	}
}

func TestOsStructsApontamParaAsTabelasDaMigracao(t *testing.T) {
	casos := []struct {
		modelo   interface{ TableName() string }
		esperado string
	}{
		{PostgresInstancia{}, "postgres_instancias"},
		{PostgresBase{}, "postgres_bases"},
	}
	for _, caso := range casos {
		if got := caso.modelo.TableName(); got != caso.esperado {
			t.Errorf("TableName = %q, esperado %q", got, caso.esperado)
		}
	}
}

func TestOMesmoServidorNaoRepeteAPorta(t *testing.T) {
	db := bancoComInventario(t)
	primeiro := servidorDoInventario(t, db, "vps-porta", "203.0.113.240")
	segundo := servidorDoInventario(t, db, "vps-porta-2", "203.0.113.241")

	instanciaDoInventario(t, db, primeiro, 5432)

	repetida := PostgresInstancia{ServerID: primeiro, Porta: 5432, Papel: "replica", Estado: "ativo", ObservadoEm: time.Now().UTC()}
	err := db.Create(&repetida).Error
	if err == nil {
		t.Fatal("a mesma porta entrou duas vezes no mesmo servidor")
	}
	if !strings.Contains(err.Error(), "idx_instancia_dono_porta") {
		t.Errorf("a recusa não veio do índice único: %v", err)
	}

	instanciaDoInventario(t, db, segundo, 5432)
}

func TestAMesmaInstanciaNaoRepeteONomeDaBase(t *testing.T) {
	db := bancoComInventario(t)
	servidor := servidorDoInventario(t, db, "vps-base", "203.0.113.242")
	primeira := instanciaDoInventario(t, db, servidor, 5432)
	segunda := instanciaDoInventario(t, db, servidor, 5433)

	baseDoInventario(t, db, primeira, "dockkeeper")

	repetida := PostgresBase{InstanciaID: primeira, Nome: "dockkeeper", ObservadoEm: time.Now().UTC()}
	err := db.Create(&repetida).Error
	if err == nil {
		t.Fatal("a mesma base entrou duas vezes na mesma instância")
	}
	if !strings.Contains(err.Error(), "idx_base_dona_nome") {
		t.Errorf("a recusa não veio do índice único: %v", err)
	}

	baseDoInventario(t, db, segunda, "dockkeeper")
}

func TestApagarServidorLevaInstanciasEBasesJunto(t *testing.T) {
	db := bancoComInventario(t)
	servidor := servidorDoInventario(t, db, "vps-cascata", "203.0.113.243")
	instancia := instanciaDoInventario(t, db, servidor, 5432)
	baseDoInventario(t, db, instancia, "dockkeeper")
	baseDoInventario(t, db, instancia, "postgres")

	if err := db.Unscoped().Where("id = ?", servidor).Delete(&Server{}).Error; err != nil {
		t.Fatalf("apagar o servidor: %v", err)
	}

	var instancias, bases int64
	db.Model(&PostgresInstancia{}).Where("server_id = ?", servidor).Count(&instancias)
	db.Model(&PostgresBase{}).Where("instancia_id = ?", instancia).Count(&bases)
	if instancias != 0 {
		t.Errorf("%d instância(s) sobreviveram ao servidor apagado", instancias)
	}
	if bases != 0 {
		t.Errorf("%d base(s) sobreviveram ao servidor apagado", bases)
	}
}

func TestApagarInstanciaLevaSoAsBasesDela(t *testing.T) {
	db := bancoComInventario(t)
	servidor := servidorDoInventario(t, db, "vps-cascata-2", "203.0.113.244")
	alvo := instanciaDoInventario(t, db, servidor, 5432)
	vizinha := instanciaDoInventario(t, db, servidor, 5433)
	baseDoInventario(t, db, alvo, "dockkeeper")
	baseDoInventario(t, db, vizinha, "dockkeeper")

	if err := db.Where("id = ?", alvo).Delete(&PostgresInstancia{}).Error; err != nil {
		t.Fatalf("apagar a instância: %v", err)
	}

	var daAlvo, daVizinha int64
	db.Model(&PostgresBase{}).Where("instancia_id = ?", alvo).Count(&daAlvo)
	db.Model(&PostgresBase{}).Where("instancia_id = ?", vizinha).Count(&daVizinha)
	if daAlvo != 0 {
		t.Errorf("%d base(s) sobreviveram à instância apagada", daAlvo)
	}
	if daVizinha != 1 {
		t.Errorf("a instância vizinha ficou com %d base(s), esperado 1", daVizinha)
	}
}

func TestPapelEEstadoForaDoConjuntoNaoEntram(t *testing.T) {
	db := bancoComInventario(t)
	servidor := servidorDoInventario(t, db, "vps-check", "203.0.113.245")

	casos := []struct {
		nome       string
		sql        string
		valor      string
		porta      int
		constraint string
	}{
		{"papel", "INSERT INTO postgres_instancias (server_id, porta, papel) VALUES (?, ?, ?)", "mestre", 5432, "chk_instancia_papel"},
		{"estado", "INSERT INTO postgres_instancias (server_id, porta, estado) VALUES (?, ?, ?)", "caiu", 5433, "chk_instancia_estado"},
	}
	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			err := db.Exec(caso.sql, servidor, caso.porta, caso.valor).Error
			if err == nil {
				t.Fatalf("%s = %q entrou", caso.nome, caso.valor)
			}
			if !strings.Contains(err.Error(), caso.constraint) {
				t.Errorf("a recusa não veio de %s: %v", caso.constraint, err)
			}
		})
	}

	valida := PostgresInstancia{
		ServerID:    servidor,
		Porta:       5434,
		Papel:       "desconhecido",
		Estado:      "sem_acesso",
		ObservadoEm: time.Now().UTC(),
	}
	if err := db.Create(&valida).Error; err != nil {
		t.Errorf("valor do conjunto foi recusado: %v", err)
	}
}

func TestBaseSemMedidaVoltaNula(t *testing.T) {
	db := bancoComInventario(t)
	servidor := servidorDoInventario(t, db, "vps-nulo", "203.0.113.246")
	instancia := instanciaDoInventario(t, db, servidor, 5432)
	baseDoInventario(t, db, instancia, "sem_medida")

	var lida PostgresBase
	if err := db.Where("instancia_id = ? AND nome = ?", instancia, "sem_medida").First(&lida).Error; err != nil {
		t.Fatalf("reler a base: %v", err)
	}
	if lida.TamanhoBytes != nil {
		t.Errorf("tamanho_bytes voltou %d; base sem medida tem de voltar nula", *lida.TamanhoBytes)
	}
	if lida.Conexoes != nil {
		t.Errorf("conexoes voltou %d; base sem medida tem de voltar nula", *lida.Conexoes)
	}

	var nulos int64
	db.Raw(`SELECT count(*) FROM postgres_bases WHERE id = ? AND tamanho_bytes IS NULL AND conexoes IS NULL`, lida.ID).Scan(&nulos)
	if nulos != 1 {
		t.Error("o banco gravou 0 no lugar de NULL")
	}

	var instanciaLida PostgresInstancia
	if err := db.Where("id = ?", instancia).First(&instanciaLida).Error; err != nil {
		t.Fatalf("reler a instância: %v", err)
	}
	if instanciaLida.MaxWalSenders != nil {
		t.Errorf("max_wal_senders voltou %d; não medido tem de voltar nulo", *instanciaLida.MaxWalSenders)
	}

	medida := int64(41943040)
	conexoes := 3
	comMedida := PostgresBase{
		InstanciaID:  instancia,
		Nome:         "com_medida",
		TamanhoBytes: &medida,
		Conexoes:     &conexoes,
		ObservadoEm:  time.Now().UTC(),
	}
	if err := db.Create(&comMedida).Error; err != nil {
		t.Fatalf("criar base medida: %v", err)
	}
	var relida PostgresBase
	if err := db.Where("id = ?", comMedida.ID).First(&relida).Error; err != nil {
		t.Fatalf("reler a base medida: %v", err)
	}
	if relida.TamanhoBytes == nil || *relida.TamanhoBytes != medida {
		t.Errorf("tamanho_bytes medido voltou %v, esperado %d", relida.TamanhoBytes, medida)
	}
	if relida.Conexoes == nil || *relida.Conexoes != conexoes {
		t.Errorf("conexoes medidas voltaram %v, esperado %d", relida.Conexoes, conexoes)
	}
}

package database

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

// Teto do pool de conexões. O padrão do database/sql é ilimitado com apenas 2
// ociosas: as goroutines de coleta somadas aos handlers HTTP estouram o
// max_connections do Postgres com "FATAL: sorry, too many clients", e quase
// todo INSERT paga handshake novo porque o excedente é descartado em vez de
// devolvido ao pool. Os valores abaixo cabem folgados no max_connections=100 de
// uma instalação limpa, inclusive com duas réplicas do painel.
const (
	defaultMaxOpenConns    = 20
	defaultMaxIdleConns    = 5
	defaultConnMaxLifetime = 30 * time.Minute
)

func Connect() error {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Println("DATABASE_URL is not set")
	}

	var err error
	DB, err = gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return err
	}

	if err := configurePool(DB); err != nil {
		return err
	}

	err = DB.AutoMigrate(&Server{}, &Container{}, &MetricServer{}, &MetricContainer{}, &MetricLoadBalancer{}, &Domain{}, &AlertRule{}, &LogEntry{}, &Site{}, &NetworkHost{}, &FloorPlan{}, &FloorPlanPin{}, &MetricServerTrend{}, &User{}, &UserSiteAccess{}, &AuditLog{},
		&EnrollmentToken{}, &DeviceCredential{}, &UserSession{}, &AlertState{})
	if err != nil {
		return fmt.Errorf("erro ao migrar as tabelas: %w", err)
	}

	log.Println("[RealTime] Schemas do Banco de Dados criados/atualizados com sucesso!")

	migrateNetworkHostSiteIP()
	migrateServerMachineID()

	return nil
}

// migrateServerMachineID cria a unicidade parcial de (unidade, machine_id).
//
// Parcial porque a coluna é vazia em host cadastrado a mão e em agente antigo
// que ainda não envia o identificador: um índice único comum trataria todos
// esses vazios como o mesmo valor e recusaria o segundo host da unidade.
//
// Roda depois do AutoMigrate, que cria a coluna mas não sabe expressar índice
// com WHERE.
func migrateServerMachineID() {
	const stmt = `CREATE UNIQUE INDEX IF NOT EXISTS idx_servers_site_machine
		ON servers (COALESCE(site_id, 0), machine_id)
		WHERE machine_id <> '' AND deleted_at IS NULL`

	if err := DB.Exec(stmt).Error; err != nil {
		log.Printf("[Migracao] AVISO: indice de machine_id nao criado: %v", err)
	}
}

// configurePool amarra o pool que o GORM abre por baixo. Sem teto observável o
// painel não tem como garantir que cabe no max_connections do servidor.
func configurePool(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("erro ao obter o pool de conexões: %w", err)
	}

	maxOpen := envInt("DB_MAX_OPEN_CONNS", defaultMaxOpenConns)
	maxIdle := envInt("DB_MAX_IDLE_CONNS", defaultMaxIdleConns)

	// O database/sql rebaixa o ocioso ao aberto em silêncio. Avisar é melhor que
	// deixar a configuração dizer uma coisa e o processo fazer outra.
	if maxIdle > maxOpen {
		log.Printf("[Banco] DB_MAX_IDLE_CONNS (%d) é maior que DB_MAX_OPEN_CONNS (%d); usando %d",
			maxIdle, maxOpen, maxOpen)
		maxIdle = maxOpen
	}

	lifetime := envDuration("DB_CONN_MAX_LIFETIME", defaultConnMaxLifetime)

	sqlDB.SetMaxOpenConns(maxOpen)
	sqlDB.SetMaxIdleConns(maxIdle)
	sqlDB.SetConnMaxLifetime(lifetime)

	log.Printf("[Banco] pool: até %d conexões abertas, %d ociosas, vida útil de %s",
		maxOpen, maxIdle, lifetime)
	return nil
}

func envInt(name string, def int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return def
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		log.Printf("[Banco] %s=%q inválido; usando %d", name, raw, def)
		return def
	}
	return v
}

func envDuration(name string, def time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return def
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		log.Printf("[Banco] %s=%q inválido; usando %s", name, raw, def)
		return def
	}
	return d
}

// migrateNetworkHostSiteIP troca a unicidade global do IP pela unicidade dentro
// da unidade.
//
// Faixa RFC1918 sobreposta é o caso normal, não a exceção: 192.168.0.0/24
// existe em toda filial. Sob o índice único global o mesmo 192.168.0.10 de duas
// unidades disputava uma única linha, e cada coletor sobrescrevia hostname, MAC
// e portas do outro a cada ciclo. A trava SiteLocked não protegia contra isso:
// ela congela o site_id, não os demais campos.
//
// Roda depois do AutoMigrate porque o GORM não converte índice único existente
// em não-único — a remoção precisa ser explícita.
func migrateNetworkHostSiteIP() {
	stmts := []string{
		// O nome do índice antigo é escolha do GORM e varia com a versão, então
		// a remoção sai do catálogo em vez de assumir um nome fixo.
		`DO $$
		DECLARE idx text;
		BEGIN
			FOR idx IN
				SELECT i.relname
				FROM pg_index x
				JOIN pg_class i ON i.oid = x.indexrelid
				JOIN pg_class t ON t.oid = x.indrelid
				JOIN pg_attribute a ON a.attrelid = t.oid AND a.attnum = x.indkey[0]
				WHERE t.relname = 'network_hosts'
				  AND x.indisunique
				  AND NOT x.indisprimary
				  AND x.indnkeyatts = 1
				  AND a.attname = 'ip'
			LOOP
				EXECUTE format('DROP INDEX IF EXISTS %I', idx);
			END LOOP;
		END $$`,

		// Na prática não apaga nada: sob o índice único global o Postgres nunca
		// deixou entrar duas linhas com o mesmo IP. Existe para o caso de o
		// índice ter sido removido a mão antes desta migração, porque aí o
		// CREATE UNIQUE abaixo falharia e o inventário ficaria sem chave.
		// Sobrevive a linha vista mais recentemente, que é a que o painel já
		// exibiria.
		`DELETE FROM network_hosts a
		 USING network_hosts b
		 WHERE a.ip = b.ip
		   AND ` + networkHostSiteExpr("a.") + ` = ` + networkHostSiteExpr("b.") + `
		   AND (a.last_seen, a.id) < (b.last_seen, b.id)`,

		`CREATE UNIQUE INDEX IF NOT EXISTS idx_network_hosts_site_ip
		 ON network_hosts (` + networkHostSiteExpr("") + `, ip)`,

		// O índice simples volta porque a busca por IP continua existindo (pino
		// de planta baixa, cadastro do operador) e o composto não a serve: ip
		// não é a primeira coluna.
		`CREATE INDEX IF NOT EXISTS idx_network_hosts_ip ON network_hosts (ip)`,
	}

	for _, s := range stmts {
		if err := DB.Exec(s).Error; err != nil {
			log.Printf("[Migração] erro ao ajustar a unicidade do inventário: %v", err)
			return
		}
	}
}

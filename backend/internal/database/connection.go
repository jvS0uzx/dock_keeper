package database

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/config"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

const (
	defaultStatementTimeout = 15 * time.Second

	defaultMaxOpenConns    = 20
	defaultMaxIdleConns    = 5
	defaultConnMaxLifetime = 30 * time.Minute
)

func From(ctx context.Context) *gorm.DB {
	if DB == nil {
		return nil
	}
	return DB.WithContext(ctx)
}

func statementTimeout() time.Duration {
	return EnvDuration("DB_STATEMENT_TIMEOUT", defaultStatementTimeout)
}

func comStatementTimeout(dsn string) string {
	limite := statementTimeout()
	if limite <= 0 || strings.Contains(dsn, "statement_timeout") {
		return dsn
	}
	ms := strconv.FormatInt(limite.Milliseconds(), 10)

	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		separador := "?"
		if strings.Contains(dsn, "?") {
			separador = "&"
		}
		return dsn + separador + "options=" + url.QueryEscape("-c statement_timeout="+ms)
	}
	if strings.TrimSpace(dsn) == "" {
		return dsn
	}
	return dsn + " statement_timeout=" + ms
}

func Connect() error {
	dsn := comStatementTimeout(os.Getenv("DATABASE_URL"))
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

	if !autoMigrateLigado() {
		return Migrate(DB)
	}

	err = DB.AutoMigrate(&Server{}, &Container{}, &MetricServer{}, &MetricContainer{}, &MetricLoadBalancer{}, &Domain{}, &AlertRule{}, &LogEntry{}, &Site{}, &NetworkHost{}, &FloorPlan{}, &FloorPlanPin{}, &MetricServerTrend{}, &User{}, &UserSiteAccess{}, &AuditLog{},
		&EnrollmentToken{}, &DeviceCredential{}, &UserSession{}, &AlertState{}, &Alert{},
		&Dashboard{}, &DashboardPanel{}, &Annotation{})
	if err != nil {
		return fmt.Errorf("erro ao migrar as tabelas: %w", err)
	}

	log.Println("[RealTime] Schemas do Banco de Dados criados/atualizados com sucesso!")

	migrateNetworkHostSiteIP()
	migrateServerMachineID()

	return nil
}

func autoMigrateLigado() bool {
	raw := strings.TrimSpace(os.Getenv("DB_AUTOMIGRATE"))
	if raw == "" {
		return false
	}
	ligado, err := strconv.ParseBool(raw)
	if err != nil {
		log.Printf("[Banco] DB_AUTOMIGRATE=%q inválido; usando as migrações versionadas", raw)
		return false
	}
	return ligado
}

func migrateServerMachineID() {
	const stmt = `CREATE UNIQUE INDEX IF NOT EXISTS idx_servers_site_machine
		ON servers (COALESCE(site_id, 0), machine_id)
		WHERE machine_id <> '' AND deleted_at IS NULL`

	if err := DB.Exec(stmt).Error; err != nil {
		log.Printf("[Migracao] AVISO: indice de machine_id nao criado: %v", err)
	}
}

func configurePool(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("erro ao obter o pool de conexões: %w", err)
	}

	maxOpen := config.Inteiro("DB_MAX_OPEN_CONNS", defaultMaxOpenConns)
	maxIdle := config.Inteiro("DB_MAX_IDLE_CONNS", defaultMaxIdleConns)

	if maxIdle > maxOpen {
		log.Printf("[Banco] DB_MAX_IDLE_CONNS (%d) é maior que DB_MAX_OPEN_CONNS (%d); usando %d",
			maxIdle, maxOpen, maxOpen)
		maxIdle = maxOpen
	}

	lifetime := config.Duracao("DB_CONN_MAX_LIFETIME", defaultConnMaxLifetime)

	sqlDB.SetMaxOpenConns(maxOpen)
	sqlDB.SetMaxIdleConns(maxIdle)
	sqlDB.SetConnMaxLifetime(lifetime)

	log.Printf("[Banco] pool: até %d conexões abertas, %d ociosas, vida útil de %s",
		maxOpen, maxIdle, lifetime)
	return nil
}

func migrateNetworkHostSiteIP() {
	stmts := []string{
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

		`DELETE FROM network_hosts a
		 USING network_hosts b
		 WHERE a.ip = b.ip
		   AND ` + networkHostSiteExpr("a.") + ` = ` + networkHostSiteExpr("b.") + `
		   AND (a.last_seen, a.id) < (b.last_seen, b.id)`,

		`CREATE UNIQUE INDEX IF NOT EXISTS idx_network_hosts_site_ip
		 ON network_hosts (` + networkHostSiteExpr("") + `, ip)`,

		`CREATE INDEX IF NOT EXISTS idx_network_hosts_ip ON network_hosts (ip)`,
	}

	for _, s := range stmts {
		if err := DB.Exec(s).Error; err != nil {
			log.Printf("[Migração] erro ao ajustar a unicidade do inventário: %v", err)
			return
		}
	}
}

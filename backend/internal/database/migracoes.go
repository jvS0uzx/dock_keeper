package database

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"fmt"
	"log"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/observabilidade"
	"gorm.io/gorm"
)

//go:embed migracoes/*.sql
var arquivosDeMigracao embed.FS

const (
	travaDeMigracao  = 8274523
	tempoDeMigracao  = 5 * time.Minute
	pastaDeMigracoes = "migracoes"
)

type migracao struct {
	versao int
	nome   string
	sql    string
	hash   string
}

type aplicada struct {
	nome string
	hash string
}

func migracoesEmbutidas() ([]migracao, error) {
	entradas, err := arquivosDeMigracao.ReadDir(pastaDeMigracoes)
	if err != nil {
		return nil, fmt.Errorf("ler as migrações embutidas: %w", err)
	}

	var lista []migracao
	for _, e := range entradas {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		partes := strings.SplitN(strings.TrimSuffix(e.Name(), ".sql"), "_", 2)
		if len(partes) != 2 {
			return nil, fmt.Errorf("migração %q fora do padrão NNN_nome.sql", e.Name())
		}
		versao, err := strconv.Atoi(partes[0])
		if err != nil {
			return nil, fmt.Errorf("migração %q tem versão inválida: %w", e.Name(), err)
		}

		conteudo, err := arquivosDeMigracao.ReadFile(path.Join(pastaDeMigracoes, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("ler a migração %q: %w", e.Name(), err)
		}
		soma := sha256.Sum256(conteudo)
		lista = append(lista, migracao{
			versao: versao, nome: partes[1], sql: string(conteudo),
			hash: hex.EncodeToString(soma[:]),
		})
	}

	sort.Slice(lista, func(i, j int) bool { return lista[i].versao < lista[j].versao })
	for i := 1; i < len(lista); i++ {
		if lista[i].versao == lista[i-1].versao {
			return nil, fmt.Errorf("duas migrações com a versão %d", lista[i].versao)
		}
	}
	return lista, nil
}

func Migrate(db *gorm.DB) error {
	lista, err := migracoesEmbutidas()
	if err != nil {
		return err
	}

	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("obter a conexão para migrar: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), tempoDeMigracao)
	defer cancel()

	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return fmt.Errorf("reservar conexão para migrar: %w", err)
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, "SELECT pg_advisory_lock($1)", travaDeMigracao); err != nil {
		return fmt.Errorf("travar a migração: %w", err)
	}
	defer func() {
		if _, err := conn.ExecContext(context.Background(), "SELECT pg_advisory_unlock($1)", travaDeMigracao); err != nil {
			log.Printf("[Migração] aviso: trava não liberada: %v", err)
		}
	}()

	if err := criarControleDeMigracao(ctx, conn); err != nil {
		return err
	}
	jaAplicadas, err := lerAplicadas(ctx, conn)
	if err != nil {
		return err
	}

	if len(lista) > 0 && lista[0].versao == 1 {
		adotado, err := adotarEsquemaExistente(ctx, conn, lista[0], jaAplicadas)
		if err != nil {
			return err
		}
		if adotado {
			jaAplicadas[lista[0].versao] = aplicada{nome: lista[0].nome, hash: lista[0].hash}
		}
	}

	aplicadasAgora := 0
	for _, m := range lista {
		anterior, existe := jaAplicadas[m.versao]
		if existe {
			if anterior.hash != m.hash {
				return fmt.Errorf(
					"migração %03d_%s mudou depois de aplicada (hash %s no banco, %s no código): crie uma migração nova em vez de editar a antiga",
					m.versao, anterior.nome, anterior.hash[:12], m.hash[:12])
			}
			continue
		}
		if err := aplicar(ctx, conn, m); err != nil {
			return err
		}
		aplicadasAgora++
		observabilidade.MigracoesAplicadas.Add(1)
	}

	versao := 0
	if len(lista) > 0 {
		versao = lista[len(lista)-1].versao
	}
	if aplicadasAgora > 0 {
		log.Printf("[Migração] %d migração(ões) aplicada(s); banco na versão %d", aplicadasAgora, versao)
	} else {
		log.Printf("[Migração] banco já estava na versão %d", versao)
	}
	return nil
}

func adotarEsquemaExistente(ctx context.Context, conn *sql.Conn, baseline migracao, jaAplicadas map[int]aplicada) (bool, error) {
	if _, existe := jaAplicadas[baseline.versao]; existe {
		return false, nil
	}

	var tabela *string
	if err := conn.QueryRowContext(ctx, "SELECT to_regclass('public.servers')::text").Scan(&tabela); err != nil {
		return false, fmt.Errorf("procurar esquema existente: %w", err)
	}
	if tabela == nil {
		return false, nil
	}

	if _, err := conn.ExecContext(ctx,
		"INSERT INTO schema_migrations (versao, nome, hash) VALUES ($1, $2, $3)",
		baseline.versao, baseline.nome, baseline.hash); err != nil {
		return false, fmt.Errorf("adotar o esquema existente como baseline: %w", err)
	}

	log.Printf("[Migração] banco já tinha as tabelas: %03d_%s adotada como baseline, sem recriar nada",
		baseline.versao, baseline.nome)
	return true, nil
}

func criarControleDeMigracao(ctx context.Context, conn *sql.Conn) error {
	const ddl = `CREATE TABLE IF NOT EXISTS schema_migrations (
		versao integer PRIMARY KEY,
		nome text NOT NULL,
		hash text NOT NULL,
		aplicada_em timestamp with time zone NOT NULL DEFAULT now()
	)`

	if _, err := conn.ExecContext(ctx, ddl); err != nil {
		return fmt.Errorf("criar schema_migrations: %w", err)
	}
	return nil
}

func lerAplicadas(ctx context.Context, conn *sql.Conn) (map[int]aplicada, error) {
	linhas, err := conn.QueryContext(ctx, "SELECT versao, nome, hash FROM schema_migrations")
	if err != nil {
		return nil, fmt.Errorf("ler schema_migrations: %w", err)
	}
	defer linhas.Close()

	fora := map[int]aplicada{}
	for linhas.Next() {
		var versao int
		var nome, hash string
		if err := linhas.Scan(&versao, &nome, &hash); err != nil {
			return nil, fmt.Errorf("ler schema_migrations: %w", err)
		}
		fora[versao] = aplicada{nome: nome, hash: hash}
	}
	return fora, linhas.Err()
}

func aplicar(ctx context.Context, conn *sql.Conn, m migracao) error {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("abrir transação da migração %03d: %w", m.versao, err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, m.sql); err != nil {
		return fmt.Errorf("migração %03d_%s falhou: %w", m.versao, m.nome, err)
	}
	if _, err := tx.ExecContext(ctx,
		"INSERT INTO schema_migrations (versao, nome, hash) VALUES ($1, $2, $3)",
		m.versao, m.nome, m.hash); err != nil {
		return fmt.Errorf("registrar a migração %03d_%s: %w", m.versao, m.nome, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("confirmar a migração %03d_%s: %w", m.versao, m.nome, err)
	}

	log.Printf("[Migração] %03d_%s aplicada", m.versao, m.nome)
	registrarLimpeza(ctx, conn, m.versao)
	return nil
}

func registrarLimpeza(ctx context.Context, conn *sql.Conn, versao int) {
	linhas, err := conn.QueryContext(ctx,
		"SELECT tabela, motivo, linhas FROM migracao_limpeza WHERE versao = $1 AND linhas > 0", versao)
	if err != nil {
		return
	}
	defer linhas.Close()

	for linhas.Next() {
		var tabela, motivo string
		var n int64
		if err := linhas.Scan(&tabela, &motivo, &n); err != nil {
			return
		}
		log.Printf("[Migração] %03d limpou %d linha(s) em %s: %s", versao, n, tabela, motivo)
	}
}

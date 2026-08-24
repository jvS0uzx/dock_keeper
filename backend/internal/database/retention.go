package database

import (
	"log"
	"time"
)

// StartRetentionWorker apaga métricas mais antigas que maxAge em intervalos fixos.
// Sem isso as tabelas metric_* crescem sem limite (inserção a cada 1-2s por servidor)
// e o Postgres incha até degradar as queries do painel.
//
// trendsReady é o sinal do StartTrendWorker. A primeira poda espera por ele:
// no boot as duas rotinas disparam juntas, e se o painel passou mais tempo fora
// do ar que a janela de retenção, podar antes de agregar apagaria dado bruto que
// o rollup ainda não consolidou — o histórico daquele período some para sempre.
// Passar nil desliga a espera (usado nos testes).
func StartRetentionWorker(maxAge, interval time.Duration, trendsReady <-chan struct{}) {
	// Lido uma vez, no boot, e não a cada passada: a retenção da auditoria não
	// muda com o processo de pé, e reler o ambiente a cada ciclo só criaria a
	// possibilidade de duas passadas usarem prazos diferentes.
	auditMaxAge := time.Duration(envInt("AUDIT_RETENTION_DAYS", defaultAuditRetentionDays)) * 24 * time.Hour

	go func() {
		if trendsReady != nil {
			<-trendsReady
		}
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			prune(maxAge, auditMaxAge)
			<-ticker.C
		}
	}()
}

// Um ano. Prazo longo de propósito: a auditoria é consultada depois do
// incidente, e o incidente costuma ser descoberto muito depois de acontecer.
const defaultAuditRetentionDays = 365

// Tamanho do lote da poda. Um DELETE sem limite numa tabela que recebe uma
// inserção por segundo por servidor segura lock por minutos e gera todo o bloat
// de uma vez; em lotes o autovacuum acompanha e o painel continua respondendo.
const pruneBatchSize = 5000

// Pausa entre lotes, para a poda não monopolizar o pool de conexões nem o WAL.
const pruneBatchPause = 100 * time.Millisecond

// Teto de lotes por tabela em cada passada. A poda roda de novo no próximo
// ciclo: convergir em algumas passadas é melhor que prender a rotina por horas
// na primeira execução sobre uma base antiga.
const pruneMaxBatches = 200

// execFunc isola o acesso ao banco para o laço de lotes poder ser testado sem
// Postgres. Devolve o número de linhas afetadas pela execução.
type execFunc func(sql string, args ...any) (int64, error)

func dbExec(sql string, args ...any) (int64, error) {
	res := DB.Exec(sql, args...)
	return res.RowsAffected, res.Error
}

func prune(maxAge, auditMaxAge time.Duration) {
	cutoff := time.Now().UTC().Add(-maxAge)
	for _, table := range []string{"metric_servers", "metric_containers", "metric_load_balancers"} {
		n, err := pruneBatched(dbExec, table, "timestamp", cutoff, time.Sleep)
		if err != nil {
			log.Printf("[Retention] erro ao podar %s: %v", table, err)
			continue
		}
		if n > 0 {
			log.Printf("[Retention] %s: %d linhas antigas removidas", table, n)
		}
	}
	pruneContainers(cutoff)
	pruneAuditLog(auditMaxAge)
}

// pruneAuditLog poda o log de auditoria com prazo PRÓPRIO, muito mais longo que
// o das métricas.
//
// São dados de naturezas opostas: métrica de duas semanas atrás não responde
// nenhuma pergunta, enquanto auditoria antiga é justamente o que se consulta
// depois de um incidente — e o incidente costuma ser descoberto meses depois de
// acontecer. Aplicar a retenção de métrica aqui apagaria a evidência antes de
// alguém saber que precisava dela.
func pruneAuditLog(maxAge time.Duration) {
	cutoff := time.Now().UTC().Add(-maxAge)
	n, err := pruneBatched(dbExec, "audit_logs", "at", cutoff, time.Sleep)
	if err != nil {
		log.Printf("[Retention] erro ao podar audit_logs: %v", err)
		return
	}
	if n > 0 {
		log.Printf("[Retention] audit_logs: %d linhas antigas removidas", n)
	}
}

// PruneOlderThan poda uma tabela em lotes, com pausa entre eles.
//
// Exportada porque internal/logstore precisa da mesma poda para log_entries, que
// é a tabela de maior volume do sistema — recebe uma linha por linha de log de
// container e de auth.log. Ela ficou de fora quando o DELETE sem LIMIT foi
// corrigido nas tabelas de métrica, e um DELETE sem limite ali trava a tabela
// justamente no momento em que mais se escreve nela.
//
// table e timeColumn são interpolados no SQL: só passe literais do código, nunca
// entrada de usuário.
func PruneOlderThan(table, timeColumn string, cutoff time.Time) (int64, error) {
	return pruneBatched(dbExec, table, timeColumn, cutoff, time.Sleep)
}

// pruneBatchSQL apaga um lote de cada vez. O subselect com LIMIT existe porque
// o DELETE do Postgres não aceita LIMIT direto.
//
// A coluna de tempo é parâmetro porque as tabelas de métrica a chamam de
// "timestamp" e o log de auditoria a chama de "at". Interpolar o nome é seguro
// aqui: os dois valores são literais deste arquivo, nunca entrada de usuário.
func pruneBatchSQL(table, timeColumn string) string {
	return "DELETE FROM " + table +
		" WHERE id IN (SELECT id FROM " + table +
		" WHERE " + timeColumn + " < ? ORDER BY id LIMIT ?)"
}

// pruneBatched repete o lote até a tabela não devolver mais nada, ou até o teto
// de lotes. sleep é parâmetro para o teste não esperar de verdade.
func pruneBatched(exec execFunc, table, timeColumn string, cutoff time.Time, sleep func(time.Duration)) (int64, error) {
	sql := pruneBatchSQL(table, timeColumn)

	var total int64
	for i := 0; i < pruneMaxBatches; i++ {
		n, err := exec(sql, cutoff, pruneBatchSize)
		if err != nil {
			return total, err
		}
		total += n

		// Lote incompleto significa que acabou o que havia para apagar.
		if n < pruneBatchSize {
			return total, nil
		}
		sleep(pruneBatchPause)
	}

	log.Printf("[Retention] %s: teto de %d lotes atingido, o restante sai no próximo ciclo",
		table, pruneMaxBatches)
	return total, nil
}

// pruneContainers remove o cadastro de container que parou de reportar.
//
// A tela já esconde esses containers (filtra por métrica recente), mas a linha
// em `containers` ficava para sempre — num ambiente que recria container a cada
// deploy, a tabela só cresce.
//
// Roda depois do prune das métricas, na mesma transação lógica: as linhas de
// metric_containers do container morto já foram embora, então o NOT EXISTS
// decide sobre uma tabela já limpa.
func pruneContainers(cutoff time.Time) {
	// O corte por created_at protege o container recém-criado que ainda não
	// gerou métrica: sem ele, o container nasceria e seria apagado na janela
	// entre o cadastro e a primeira amostra.
	res := DB.Exec(`
		DELETE FROM containers c
		WHERE c.created_at < ?
		  AND NOT EXISTS (
		      SELECT 1 FROM metric_containers m
		      WHERE m.container_id = c.id AND m.timestamp >= ?
		  )
	`, cutoff, cutoff)
	if res.Error != nil {
		log.Printf("[Retention] erro ao podar containers: %v", res.Error)
		return
	}
	if res.RowsAffected > 0 {
		log.Printf("[Retention] containers: %d cadastros sem métrica removidos", res.RowsAffected)
	}

	// Métrica cujo container sumiu (apagado a mão, ou por uma versão anterior
	// desta rotina) não aparece em tela nenhuma e nunca seria coletada de novo.
	res = DB.Exec(`
		DELETE FROM metric_containers m
		WHERE NOT EXISTS (SELECT 1 FROM containers c WHERE c.id = m.container_id)
	`)
	if res.Error != nil {
		log.Printf("[Retention] erro ao podar métricas órfãs: %v", res.Error)
		return
	}
	if res.RowsAffected > 0 {
		log.Printf("[Retention] metric_containers: %d métricas órfãs removidas", res.RowsAffected)
	}
}

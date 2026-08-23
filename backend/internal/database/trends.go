package database

import (
	"fmt"
	"log"
	"sync"
	"time"
)

// Retenção da trend. Muito maior que a do dado bruto (7 dias) porque é ela que
// sustenta o gráfico histórico: 24 linhas por dia por host ocupam pouco.
// Prazo da tendência agregada. Parametrizável porque é a primeira coisa que um
// adotante ajusta: quem só quer 90 dias não deve precisar recompilar.
var trendRetention = RetentionDays("TREND_RETENTION_DAYS", DefaultTrendRetentionDays)

// Janela do rollup incremental. Sem limite inferior a rotina varria
// metric_servers inteira a cada 15 minutos e reescrevia todos os baldes de
// todos os hosts via ON CONFLICT DO UPDATE — custo crescendo com o histórico,
// para reconsolidar horas que não mudaram. Três horas cobrem com folga a hora
// recém-fechada e a coleta que chegou atrasada.
const trendRollupWindow = 3 * time.Hour

// StartTrendWorker agrega o histórico bruto em médias horárias.
//
// Sem isso, um gráfico de 30 dias varre milhões de linhas de metric_servers a
// cada abertura de tela.
//
// Devolve um canal fechado assim que o primeiro rollup termina. A poda espera
// por ele: enquanto o bruto não virou trend, apagá-lo destrói histórico que
// ninguém mais consegue reconstruir.
func StartTrendWorker(interval time.Duration) <-chan struct{} {
	ready := make(chan struct{})
	var once sync.Once

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		// A primeira passada varre tudo, sem janela. Se o painel ficou fora do ar
		// por mais tempo que trendRollupWindow, o bruto daquele período ainda não
		// virou trend — e a poda, que espera justamente este sinal, o apagaria em
		// seguida.
		full := true
		for {
			rollupTrends(rollupSince(full))
			full = false

			// Fecha depois da PRIMEIRA passada, com ou sem erro. Um banco
			// indisponível não pode travar a poda para sempre: tabela crescendo
			// sem limite derruba o painel inteiro, enquanto o risco que o sinal
			// evita se restringe ao histórico de quem ficou dias fora do ar.
			once.Do(func() { close(ready) })

			pruneTrends()
			<-ticker.C
		}
	}()

	return ready
}

// rollupSince devolve o limite inferior da agregação: nulo na varredura
// completa do boot, a janela incremental nas passadas seguintes.
func rollupSince(full bool) *time.Time {
	if full {
		return nil
	}
	since := time.Now().UTC().Add(-trendRollupWindow)
	return &since
}

// rollupSQL monta a agregação. O WHERE é construído a partir de constantes; o
// único valor variável entra por placeholder.
func rollupSQL(since *time.Time) (string, []any) {
	where := "timestamp < date_trunc('hour', NOW())"
	args := []any{}
	if since != nil {
		where += " AND timestamp >= ?"
		args = append(args, *since)
	}

	return fmt.Sprintf(`
		INSERT INTO metric_server_trends (
			server_id, bucket,
			cpu_avg, cpu_max,
			mem_percent_avg, disk_percent_avg,
			load_avg1_avg, load_avg1_max,
			temperature_avg, temperature_max,
			samples
		)
		SELECT
			server_id,
			date_trunc('hour', timestamp) AS bucket,
			AVG(cpu_usage_percent), MAX(cpu_usage_percent),
			AVG(mem_used_bytes::float8  / NULLIF(mem_total_bytes, 0)  * 100),
			AVG(disk_used_bytes::float8 / NULLIF(disk_total_bytes, 0) * 100),
			AVG(load_avg1), MAX(load_avg1),
			AVG(NULLIF(temperature_c, 0)), MAX(temperature_c),
			COUNT(*)
		FROM metric_servers
		WHERE %s
		GROUP BY server_id, bucket
		ON CONFLICT (server_id, bucket) DO UPDATE SET
			cpu_avg          = EXCLUDED.cpu_avg,
			cpu_max          = EXCLUDED.cpu_max,
			mem_percent_avg  = EXCLUDED.mem_percent_avg,
			disk_percent_avg = EXCLUDED.disk_percent_avg,
			load_avg1_avg    = EXCLUDED.load_avg1_avg,
			load_avg1_max    = EXCLUDED.load_avg1_max,
			temperature_avg  = EXCLUDED.temperature_avg,
			temperature_max  = EXCLUDED.temperature_max,
			samples          = EXCLUDED.samples
	`, where), args
}

// rollupTrends consolida as horas já fechadas.
//
// A hora corrente fica de fora: agregá-la gravaria uma média parcial que seria
// substituída no ciclo seguinte. O ON CONFLICT existe para o caso de a coleta
// atrasar e completar uma hora já agregada.
func rollupTrends(since *time.Time) {
	sql, args := rollupSQL(since)

	res := DB.Exec(sql, args...)
	if res.Error != nil {
		log.Printf("[Trends] erro ao agregar: %v", res.Error)
		return
	}
	if res.RowsAffected > 0 {
		log.Printf("[Trends] %d baldes horários consolidados", res.RowsAffected)
	}
}

func pruneTrends() {
	cutoff := time.Now().UTC().Add(-trendRetention)
	res := DB.Where("bucket < ?", cutoff).Delete(&MetricServerTrend{})
	if res.Error != nil {
		log.Printf("[Trends] erro ao podar: %v", res.Error)
		return
	}
	if res.RowsAffected > 0 {
		log.Printf("[Trends] %d baldes antigos removidos", res.RowsAffected)
	}
}

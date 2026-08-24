package rules

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"gorm.io/gorm/clause"

	"github.com/joaov/vd_stats/internal/alert"
	"github.com/joaov/vd_stats/internal/database"
)

// metricValue calcula o valor da métrica a partir da última leitura do host.
// Retorna (valor, ok=false) quando não há dado suficiente (ex: total zerado).
func metricValue(m database.MetricServer, metric string) (float64, bool) {
	switch metric {
	case "cpu":
		return m.CPUUsagePercent, true
	case "load":
		return m.LoadAvg1, true
	case "mem":
		if m.MemTotalBytes > 0 {
			return float64(m.MemUsedBytes) / float64(m.MemTotalBytes) * 100, true
		}
	case "disk":
		if m.DiskTotalBytes > 0 {
			return float64(m.DiskUsedBytes) / float64(m.DiskTotalBytes) * 100, true
		}
	}
	return 0, false
}

// resolveTargets expande o alvo da regra na lista de servidores que ela deve
// avaliar. É função pura de propósito: a escolha do alvo é a parte da regra
// com mais casos de borda, e testá-la não deveria exigir banco.
//
// Precedência: unidade vence "*", porque uma regra por unidade só chega aqui
// com Target="*" (o handler força isso para o campo não ficar ambíguo).
func resolveTargets(rule database.AlertRule, servers []database.Server) []string {
	if rule.TargetSiteID != nil {
		// Unidade sem servidor devolve lista vazia, não todos: uma filial que
		// ainda não tem máquina cadastrada não pode disparar alerta do parque
		// inteiro.
		targets := make([]string, 0)
		for _, s := range servers {
			if s.SiteID != nil && *s.SiteID == *rule.TargetSiteID {
				targets = append(targets, s.ID)
			}
		}
		return targets
	}

	if rule.Target == "*" {
		targets := make([]string, 0, len(servers))
		for _, s := range servers {
			targets = append(targets, s.ID)
		}
		return targets
	}

	return []string{rule.Target}
}

func violates(value float64, operator string, threshold float64) bool {
	switch operator {
	case ">":
		return value > threshold
	case "<":
		return value < threshold
	}
	return false
}

// minNotifySeverity lê o piso de notificação do ambiente. Padrão warning:
// info fica só no log.
func minNotifySeverity() string {
	if s := strings.ToLower(strings.TrimSpace(os.Getenv("ALERT_MIN_SEVERITY"))); ValidSeverity(s) {
		return s
	}
	return SeverityWarning
}

// Teto da varredura no banco. O lookback não é a janela: ele só limita o quanto
// a consulta lê, e a janela de cada host é aplicada depois, em recentMetrics,
// por database.LiveWindowFor.
const metricLookback = "10 minutes"

// recentMetrics reduz a última métrica de cada host ao que ainda cabe na janela
// daquele host. Estar no mapa é o que o motor trata como "este host está
// reportando": vale tanto para avaliar a regra quanto para decidir se a
// dependência dela está no ar.
//
// Host que não está mais em servers fica de fora: regra apontada para máquina
// removida não dispara com métrica órfã.
func recentMetrics(latest []database.MetricServer, servers []database.Server, now time.Time) map[string]database.MetricServer {
	byServer := make(map[string]database.MetricServer, len(latest))
	for _, m := range latest {
		byServer[m.ServerID] = m
	}

	recent := make(map[string]database.MetricServer, len(byServer))
	for _, s := range servers {
		m, ok := byServer[s.ID]
		if !ok {
			continue
		}
		if now.Sub(m.Timestamp) <= database.LiveWindowFor(s.ReportIntervalSec) {
			recent[s.ID] = m
		}
	}
	return recent
}

// tickInterval é o intervalo entre avaliações, guardado por StartEngine.
//
// Serve para decidir se duas violações consecutivas pertencem à mesma sequência
// ou se houve um buraco entre elas — ver breachGap. O padrão só vale para quem
// chama evaluate sem passar por StartEngine, que hoje são os testes.
var tickInterval = 30 * time.Second

// breachGap é o intervalo máximo entre duas avaliações para a violação contar
// como ininterrupta.
//
// Dois ticks de tolerância: um tick atrasado ou perdido não pode zerar uma
// contagem de cinco minutos, mas dois seguidos significam que o painel não tem
// evidência do que aconteceu no meio — e "acima de X por 5 minutos" só pode ser
// afirmado sobre tempo observado.
func breachGap() time.Duration {
	if gap := 2 * tickInterval; gap > 90*time.Second {
		return gap
	}
	return 90 * time.Second
}

// stateKey é a chave de AlertState. Igual à chave de cooldown do pacote alert de
// propósito: as duas metades da linha descrevem o mesmo par regra/alvo.
func stateKey(ruleID uint, serverID string) string {
	return fmt.Sprintf("rule:%d:%s", ruleID, serverID)
}

// loadAlertStates carrega o estado gravado de uma regra numa consulta só.
//
// Por rule_id, que é indexado, em vez de um IN com a lista de alvos: uma regra
// com alvo "*" num parque grande geraria um IN de centenas de elementos a cada
// tick para ler as mesmas linhas.
//
// Erro devolve mapa vazio. A consequência é deliberada e assimétrica: sem estado
// a contagem de duração recomeça, então regra COM duração segura o disparo até o
// banco voltar, regra SEM duração continua alertando normalmente, e nenhuma
// recuperação é anunciada. Nunca tranquilizar sem evidência.
func loadAlertStates(ruleID uint) map[string]database.AlertState {
	states := make(map[string]database.AlertState)
	if database.DB == nil {
		return states
	}

	var rows []database.AlertState
	if err := database.DB.Where("rule_id = ?", ruleID).Find(&rows).Error; err != nil {
		log.Printf("[rules] erro ao carregar estado da regra %d: %v", ruleID, err)
		return states
	}
	for _, r := range rows {
		states[r.Key] = r
	}
	return states
}

// flushBreaches grava o estado de todos os alvos violando numa escrita só.
//
// Em lote porque uma regra com alvo "*" e centenas de hosts violando faria uma
// instrução por host a cada tick. São linhas diferentes, então não há a
// contenção de lock que existia em alert_rules, mas continua sendo tráfego
// desnecessário.
//
// last_notified_at fica fora do DoUpdates: aquela coluna pertence ao pacote
// alert, que sabe quando o operador foi avisado. Aqui só se escreve o que o
// motor observou.
func flushBreaches(rows []database.AlertState) {
	if len(rows) == 0 || database.DB == nil {
		return
	}

	err := database.DB.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"rule_id", "server_id", "severity",
			"first_breach_at", "last_breach_at", "active", "updated_at",
		}),
	}).Create(&rows).Error
	if err != nil {
		log.Printf("[rules] erro ao gravar estado de %d alvo(s): %v", len(rows), err)
	}
}

// flushSettled encerra a sequência de violação dos alvos que voltaram ao limite.
//
// Zera first_breach_at e last_breach_at, e não só baixa a bandeira: é o
// zeramento explícito que faz uma amostra dentro do limite recomeçar a contagem
// de duração. Sem ele, "acima de X por 5 minutos" aceitaria cinco minutos
// somados ao longo do dia, porque a violação seguinte ainda encontraria o
// last_breach_at antigo dentro da tolerância de breachGap.
//
// Só é chamado para alvos que TÊM estado gravado. Escrever "não está violando"
// para todo host em toda regra a cada tick seria uma escrita por host por tick
// para registrar que nada aconteceu.
func flushSettled(keys []string, at time.Time) {
	if len(keys) == 0 || database.DB == nil {
		return
	}

	err := database.DB.Model(&database.AlertState{}).
		Where("key IN ?", keys).
		Updates(map[string]any{
			"active":          false,
			"first_breach_at": time.Time{},
			"last_breach_at":  time.Time{},
			"updated_at":      at,
		}).Error
	if err != nil {
		log.Printf("[rules] erro ao encerrar a violação de %d alvo(s): %v", len(keys), err)
	}
}

// breachStart decide quando começou a sequência de violação em curso.
//
// É o coração da regra de duração, e por isso é função pura: a decisão depende
// de uma série temporal, e testá-la não deveria exigir banco.
//
// Duas coisas quebram a sequência, e cada uma cobre um buraco diferente:
//   - uma amostra dentro do limite, que zera o estado explicitamente em
//     flushSettled — é o caso comum, o host oscilando em volta do limiar;
//   - um intervalo maior que breachGap desde a última violação observada, que é
//     o caso do painel fora do ar. Nesse período nenhuma amostra foi avaliada,
//     então não existe zeramento explícito para ler, e sem esta guarda um
//     reinício depois de uma hora fora dispararia na primeira avaliação como se
//     tivesse observado a hora inteira.
func breachStart(state database.AlertState, hasState bool, now time.Time) time.Time {
	if !hasState || state.FirstBreachAt.IsZero() {
		return now
	}
	if now.Sub(state.LastBreachAt) > breachGap() {
		return now
	}
	return state.FirstBreachAt
}

// StartEngine sobe uma goroutine que avalia as regras de alerta a cada tick.
func StartEngine(interval time.Duration) {
	if interval > 0 {
		tickInterval = interval
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			evaluate()
		}
	}()
}

func evaluate() {
	var rules []database.AlertRule
	if err := database.DB.Where("enabled = ?", true).Find(&rules).Error; err != nil {
		log.Printf("[rules] erro ao carregar regras: %v", err)
		return
	}
	if len(rules) == 0 {
		return
	}

	// nomes dos servers para mensagem legível.
	var servers []database.Server
	if err := database.DB.Find(&servers).Error; err != nil {
		log.Printf("[rules] erro ao carregar servidores: %v", err)
		return
	}
	nameByID := make(map[string]string, len(servers))
	for _, s := range servers {
		nameByID[s.ID] = s.Name
	}

	// Nomes das unidades numa consulta só. A regra por unidade cita a filial na
	// mensagem: sem isso o plantão recebe "CPU alta em PC-RH" sem saber de
	// qual das filiais o PC-RH é.
	var sites []database.Site
	if err := database.DB.Find(&sites).Error; err != nil {
		log.Printf("[rules] erro ao carregar unidades: %v", err)
		return
	}
	siteNameByID := make(map[uint]string, len(sites))
	for _, s := range sites {
		siteNameByID[s.ID] = s.Name
	}

	// Última métrica de cada server dentro do lookback. Quem decide o que é
	// recente é recentMetrics, host a host.
	var latest []database.MetricServer
	if err := database.DB.Raw(`
		SELECT DISTINCT ON (server_id) *
		FROM metric_servers
		WHERE timestamp >= NOW() - INTERVAL '` + metricLookback + `'
		ORDER BY server_id, timestamp DESC
	`).Scan(&latest).Error; err != nil {
		log.Printf("[rules] erro ao carregar métricas recentes: %v", err)
		return
	}

	now := time.Now()
	metricByServer := recentMetrics(latest, servers, now)

	// Severidade mínima que gera notificação. Abaixo disso o alerta fica só no
	// log — evita que regra informativa acorde alguém de madrugada.
	minSeverity := Rank(minNotifySeverity())

	for _, rule := range rules {
		// Dependência: se o host do qual esta regra depende está fora, o
		// problema é lá. Alertar as máquinas atrás dele só multiplica ruído.
		if rule.DependsOnServerID != nil {
			if _, parentUp := metricByServer[*rule.DependsOnServerID]; !parentUp {
				continue
			}
		}

		targets := resolveTargets(rule, servers)
		states := loadAlertStates(rule.ID)
		required := time.Duration(rule.ForDurationSec) * time.Second

		var breaches []database.AlertState
		var settled []string
		fired := false

		for _, serverID := range targets {
			m, ok := metricByServer[serverID]
			if !ok {
				// Sem métrica recente não se sabe se violou nem se recuperou.
				// O estado fica como está: anunciar recuperação de um host que
				// apenas parou de reportar seria tranquilizar sem evidência, e
				// host calado é assunto de outra regra.
				continue
			}
			value, ok := metricValue(m, rule.Metric)
			if !ok {
				continue
			}

			key := stateKey(rule.ID, serverID)
			state, hasState := states[key]
			serverName := displayName(rule, serverID, nameByID, siteNameByID)

			if !violates(value, rule.Operator, rule.Threshold) {
				// A recuperação só é anunciada se o problema chegou a ser
				// anunciado. Active espelha "o operador foi avisado", e não
				// "existe problema" — e é isso que contém o flapping: como o
				// alerta passa pelo cooldown, o par alerta/recuperação não
				// acontece mais de uma vez por janela de cooldown, por mais que
				// o host oscile em volta do limite.
				if hasState && state.Active {
					alert.Send(fmt.Sprintf(
						"%s Recuperado - Regra %s: %s=%.2f voltou ao limite (%s %.2f) em %s",
						Prefix(SeverityInfo), rule.Name, rule.Metric, value,
						rule.Operator, rule.Threshold, serverName,
					))
				}
				// Zera a contagem mesmo sem ter havido anúncio: a sequência de
				// violação terminou aqui, e é isso que a duração mede.
				if hasState && (state.Active || !state.FirstBreachAt.IsZero()) {
					settled = append(settled, key)
				}
				continue
			}

			// Violando: a regra só dispara depois de ForDurationSec de violação
			// ininterrupta. Zero mantém o comportamento antigo, de disparar na
			// primeira amostra, que é o valor de toda regra já cadastrada.
			firstBreach := breachStart(state, hasState, now)

			active := hasState && state.Active
			if now.Sub(firstBreach) >= required {
				msg := fmt.Sprintf(
					"%s Regra %s: %s=%.2f %s %.2f em %s",
					Prefix(rule.Severity), rule.Name, rule.Metric, value, rule.Operator, rule.Threshold, serverName,
				)
				if Rank(rule.Severity) >= minSeverity {
					// Só o aviso que de fato saiu levanta a bandeira. Alerta
					// engolido pelo cooldown não pode prometer uma recuperação
					// que o operador não vai entender.
					if alert.Notify(key, msg) {
						active = true
					}
				} else {
					log.Printf("[rules] (abaixo do mínimo notificável) %s", msg)
				}
				fired = true
			}

			breaches = append(breaches, database.AlertState{
				Key:           key,
				RuleID:        rule.ID,
				ServerID:      serverID,
				Severity:      rule.Severity,
				FirstBreachAt: firstBreach,
				LastBreachAt:  now,
				Active:        active,
				UpdatedAt:     now,
			})
		}

		flushBreaches(breaches)
		flushSettled(settled, now)

		// Uma escrita por regra por tick, não uma por alvo. Uma regra com alvo
		// "*" e 500 hosts violando fazia 500 UPDATE na mesma linha a cada tick:
		// contenção de lock e bloat em alert_rules, para gravar o mesmo valor
		// 500 vezes. last_fired marca "esta regra disparou agora" e não é lido
		// por nada além do painel — o cooldown por host vive em alert.Notify,
		// na chave rule:<id>:<server_id>, e não muda com isto.
		if fired {
			at := now
			if err := database.DB.Model(&database.AlertRule{}).
				Where("id = ?", rule.ID).
				Update("last_fired", &at).Error; err != nil {
				log.Printf("[rules] erro ao marcar disparo da regra %d: %v", rule.ID, err)
			}
		}
	}
}

// displayName é o nome do host como aparece na mensagem.
//
// Regra por unidade acrescenta a filial: sem isso o plantão recebe "CPU alta em
// PC-RH" sem saber de qual das filiais o PC-RH é.
func displayName(rule database.AlertRule, serverID string, nameByID map[string]string, siteNameByID map[uint]string) string {
	name := nameByID[serverID]
	if name == "" {
		name = serverID
	}
	if rule.TargetSiteID != nil {
		if siteName := siteNameByID[*rule.TargetSiteID]; siteName != "" {
			name = fmt.Sprintf("%s (%s)", name, siteName)
		}
	}
	return name
}

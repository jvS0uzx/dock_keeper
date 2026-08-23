package database

import "time"

// MinLiveWindow é o piso da janela: quanto tempo sem métrica um host ainda conta
// como reportando quando o intervalo dele é desconhecido — coleta por SSH, ou
// agente antigo que não informa o próprio intervalo.
const MinLiveWindow = 30 * time.Second

// LiveWindowFor devolve por quanto tempo a última métrica de um host ainda vale,
// derivada do intervalo que o próprio agente informou: três ciclos de
// tolerância, para um atraso pontual não derrubar o host.
//
// Mora no pacote do banco, junto de Server.ReportIntervalSec, porque é a mesma
// definição de "recente" para dois consumidores que não podem depender um do
// outro: o painel, que decide se a máquina aparece online, e o motor de regras,
// que decide se a métrica é fresca o bastante para avaliar. Enquanto a definição
// estava duplicada nos dois, a janela fixa de 60 s do motor fazia agente com
// AGENT_INTERVAL=120 nunca disparar regra nenhuma, e o sintoma era silêncio.
func LiveWindowFor(intervalSec int) time.Duration {
	if intervalSec <= 0 {
		return MinLiveWindow
	}
	if w := time.Duration(intervalSec) * 3 * time.Second; w > MinLiveWindow {
		return w
	}
	return MinLiveWindow
}

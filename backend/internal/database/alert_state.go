package database

import "time"

// AlertState guarda o estado de cada alerta entre um tick e o próximo.
//
// Antes o cooldown de 30 minutos vivia num mapa em memória, zerado a cada
// reinício. Um painel que reinicia — deploy, atualização, queda — recomeçava
// notificando tudo de novo, e o operador aprendia a ignorar o canal. Também não
// havia aviso de recuperação: o alerta entrava e nunca saía.
type AlertState struct {
	// Key identifica o par regra/alvo: "rule:<id>:<server_id>". É a mesma chave
	// que o mapa em memória usava, para a migração não precisar reinterpretar
	// nada.
	Key string `gorm:"size:128;primaryKey" json:"key"`

	RuleID   uint   `gorm:"index;not null" json:"rule_id"`
	ServerID string `gorm:"size:64;index" json:"server_id"`
	Severity string `gorm:"size:16" json:"severity"`

	// FirstBreachAt é quando a condição começou a ser violada sem interrupção.
	// É o que permite a regra "acima de X por N minutos": sem persistir o
	// começo, cada tick só enxerga a amostra dele.
	FirstBreachAt time.Time `json:"first_breach_at"`

	LastBreachAt   time.Time  `json:"last_breach_at"`
	LastNotifiedAt *time.Time `gorm:"index" json:"last_notified_at"`

	// Active distingue "violando agora" de "já violou um dia". É o que torna
	// possível notificar a recuperação uma única vez, quando passa a false.
	Active bool `gorm:"index;default:false" json:"active"`

	UpdatedAt time.Time `json:"updated_at"`
}

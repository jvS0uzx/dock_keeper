package database

import "time"

// AuditLog registra quem fez o quê, em qual alvo, com qual resultado.
//
// Existe porque nenhuma escrita do painel deixava rastro atribuível: parar um
// container, remover uma unidade ou criar um administrador eram indistinguíveis
// de qualquer outra linha do stdout do processo. Para um sistema que executa
// comando como root remotamente, o rastro é requisito de adoção, não conforto.
type AuditLog struct {
	ID uint      `gorm:"primaryKey" json:"id"`
	At time.Time `gorm:"index;not null" json:"at"`

	// O nome e o papel são COPIADOS, não referenciados. Usuário apagado não pode
	// levar embora o próprio rastro, e o papel muda com o tempo — o que importa
	// é qual era no momento da ação, não qual é hoje.
	ActorUserID   *uint  `gorm:"index" json:"actor_user_id"`
	ActorUsername string `gorm:"size:64;index" json:"actor_username"`
	ActorRole     string `gorm:"size:16" json:"actor_role"`

	SourceIP  string `gorm:"size:45;index" json:"source_ip"`
	UserAgent string `gorm:"size:255" json:"user_agent"`

	// Ação no formato recurso.verbo: container.stop, user.create, site.delete,
	// ssh.exec. O ponto separa para a consulta poder agrupar por prefixo.
	Action string `gorm:"size:64;index;not null" json:"action"`

	TargetType string `gorm:"size:32;index" json:"target_type"`
	TargetID   string `gorm:"size:64;index" json:"target_id"`
	// Rótulo copiado pelo mesmo motivo do nome do ator: o alvo pode deixar de
	// existir, e "removeu o servidor 7" não diz nada seis meses depois.
	TargetLabel string `gorm:"size:255" json:"target_label"`

	SiteID *uint `gorm:"index" json:"site_id"`

	// ok, denied ou error. Separados porque a resposta operacional é outra:
	// denied revela alguém tentando alcançar unidade alheia, error revela
	// máquina quebrada.
	Result string `gorm:"size:16;index;not null" json:"result"`

	// JSON com o que a ação recebeu, montado por allowlist em cada chamador —
	// nunca copiando o corpo da requisição. Uma auditoria que copiasse o corpo
	// de POST /api/auth/login viraria um depósito de senha em claro.
	Detail string `gorm:"type:jsonb" json:"detail"`
}

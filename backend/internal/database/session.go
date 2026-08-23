package database

import "time"

// UserSession persiste a sessão de login.
//
// Antes o mapa vivia em memória: reiniciar o painel derrubava todos os logins e
// impedia rodar mais de uma réplica, porque a sessão aberta contra uma delas não
// existia para a outra. Para um projeto que se propõe a ser instalado por
// terceiros, as duas coisas são bloqueio.
type UserSession struct {
	// A chave é o SHA-256 do token, nunca o token. Um vazamento da tabela — por
	// backup, por réplica de leitura, por SELECT de quem só deveria consultar —
	// não pode entregar sessão ativa de ninguém.
	TokenHash string `gorm:"size:64;primaryKey" json:"-"`

	UserID uint   `gorm:"index;not null" json:"user_id"`
	Role   string `gorm:"size:16;not null" json:"role"`

	// Username é copiado para o log de auditoria não depender de um JOIN com uma
	// linha que pode ter sido apagada.
	Username string `gorm:"size:64;index" json:"username"`

	ExpiresAt  time.Time `gorm:"index;not null" json:"expires_at"`
	CreatedAt  time.Time `json:"created_at"`
	LastSeenAt time.Time `json:"last_seen_at"`
}

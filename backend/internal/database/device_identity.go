package database

import "time"

// EnrollmentToken é o convite de uso único que um administrador emite para
// habilitar um dispositivo novo — agente ou coletor — numa unidade.
//
// Existe para acabar com o AGENT_INGEST_TOKEN único, que autenticava todos os
// dispositivos de todas as unidades: qualquer portador declarava a unidade que
// quisesse no corpo do envio, então uma estação comprometida em qualquer filial
// forjava inventário de outra, criava servidor por hostname e injetava métrica
// falsa que dispara ou silencia alerta.
type EnrollmentToken struct {
	ID uint `gorm:"primaryKey" json:"id"`

	// Só o hash mora aqui. O valor em claro é devolvido uma única vez, na
	// resposta da emissão — mesmo precedente de User.PasswordHash.
	TokenHash string `gorm:"size:64;uniqueIndex;not null" json:"-"`

	SiteID uint   `gorm:"index;not null" json:"site_id"`
	Kind   string `gorm:"size:16;not null" json:"kind"` // agent | collector

	ExpiresAt time.Time  `gorm:"index;not null" json:"expires_at"`
	UsedAt    *time.Time `json:"used_at"`

	CreatedBy uint      `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
}

// DeviceCredential é a credencial própria de um dispositivo, amarrada a uma
// unidade. Substitui o token compartilhado na ingestão.
type DeviceCredential struct {
	// DeviceID viaja em claro no header e identifica a linha. Existe separado do
	// segredo para a busca ser por chave, e não uma varredura comparando hash em
	// toda linha da tabela a cada push.
	DeviceID string `gorm:"size:32;primaryKey" json:"device_id"`

	SecretHash string `gorm:"size:64;not null" json:"-"`

	SiteID uint   `gorm:"index;not null" json:"site_id"`
	Kind   string `gorm:"size:16;not null" json:"kind"`

	// MachineID e Hostname são o que o dispositivo declarou no enrollment.
	// Guardados para o administrador reconhecer a linha na tela de dispositivos;
	// a autoridade sobre a unidade é a coluna SiteID, nunca o que vem no envio.
	MachineID string `gorm:"size:128;index" json:"machine_id"`
	Hostname  string `gorm:"size:255" json:"hostname"`

	CreatedAt  time.Time  `json:"created_at"`
	LastSeenAt *time.Time `json:"last_seen_at"`

	// RevokedAt marca a revogação em vez de apagar a linha: o histórico de
	// auditoria precisa continuar apontando para um dispositivo que existiu.
	RevokedAt *time.Time `gorm:"index" json:"revoked_at"`
}

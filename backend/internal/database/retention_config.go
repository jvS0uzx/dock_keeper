package database

import (
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

// Prazos padrão. Ficam juntos porque a decisão de cada um só faz sentido ao lado
// dos outros: métrica bruta é volumosa e vira tendência, tendência é barata e
// serve à comparação ano a ano, inventário é cadastro e some quando o
// equipamento some de verdade.
const (
	// Sete dias de métrica bruta. É inserção a cada poucos segundos por host;
	// mais que isso incha o Postgres até degradar as consultas do painel, e o
	// que se olha depois de uma semana é a tendência, não a amostra.
	DefaultMetricRetentionDays = 7
	DefaultLogRetentionDays    = 7

	// Pouco mais de um ano de tendência agregada por hora. O prazo existe para
	// permitir comparar um mês com o mesmo mês do ano anterior; encurtá-lo tira
	// a única pergunta que a tabela de tendências responde melhor que a bruta.
	DefaultTrendRetentionDays = 400

	// Trinta dias sem ser visto na varredura para o host sair do inventário.
	// Curto demais apaga notebook de quem tirou férias; longo demais deixa a
	// tela cheia de máquina que não existe mais.
	DefaultHostRetentionDays = 30
)

// RetentionDays lê um prazo em dias do ambiente, com padrão.
//
// Valor inválido, zero ou negativo cai no padrão com aviso: zero significaria
// "apagar tudo a cada passada", e um erro de digitação na configuração não pode
// ter esse efeito.
func RetentionDays(key string, def int) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return time.Duration(def) * 24 * time.Hour
	}

	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		log.Printf("[Config] %s=%q inválido, usando o padrão de %d dias", key, raw, def)
		return time.Duration(def) * 24 * time.Hour
	}
	return time.Duration(n) * 24 * time.Hour
}

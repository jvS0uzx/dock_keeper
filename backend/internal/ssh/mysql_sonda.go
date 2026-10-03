package ssh

import (
	"context"
	"log"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/config"
	"github.com/jvS0uzx/dock_keeper/scripts"
)

const intervaloPadraoDaSondaMySQL = 15 * time.Minute

var motoresDoMySQL = []string{motorMySQL, motorMariaDB}

func intervaloDaSondaMySQL() time.Duration {
	return config.Duracao("MYSQL_PROBE_INTERVAL", intervaloPadraoDaSondaMySQL)
}

func normalizarSondaMySQL(p PostgresProbePayload) PostgresProbePayload {
	return normalizarSondaDeBancos(p, motoresDoMySQL)
}

func gravarSondaMySQL(serverID string, p PostgresProbePayload, agora time.Time) error {
	return gravarSondaDeBancos(serverID, p, agora, motoresDoMySQL)
}

func SondarMySQL(ctx context.Context, t Target) (PostgresProbePayload, error) {
	return rodarSondaDeBancos(ctx, t, scripts.ProbeMySQL, "sonda do mysql não devolveu JSON")
}

func executarSondaMySQL(ctx context.Context, t Target) {
	payload, err := SondarMySQL(ctx, t)
	if err != nil {
		if ctx.Err() == nil {
			log.Printf("[MySQL] sonda de %s falhou: %v", t.Host, err)
		}
		return
	}
	if ctx.Err() != nil {
		return
	}

	agora := time.Now().UTC()
	if err := gravarSondaMySQL(t.ID, payload, agora); err != nil {
		log.Printf("[MySQL] erro ao gravar a sonda de %s: %v", t.Host, err)
		return
	}

	if !payload.DescobertaHostOk {
		log.Printf("[MySQL] %s: a listagem de portas não respondeu ou não mostra o dono do socket; "+
			"instâncias de host preservadas", t.Host)
	}
	if !payload.DescobertaContainerOk {
		log.Printf("[MySQL] %s: docker ps não respondeu; instâncias em container preservadas", t.Host)
	}

	for _, inst := range normalizarSondaMySQL(payload).Instancias {
		if inst.Estado == estadoPgAtivo {
			continue
		}
		log.Printf("[MySQL] %s porta %d classificada como %s: %s", t.Host, inst.Porta, inst.Estado, inst.Motivo)
	}
	log.Printf("[MySQL] %s: %d instância(s) inventariada(s)", t.Host, len(payload.Instancias))
}

func SondarMySQLPeriodicamente(ctx context.Context, t Target) {
	if sondaDeBancosDesligada("MySQL", t) {
		return
	}

	for {
		executarSondaMySQL(ctx, t)

		select {
		case <-ctx.Done():
			return
		case <-time.After(intervaloDaSondaMySQL()):
		}
	}
}

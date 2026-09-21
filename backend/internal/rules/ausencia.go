package rules

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/alert"
	"github.com/jvS0uzx/dock_keeper/internal/config"
	"github.com/jvS0uzx/dock_keeper/internal/database"
	"github.com/jvS0uzx/dock_keeper/internal/safego"
)

const (
	chaveAgenteAusente  = "agent_absent:"
	chaveColetorAusente = "collector_absent:"

	intervaloPadraoDoColetorSec = 900
)

type dispositivoVigiado struct {
	Chave        string
	Rotulo       string
	Severidade   string
	Nome         string
	AlvoID       string
	ServerID     *string
	SiteID       *uint
	IntervaloSec int
	Visto        time.Time
}

func StartAbsenceWatch(interval time.Duration) bool {
	if !config.Booleano("ABSENCE_ALERT", true) {
		log.Println("[rules] ABSENCE_ALERT=false: estação ou coletor que para de reportar não gera alerta")
		return false
	}

	inicio := time.Now()
	safego.Run(context.Background(), "rules:ausencia", func(context.Context) {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			vigiarAusencia(inicio, time.Now())
		}
	})
	return true
}

func vigiarAusencia(inicio, now time.Time) {
	vigiados, err := dispositivosVigiados()
	if err != nil {
		log.Printf("[rules] erro ao listar os dispositivos para o alerta de ausência: %v", err)
		return
	}

	abertas := map[string]bool{}
	for _, prefixo := range []string{chaveAgenteAusente, chaveColetorAusente} {
		for _, chave := range alert.ChavesAbertas(prefixo) {
			abertas[chave] = true
		}
	}

	for _, d := range vigiados {
		janela := database.LiveWindowFor(d.IntervaloSec)
		calado := now.Sub(d.Visto)
		aberta := abertas[d.Chave]
		delete(abertas, d.Chave)

		segundosCalado := calado.Seconds()
		segundosJanela := janela.Seconds()

		if calado > janela {
			if now.Sub(inicio) < janela {
				continue
			}
			alert.Enqueue(alert.Entrada{
				Key: d.Chave, Severity: d.Severidade, ServerID: d.ServerID, SiteID: d.SiteID,
				Text: fmt.Sprintf("%s %s %s sem reportar há %s (tolerância de %s)",
					Prefix(d.Severidade), d.Rotulo, d.Nome, calado.Round(time.Second), janela),
				AlvoTipo: database.AlvoTipoHost, AlvoID: d.AlvoID, AlvoNome: d.Nome,
				Metrica: "ausencia", Valor: &segundosCalado, Limiar: &segundosJanela, Unidade: "s",
			})
			continue
		}
		if aberta {
			alert.Recovered(alert.Entrada{
				Key: d.Chave, ServerID: d.ServerID, SiteID: d.SiteID,
				Text:     fmt.Sprintf("%s Recuperado - %s %s voltou a reportar", Prefix(SeverityInfo), d.Rotulo, d.Nome),
				AlvoTipo: database.AlvoTipoHost, AlvoID: d.AlvoID, AlvoNome: d.Nome,
				Metrica: "ausencia", Valor: &segundosCalado, Limiar: &segundosJanela, Unidade: "s",
			})
		}
	}

	for chave := range abertas {
		alert.Recovered(alert.Entrada{Key: chave})
	}
}

func dispositivosVigiados() ([]dispositivoVigiado, error) {
	var agentes []struct {
		ID           string
		Name         string
		SiteID       *uint
		IntervaloSec int
		Visto        time.Time
	}
	err := database.DB.Raw(`
		SELECT s.id, s.name, s.site_id, s.report_interval_sec AS intervalo_sec,
		       COALESCE((SELECT MAX(m.timestamp) FROM metric_servers m WHERE m.server_id = s.id), s.updated_at) AS visto
		FROM servers s
		WHERE s.deleted_at IS NULL AND s.kind = 'agent' AND s.absence_alert
		  AND NOT (
		    EXISTS (SELECT 1 FROM device_credentials d
		            WHERE d.kind = 'agent' AND d.machine_id = s.machine_id AND s.machine_id <> '' AND d.revoked_at IS NOT NULL)
		    AND NOT EXISTS (SELECT 1 FROM device_credentials d
		            WHERE d.kind = 'agent' AND d.machine_id = s.machine_id AND d.revoked_at IS NULL)
		  )
	`).Scan(&agentes).Error
	if err != nil {
		return nil, err
	}

	var coletores []struct {
		DeviceID     string
		Hostname     string
		SiteID       uint
		IntervaloSec int
		Visto        time.Time
	}
	err = database.DB.Raw(`
		SELECT DISTINCT ON (COALESCE(NULLIF(d.machine_id, ''), d.device_id))
		       d.device_id, d.hostname, d.site_id, d.report_interval_sec AS intervalo_sec,
		       COALESCE(d.last_seen_at, d.created_at) AS visto
		FROM device_credentials d
		WHERE d.kind = 'collector' AND d.revoked_at IS NULL
		ORDER BY COALESCE(NULLIF(d.machine_id, ''), d.device_id), COALESCE(d.last_seen_at, d.created_at) DESC
	`).Scan(&coletores).Error
	if err != nil {
		return nil, err
	}

	vigiados := make([]dispositivoVigiado, 0, len(agentes)+len(coletores))
	for _, a := range agentes {
		id := a.ID
		vigiados = append(vigiados, dispositivoVigiado{
			Chave: chaveAgenteAusente + a.ID, Rotulo: "Estação", Severidade: SeverityHigh, Nome: a.Name,
			AlvoID: a.ID, ServerID: &id, SiteID: a.SiteID, IntervaloSec: a.IntervaloSec, Visto: a.Visto,
		})
	}
	for _, c := range coletores {
		unidade := c.SiteID
		intervalo := c.IntervaloSec
		if intervalo <= 0 {
			intervalo = intervaloPadraoDoColetorSec
		}
		nome := c.Hostname
		if nome == "" {
			nome = c.DeviceID
		}
		vigiados = append(vigiados, dispositivoVigiado{
			Chave: chaveColetorAusente + c.DeviceID, Rotulo: "Coletor", Severidade: SeverityCritical, Nome: nome,
			AlvoID: c.DeviceID, SiteID: &unidade, IntervaloSec: intervalo, Visto: c.Visto,
		})
	}
	return vigiados, nil
}

package network

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/alert"
	"github.com/jvS0uzx/dock_keeper/internal/database"
	"github.com/jvS0uzx/dock_keeper/internal/safego"
)

const sslWarnDays = 14

const sslConcurrency = 8

func StartSSLWorker(interval time.Duration) {
	safego.Run(context.Background(), "network:ssl", func(context.Context) {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			CheckAllDomains()
			<-ticker.C
		}
	})
}

func CheckAllDomains() {
	var domains []database.Domain
	if err := database.DB.Find(&domains).Error; err != nil {
		log.Printf("[SSL] erro ao listar domínios: %v", err)
		return
	}

	sem := make(chan struct{}, sslConcurrency)
	var wg sync.WaitGroup
	for _, d := range domains {
		wg.Add(1)
		sem <- struct{}{}
		go func(dom database.Domain) {
			defer wg.Done()
			defer func() { <-sem }()
			CheckAndStore(dom)
		}(d)
	}
	wg.Wait()
	log.Printf("[SSL] verificação concluída: %d domínios", len(domains))
}

func CheckAndStore(d database.Domain) database.Domain {
	info := CheckSSL(d.Name)
	now := time.Now().UTC()

	d.Valid = info.Valid
	d.Issuer = info.Issuer
	d.DaysLeft = info.DaysLeft
	d.ErrorMsg = info.ErrorMsg
	d.InvalidReason = info.InvalidReason
	d.LastCheck = &now

	if err := database.DB.Model(&database.Domain{}).Where("id = ?", d.ID).Updates(map[string]any{
		"valid":          d.Valid,
		"issuer":         d.Issuer,
		"days_left":      d.DaysLeft,
		"error_msg":      d.ErrorMsg,
		"invalid_reason": d.InvalidReason,
		"last_check":     &now,
	}).Error; err != nil {
		log.Printf("[SSL] erro ao persistir o estado de %s: %v", d.Name, err)
	}

	avisarCertificado(d, info)
	return d
}

var (
	notifyAlert  = alert.Enqueue
	resolveAlert = alert.Recovered
)

func avisarCertificado(d database.Domain, info SSLInfo) {
	invalido := alertaDoDominio(d, "ssl_invalid:"+d.Name)
	vencendo := alertaDoDominio(d, "ssl_expiring:"+d.Name)

	invalido.Metrica = "estado"
	vencendo.Metrica = "certificado_dias"
	vencendo.Unidade = "dias"
	limiar := float64(sslWarnDays)
	vencendo.Limiar = &limiar

	if !info.Valid {
		invalido.Severity = "critical"
		invalido.Text = fmt.Sprintf("[CRITICO] Certificado de %s inválido: %s", d.Name, info.ErrorMsg)
		notifyAlert(invalido)
		return
	}

	invalido.Text = fmt.Sprintf("[INFO] Recuperado - Certificado de %s voltou a ser válido", d.Name)
	resolveAlert(invalido)

	dias := float64(info.DaysLeft)
	vencendo.Valor = &dias

	if info.DaysLeft <= sslWarnDays {
		vencendo.Severity = "high"
		vencendo.Text = fmt.Sprintf("[ALERTA] Certificado de %s expira em %d dias", d.Name, info.DaysLeft)
		notifyAlert(vencendo)
		return
	}
	vencendo.Text = fmt.Sprintf("[INFO] Recuperado - Certificado de %s renovado: %d dias de validade", d.Name, info.DaysLeft)
	resolveAlert(vencendo)
}

func alertaDoDominio(d database.Domain, chave string) alert.Entrada {
	e := alert.Entrada{
		Key: chave, ServerID: d.ServerID,
		AlvoTipo: database.AlvoTipoServico, AlvoID: d.Name, AlvoNome: d.Name,
	}
	if d.ServerID == nil || database.DB == nil {
		return e
	}

	var unidades []*uint
	err := database.DB.Model(&database.Server{}).Where("id = ?", *d.ServerID).Limit(1).Pluck("site_id", &unidades).Error
	if err != nil {
		log.Printf("[SSL] erro ao buscar a unidade do servidor de %s: %v", d.Name, err)
		return e
	}
	if len(unidades) == 1 {
		e.SiteID = unidades[0]
	}
	return e
}

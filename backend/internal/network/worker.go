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

	if !info.Valid {
		alert.Notify("ssl_invalid:"+d.Name,
			fmt.Sprintf("[CRITICO] Certificado de %s inválido: %s", d.Name, info.ErrorMsg))
	} else if info.DaysLeft <= sslWarnDays {
		alert.Notify("ssl_expiring:"+d.Name,
			fmt.Sprintf("[ALERTA] Certificado de %s expira em %d dias", d.Name, info.DaysLeft))
	}
	return d
}

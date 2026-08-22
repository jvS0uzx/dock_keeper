package network

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/joaov/vd_stats/internal/alert"
	"github.com/joaov/vd_stats/internal/database"
)

// Alerta quando o certificado está a esta distância (ou menos) do vencimento.
const sslWarnDays = 14

// Máximo de handshakes simultâneos ao varrer todos os domínios.
const sslConcurrency = 8

// StartSSLWorker checa todos os domínios cadastrados a cada interval, persiste
// o resultado em Domain e dispara alerta para os que estão vencendo ou inválidos.
func StartSSLWorker(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			CheckAllDomains()
			<-ticker.C
		}
	}()
}

// CheckAllDomains verifica todos os domínios em paralelo (pool limitado) e
// persiste cada resultado. Pode ser chamada pelo worker ou por endpoint manual.
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

// CheckAndStore faz o handshake de um domínio, grava o estado em Domain e
// dispara alerta se estiver inválido ou vencendo. Retorna o registro atualizado.
func CheckAndStore(d database.Domain) database.Domain {
	info := CheckSSL(d.Name)
	now := time.Now().UTC()

	d.Valid = info.Valid
	d.Issuer = info.Issuer
	d.DaysLeft = info.DaysLeft
	d.ErrorMsg = info.ErrorMsg
	d.LastCheck = &now

	database.DB.Model(&database.Domain{}).Where("id = ?", d.ID).Updates(map[string]interface{}{
		"valid":      d.Valid,
		"issuer":     d.Issuer,
		"days_left":  d.DaysLeft,
		"error_msg":  d.ErrorMsg,
		"last_check": &now,
	})

	if !info.Valid {
		alert.Notify("ssl_invalid:"+d.Name,
			fmt.Sprintf("[CRITICO] Certificado de *%s* inválido: %s", d.Name, info.ErrorMsg))
	} else if info.DaysLeft <= sslWarnDays {
		alert.Notify("ssl_expiring:"+d.Name,
			fmt.Sprintf("[ALERTA] Certificado de *%s* expira em *%d dias*", d.Name, info.DaysLeft))
	}
	return d
}

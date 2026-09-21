package database

import (
	"testing"
	"time"
)

func TestPodaTiraResolvidoVelhoEDeixaAberto(t *testing.T) {
	setupEnderecoDB(t)
	const prefixo = "m5-poda:"
	limpar := func() { DB.Where("key LIKE ?", prefixo+"%").Delete(&Alert{}) }
	limpar()
	t.Cleanup(limpar)

	velho := time.Now().UTC().Add(-60 * 24 * time.Hour)
	linhas := []Alert{
		{Key: prefixo + "resolvido", Severity: "info", Text: "resolvido velho",
			Status: AlertStatusResolved, CreatedAt: velho, Delivery: AlertDeliveryEnviado},
		{Key: prefixo + "aberto", Severity: "critical", Text: "aberto velho",
			Status: AlertStatusOpen, CreatedAt: velho, Delivery: AlertDeliveryEnviado},
		{Key: prefixo + "recente", Severity: "info", Text: "resolvido recente",
			Status: AlertStatusResolved, CreatedAt: time.Now().UTC(), Delivery: AlertDeliveryEnviado},
	}
	if err := DB.Create(&linhas).Error; err != nil {
		t.Fatalf("criar alertas: %v", err)
	}

	pruneAlerts(30 * 24 * time.Hour)

	for chave, quer := range map[string]int64{prefixo + "resolvido": 0, prefixo + "aberto": 1, prefixo + "recente": 1} {
		var n int64
		DB.Model(&Alert{}).Where("key = ?", chave).Count(&n)
		if n != quer {
			t.Errorf("%s: %d linha(s), esperado %d", chave, n, quer)
		}
	}
}

package alert

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/database"
)

type canalWebhook struct{}

func (canalWebhook) Nome() string { return CanalWebhook }

func (canalWebhook) Ativo() bool { return destinoDoWebhook() != "" }

func destinoDoWebhook() string {
	return strings.TrimSpace(os.Getenv("ALERT_WEBHOOK_URL"))
}

type webhookAlvo struct {
	Tipo string `json:"tipo,omitempty"`
	ID   string `json:"id,omitempty"`
	Nome string `json:"nome,omitempty"`
}

type webhookMedida struct {
	Metrica string   `json:"metrica,omitempty"`
	Valor   *float64 `json:"valor,omitempty"`
	Limiar  *float64 `json:"limiar,omitempty"`
	Unidade string   `json:"unidade,omitempty"`
}

type webhookCorpo struct {
	Chave       string        `json:"chave"`
	Severidade  string        `json:"severidade"`
	Status      string        `json:"status"`
	Texto       string        `json:"texto"`
	Alvo        webhookAlvo   `json:"alvo"`
	Medida      webhookMedida `json:"medida"`
	ServidorID  *string       `json:"servidor_id,omitempty"`
	UnidadeID   *uint         `json:"unidade_id,omitempty"`
	RegraID     *uint         `json:"regra_id,omitempty"`
	CriadoEm    time.Time     `json:"criado_em"`
	ResolvidoEm *time.Time    `json:"resolvido_em,omitempty"`
}

func corpoDoWebhook(a database.Alert) webhookCorpo {
	return webhookCorpo{
		Chave:      a.Key,
		Severidade: a.Severity,
		Status:     a.Status,
		Texto:      textoDoAlerta(a),
		Alvo: webhookAlvo{
			Tipo: textoDoPonteiro(a.AlvoTipo),
			ID:   textoDoPonteiro(a.AlvoID),
			Nome: textoDoPonteiro(a.AlvoNome),
		},
		Medida: webhookMedida{
			Metrica: textoDoPonteiro(a.Metrica),
			Valor:   a.Valor,
			Limiar:  a.Limiar,
			Unidade: textoDoPonteiro(a.Unidade),
		},
		ServidorID:  a.ServerID,
		UnidadeID:   a.SiteID,
		RegraID:     a.RuleID,
		CriadoEm:    a.CreatedAt,
		ResolvidoEm: a.ResolvedAt,
	}
}

func (canalWebhook) Entregar(a database.Alert) error {
	destino := destinoDoWebhook()
	if destino == "" {
		return ErrSemCanal
	}

	corpo, err := json.Marshal(corpoDoWebhook(a))
	if err != nil {
		return fmt.Errorf("montar o corpo do webhook: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, destino, bytes.NewReader(corpo))
	if err != nil {
		return fmt.Errorf("montar a requisição do webhook: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if segredo := strings.TrimSpace(os.Getenv("ALERT_WEBHOOK_TOKEN")); segredo != "" {
		req.Header.Set("Authorization", "Bearer "+segredo)
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))

	if resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("webhook respondeu %s", resp.Status)
	}
	return nil
}

package alert

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/jvS0uzx/dock_keeper/internal/database"
)

const servidorNtfyPadrao = "https://ntfy.sh"

var prioridadeDoNtfy = map[string]string{
	"critical": "urgent",
	"high":     "high",
	"warning":  "default",
	"info":     "low",
}

type canalNtfy struct{}

func (canalNtfy) Nome() string { return CanalNtfy }

func (canalNtfy) Ativo() bool { return topicoDoNtfy() != "" }

func topicoDoNtfy() string {
	return strings.Trim(strings.TrimSpace(os.Getenv("ALERT_NTFY_TOPIC")), "/")
}

func servidorDoNtfy() string {
	servidor := strings.TrimRight(strings.TrimSpace(os.Getenv("ALERT_NTFY_URL")), "/")
	if servidor == "" {
		return servidorNtfyPadrao
	}
	return servidor
}

func (canalNtfy) Entregar(a database.Alert) error {
	topico := topicoDoNtfy()
	if topico == "" {
		return ErrSemCanal
	}

	texto := textoDoAlerta(a)
	req, err := http.NewRequest(http.MethodPost, servidorDoNtfy()+"/"+topico, strings.NewReader(texto))
	if err != nil {
		return fmt.Errorf("montar a requisição do ntfy: %w", err)
	}

	req.Header.Set("Content-Type", "text/plain; charset=utf-8")
	req.Header.Set("Title", tituloDoNtfy(a))
	if prioridade, ok := prioridadeDoNtfy[strings.ToLower(strings.TrimSpace(a.Severity))]; ok {
		req.Header.Set("Priority", prioridade)
	}
	if segredo := strings.TrimSpace(os.Getenv("ALERT_NTFY_TOKEN")); segredo != "" {
		req.Header.Set("Authorization", "Bearer "+segredo)
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))

	if resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("ntfy respondeu %s", resp.Status)
	}
	return nil
}

func tituloDoNtfy(a database.Alert) string {
	if alvo := descricaoDoAlvo(a); alvo != "" {
		return "DockKeeper - " + alvo
	}
	return "DockKeeper"
}

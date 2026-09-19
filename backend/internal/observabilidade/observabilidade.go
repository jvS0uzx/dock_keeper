package observabilidade

import (
	"context"
	"expvar"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"sort"
	"strings"
	"sync"
)

var (
	AlertasEnfileirados = expvar.NewInt("dockkeeper_alertas_enfileirados")
	AlertasEntregues    = expvar.NewInt("dockkeeper_alertas_entregues")
	AlertasFalhos       = expvar.NewInt("dockkeeper_alertas_falhos")
	AlertasDescartados  = expvar.NewInt("dockkeeper_alertas_descartados")
	LogsDescartados     = expvar.NewInt("dockkeeper_logs_descartados")
	SessoesSSH          = expvar.NewInt("dockkeeper_sessoes_ssh_abertas")
	ReconexoesSSH       = expvar.NewInt("dockkeeper_reconexoes_ssh")
	PanicosRecuperados  = expvar.NewInt("dockkeeper_panicos_recuperados")
	MigracoesAplicadas  = expvar.NewInt("dockkeeper_migracoes_aplicadas")
)

var uma sync.Once

func Configurar() {
	uma.Do(func() {
		nivel := new(slog.LevelVar)
		nivel.Set(nivelConfigurado())

		handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: nivel})
		logger := slog.New(handler)
		slog.SetDefault(logger)

		log.SetFlags(0)
		log.SetOutput(&pontePadrao{logger: logger})
	})
}

func nivelConfigurado() slog.Level {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("LOG_LEVEL"))) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

type pontePadrao struct {
	logger *slog.Logger
}

func (p *pontePadrao) Write(linha []byte) (int, error) {
	texto := strings.TrimRight(string(linha), "\n")
	componente, mensagem := separarComponente(texto)

	nivel := slog.LevelInfo
	switch {
	case strings.Contains(mensagem, "erro") || strings.Contains(mensagem, "falha"):
		nivel = slog.LevelError
	case strings.Contains(mensagem, "AVISO") || strings.Contains(mensagem, "aviso"):
		nivel = slog.LevelWarn
	}

	if componente == "" {
		p.logger.Log(context.Background(), nivel, mensagem)
		return len(linha), nil
	}
	p.logger.Log(context.Background(), nivel, mensagem, slog.String("componente", componente))
	return len(linha), nil
}

func separarComponente(texto string) (string, string) {
	if !strings.HasPrefix(texto, "[") {
		return "", texto
	}
	fim := strings.Index(texto, "]")
	if fim < 0 {
		return "", texto
	}
	return strings.ToLower(texto[1:fim]), strings.TrimSpace(texto[fim+1:])
}

func Escrever(w io.Writer) {
	var nomes []string
	valores := map[string]string{}
	expvar.Do(func(kv expvar.KeyValue) {
		if strings.HasPrefix(kv.Key, "dockkeeper_") {
			nomes = append(nomes, kv.Key)
			valores[kv.Key] = kv.Value.String()
		}
	})

	sort.Strings(nomes)
	for _, nome := range nomes {
		fmt.Fprintf(w, "%s %s\n", nome, valores[nome])
	}
}

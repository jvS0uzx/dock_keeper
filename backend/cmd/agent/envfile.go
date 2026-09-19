package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func caminhoDoArquivoDeEnv(getenv func(string) string) string {
	if v := strings.TrimSpace(getenv("AGENT_ENV_FILE")); v != "" {
		return v
	}
	return filepath.Join(getenv("ProgramData"), "dockkeeper-agent", "agent.env")
}

func carregarArquivoDeEnv(caminho string, setenv func(chave, valor string) error) error {
	arquivo, err := os.Open(caminho)
	if err != nil {
		return fmt.Errorf("config %s: %w", caminho, err)
	}
	defer arquivo.Close()

	leitor := bufio.NewScanner(arquivo)
	for leitor.Scan() {
		linha := strings.TrimSpace(leitor.Text())
		if linha == "" || strings.HasPrefix(linha, "#") {
			continue
		}
		chave, valor, ok := strings.Cut(linha, "=")
		if !ok {
			continue
		}
		chave = strings.TrimSpace(chave)
		if chave == "" {
			continue
		}
		if err := setenv(chave, semAspas(strings.TrimSpace(valor))); err != nil {
			return fmt.Errorf("config %s, variavel %s: %w", caminho, chave, err)
		}
	}
	if err := leitor.Err(); err != nil {
		return fmt.Errorf("config %s: %w", caminho, err)
	}
	return nil
}

func semAspas(valor string) string {
	if len(valor) >= 2 {
		primeiro, ultimo := valor[0], valor[len(valor)-1]
		if (primeiro == '"' || primeiro == '\'') && primeiro == ultimo {
			return valor[1 : len(valor)-1]
		}
	}
	return valor
}

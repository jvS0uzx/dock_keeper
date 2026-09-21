package config

import (
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

func Inteiro(chave string, padrao int) int {
	raw := strings.TrimSpace(os.Getenv(chave))
	if raw == "" {
		return padrao
	}

	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		log.Printf("[Config] %s=%q inválido; usando %d", chave, raw, padrao)
		return padrao
	}
	return v
}

func Duracao(chave string, padrao time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(chave))
	if raw == "" {
		return padrao
	}

	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		log.Printf("[Config] %s=%q inválido; usando %s", chave, raw, padrao)
		return padrao
	}
	return d
}

func Booleano(chave string, padrao bool) bool {
	raw := strings.TrimSpace(os.Getenv(chave))
	if raw == "" {
		return padrao
	}

	v, err := strconv.ParseBool(raw)
	if err != nil {
		log.Printf("[Config] %s=%q inválido; usando %t", chave, raw, padrao)
		return padrao
	}
	return v
}

func Dias(chave string, padrao int) time.Duration {
	return time.Duration(Inteiro(chave, padrao)) * 24 * time.Hour
}

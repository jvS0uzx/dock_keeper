package api

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"io"
	"log"
	"mime/multipart"
	"os"
	"path/filepath"

	// Registram os decodificadores usados por image.DecodeConfig.
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
)

const (
	// Teto do upload da planta. Planta baixa de escritório cabe folgado em 8 MB;
	// o limite existe para uma requisição não conseguir encher o disco.
	maxPlanBytes = 8 << 20

	// Menor dimensão aceita. Abaixo disso não dá para posicionar marcador.
	minPlanSide = 100
)

// Tipos aceitos. Lista fechada: o valor vem do cliente e vira Content-Type na
// resposta, então um tipo arbitrário permitiria servir HTML pelo endpoint da
// imagem e transformar o painel em vetor de XSS.
var planContentTypes = map[string]string{
	"image/png":  ".png",
	"image/jpeg": ".jpg",
	"image/gif":  ".gif",
}

var errUnsupportedImage = errors.New("formato não suportado: use PNG, JPEG ou GIF")

// storedPlan é o resultado de gravar uma planta em disco.
type storedPlan struct {
	Path          string
	ContentType   string
	Width, Height int
}

// planDir devolve (criando se preciso) o diretório dos uploads.
func planDir() (string, error) {
	dir := os.Getenv("FLOORPLAN_DIR")
	if dir == "" {
		dir = "data/floorplans"
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", fmt.Errorf("erro ao preparar %s: %w", dir, err)
	}
	return dir, nil
}

// storePlanImage valida e grava a imagem enviada.
//
// O tipo e as dimensões saem do conteúdo do arquivo, nunca do que o cliente
// declarou: Content-Type e nome de arquivo são texto controlado por quem envia.
// O nome em disco é aleatório, o que também elimina path traversal.
func storePlanImage(file multipart.File) (storedPlan, error) {
	limited := io.LimitReader(file, maxPlanBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return storedPlan{}, fmt.Errorf("erro ao ler o arquivo: %w", err)
	}
	if len(data) > maxPlanBytes {
		return storedPlan{}, fmt.Errorf("arquivo maior que %d MB", maxPlanBytes>>20)
	}
	if len(data) == 0 {
		return storedPlan{}, errors.New("arquivo vazio")
	}

	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return storedPlan{}, errUnsupportedImage
	}

	contentType := "image/" + format
	if format == "jpeg" {
		contentType = "image/jpeg"
	}
	ext, ok := planContentTypes[contentType]
	if !ok {
		return storedPlan{}, errUnsupportedImage
	}
	if cfg.Width < minPlanSide || cfg.Height < minPlanSide {
		return storedPlan{}, fmt.Errorf("imagem menor que %dx%d", minPlanSide, minPlanSide)
	}

	dir, err := planDir()
	if err != nil {
		return storedPlan{}, err
	}

	name := make([]byte, 16)
	if _, err := rand.Read(name); err != nil {
		return storedPlan{}, err
	}
	path := filepath.Join(dir, hex.EncodeToString(name)+ext)

	if err := os.WriteFile(path, data, 0o640); err != nil {
		return storedPlan{}, fmt.Errorf("erro ao gravar a planta: %w", err)
	}

	return storedPlan{Path: path, ContentType: contentType, Width: cfg.Width, Height: cfg.Height}, nil
}

// removePlanImage apaga o arquivo da planta, ignorando ausência.
func removePlanImage(path string) {
	if path == "" {
		return
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		log.Printf("[API] erro ao remover a planta %s: %v", path, err)
	}
}

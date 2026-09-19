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

	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
)

const (
	maxPlanBytes = 8 << 20

	minPlanSide = 100
)

var planContentTypes = map[string]string{
	"image/png":  ".png",
	"image/jpeg": ".jpg",
	"image/gif":  ".gif",
}

var errUnsupportedImage = errors.New("formato não suportado: use PNG, JPEG ou GIF")

type storedPlan struct {
	Path          string
	ContentType   string
	Width, Height int
}

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

func removePlanImage(path string) {
	if path == "" {
		return
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		log.Printf("[API] erro ao remover a planta %s: %v", path, err)
	}
}

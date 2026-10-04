package database

import (
	"context"
	"testing"
	"time"
)

const prefixoDaPodaDeAnotacao = "poda-de-anotacao:"

func anotacoesDePoda(t *testing.T) {
	t.Helper()

	setupRetentionDB(t)
	limpar := func() { DB.Where("text LIKE ?", prefixoDaPodaDeAnotacao+"%").Delete(&Annotation{}) }
	limpar()
	t.Cleanup(limpar)

	agora := time.Now().UTC()
	linhas := []Annotation{
		{At: agora.Add(-400 * 24 * time.Hour), Text: prefixoDaPodaDeAnotacao + "antiga", AuthorUserID: 1},
		{At: agora.Add(-30 * 24 * time.Hour), Text: prefixoDaPodaDeAnotacao + "recente", AuthorUserID: 1},
	}
	if err := DB.Create(&linhas).Error; err != nil {
		t.Fatalf("criar anotações: %v", err)
	}
}

func contarAnotacao(t *testing.T, sufixo string) int64 {
	t.Helper()

	var n int64
	DB.Model(&Annotation{}).Where("text = ?", prefixoDaPodaDeAnotacao+sufixo).Count(&n)
	return n
}

func TestPodaTiraAnotacaoAlemDaRetencao(t *testing.T) {
	t.Setenv("ANNOTATION_RETENTION_DAYS", "")
	anotacoesDePoda(t)

	prune(context.Background(), 7*24*time.Hour, 365*24*time.Hour)

	if n := contarAnotacao(t, "antiga"); n != 0 {
		t.Errorf("anotação de 400 dias sobreviveu à retenção padrão de %d dias", defaultAnnotationRetentionDays)
	}
	if n := contarAnotacao(t, "recente"); n != 1 {
		t.Errorf("anotação de 30 dias foi podada")
	}
}

func TestRetencaoDeAnotacaoZeroNuncaPoda(t *testing.T) {
	t.Setenv("ANNOTATION_RETENTION_DAYS", "0")
	anotacoesDePoda(t)

	prune(context.Background(), 7*24*time.Hour, 365*24*time.Hour)

	if n := contarAnotacao(t, "antiga"); n != 1 {
		t.Errorf("ANNOTATION_RETENTION_DAYS=0 podou a anotação antiga")
	}
}

func TestRetencaoDeAnotacaoConfiguravel(t *testing.T) {
	t.Setenv("ANNOTATION_RETENTION_DAYS", "10")
	anotacoesDePoda(t)

	prune(context.Background(), 7*24*time.Hour, 365*24*time.Hour)

	if n := contarAnotacao(t, "recente"); n != 0 {
		t.Errorf("ANNOTATION_RETENTION_DAYS=10 deixou a anotação de 30 dias")
	}
}

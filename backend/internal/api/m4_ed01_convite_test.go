package api

import (
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/database"
)

func TestConviteConcorrenteGeraUmaCredencialSo(t *testing.T) {

	t.Setenv("INGEST_RATE_MAX_ENROLL", "1000")
	zerarLimiteDeIngestao()
	sedeA, _ := setupEnrollDB(t)

	const rodadas = 6
	const simultaneos = 32

	for rodada := 0; rodada < rodadas; rodada++ {
		convite := emitirConvite(t, sedeA, time.Hour)
		codigos := make([]int, simultaneos)

		var largada, fim sync.WaitGroup
		largada.Add(1)
		for i := 0; i < simultaneos; i++ {
			fim.Add(1)
			go func(i int) {
				defer fim.Done()
				largada.Wait()
				codigos[i] = chamarEnroll(t, convite, fmt.Sprintf("maquina-%d-%d", rodada, i)).Code
			}(i)
		}
		largada.Done()
		fim.Wait()

		criadas, recusadas := 0, 0
		for _, c := range codigos {
			switch c {
			case http.StatusCreated:
				criadas++
			case http.StatusUnauthorized:
				recusadas++
			default:
				t.Errorf("rodada %d: status inesperado %d", rodada, c)
			}
		}
		if criadas != 1 {
			t.Errorf("rodada %d: %d enrollments receberam 201, esperado 1 — convite de uso único gasto mais de uma vez", rodada, criadas)
		}
		if recusadas != simultaneos-1 {
			t.Errorf("rodada %d: %d recusas, esperadas %d", rodada, recusadas, simultaneos-1)
		}
	}

	var n int64
	database.DB.Model(&database.DeviceCredential{}).Where("site_id = ?", sedeA).Count(&n)
	if n != rodadas {
		t.Errorf("credenciais gravadas = %d, esperadas %d (uma por convite)", n, rodadas)
	}
}

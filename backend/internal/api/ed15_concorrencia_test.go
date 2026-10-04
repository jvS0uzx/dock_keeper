package api

import (
	"sync"
	"testing"
)

func TestEnviosConcorrentesSemMachineIDCriamUmServidorSo(t *testing.T) {
	sedeA, _ := setupChaveAgente(t)

	for _, caso := range []struct {
		nome string
		site *uint
	}{
		{"qa-n4-concorrente-sem-unidade", nil},
		{"qa-n4-concorrente-com-unidade", &sedeA},
	} {
		const envios = 16
		var wg sync.WaitGroup
		erros := make(chan error, envios)
		ids := make(chan string, envios)
		inicio := make(chan struct{})
		for range envios {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-inicio
				s, err := findOrCreateAgentServer(caso.nome, "", "10.95.0.1", caso.site)
				if err != nil {
					erros <- err
					return
				}
				ids <- s.ID
			}()
		}
		close(inicio)
		wg.Wait()
		close(erros)
		close(ids)

		for err := range erros {
			t.Errorf("%s: findOrCreateAgentServer concorrente: %v", caso.nome, err)
		}
		vistos := map[string]bool{}
		for id := range ids {
			vistos[id] = true
		}
		if len(vistos) != 1 {
			t.Errorf("%s: os envios devolveram %d servidores distintos, esperado 1", caso.nome, len(vistos))
		}
		if n := contarServidores(t, caso.nome); n != 1 {
			t.Errorf("%s: %d servidores gravados, esperado 1", caso.nome, n)
		}
	}
}

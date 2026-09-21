package database

import (
	"fmt"
	"testing"
	"time"
)

func limparAvisosDeRecusa() {
	avisosDeRecusa.mu.Lock()
	avisosDeRecusa.vistos = map[string]time.Time{}
	avisosDeRecusa.mu.Unlock()
}

func servidorDeColeta(t *testing.T, nome, ip, kind string, siteID *uint) string {
	t.Helper()

	DB.Unscoped().Where("name = ?", nome).Delete(&Server{})
	s := Server{Name: nome, HostIP: ip, User: "root", Port: 22, Kind: kind, SiteID: siteID}
	if err := DB.Create(&s).Error; err != nil {
		t.Fatalf("criar servidor %s: %v", nome, err)
	}
	t.Cleanup(func() {
		DB.Where("server_id = ?", s.ID).Delete(&ServerAddress{})
		DB.Unscoped().Where("id = ?", s.ID).Delete(&Server{})
	})
	return s.ID
}

func unidadeDeColeta(t *testing.T, codigo string) *uint {
	t.Helper()

	DB.Where("code = ?", codigo).Delete(&Site{})
	site := Site{Name: codigo, Code: codigo}
	if err := DB.Create(&site).Error; err != nil {
		t.Fatalf("criar unidade %s: %v", codigo, err)
	}
	t.Cleanup(func() { DB.Where("code = ?", codigo).Delete(&Site{}) })
	return &site.ID
}

func TestColetaNaoTomaHostIPDeOutroServidor(t *testing.T) {
	setupEnderecoDB(t)
	limparAvisosDeRecusa()
	servidorDeColeta(t, "m5-vps-dona", "203.0.113.41", "ssh", nil)
	estacao := servidorDeColeta(t, "m5-estacao-invasora", "203.0.113.42", "agent", nil)

	RegistrarEnderecos(estacao, []string{"203.0.113.41", "10.20.30.40"})

	gravados := enderecosDe(t, estacao)
	if _, tomou := gravados["203.0.113.41"]; tomou {
		t.Fatalf("a estação gravou como seu o host_ip de outro servidor: %v", gravados)
	}
	if _, ok := gravados["10.20.30.40"]; !ok {
		t.Errorf("o endereço legítimo do mesmo envio foi perdido: %v", gravados)
	}
}

func TestColetaNaoTomaAliasManualDeOutro(t *testing.T) {
	setupEnderecoDB(t)
	limparAvisosDeRecusa()
	dona := servidorDeColeta(t, "m5-dona-do-alias", "203.0.113.43", "ssh", nil)
	RegistrarAliases(dona, []string{"100.100.0.80"})
	outra := servidorDeColeta(t, "m5-quer-o-alias", "203.0.113.44", "ssh", nil)

	RegistrarEnderecos(outra, []string{"100.100.0.80"})

	if _, tomou := enderecosDe(t, outra)["100.100.0.80"]; tomou {
		t.Fatal("a coleta gravou como seu o alias manual de outro servidor")
	}
}

func TestColetaRespeitaTetoPorEnvio(t *testing.T) {
	setupEnderecoDB(t)
	limparAvisosDeRecusa()
	id := servidorDeColeta(t, "m5-muitos-enderecos", "203.0.113.45", "agent", nil)

	var muitos []string
	for i := 1; i <= 60; i++ {
		muitos = append(muitos, fmt.Sprintf("10.77.0.%d", i))
	}
	RegistrarEnderecos(id, muitos)

	if n := len(enderecosDe(t, id)); n != MaxEnderecosPorEnvio {
		t.Fatalf("envio com 60 endereços gravou %d, esperado o teto de %d", n, MaxEnderecosPorEnvio)
	}
}

func TestEnderecoColetadoParadoMudaDeDono(t *testing.T) {
	setupEnderecoDB(t)
	limparAvisosDeRecusa()
	antiga := servidorDeColeta(t, "m5-estacao-antiga", "203.0.113.46", "agent", nil)
	nova := servidorDeColeta(t, "m5-estacao-nova", "203.0.113.47", "agent", nil)

	RegistrarEnderecos(antiga, []string{"10.88.0.50"})
	RegistrarEnderecos(nova, []string{"10.88.0.50"})
	if _, tomou := enderecosDe(t, nova)["10.88.0.50"]; tomou {
		t.Fatal("endereço que a outra estação reportou agora mesmo mudou de dono")
	}

	velho := time.Now().UTC().Add(-2 * JanelaDeDonoColetado)
	DB.Model(&ServerAddress{}).Where("server_id = ? AND address = ?", antiga, "10.88.0.50").
		Update("ultimo_visto", velho)
	limparAvisosDeRecusa()

	RegistrarEnderecos(nova, []string{"10.88.0.50"})
	if _, ok := enderecosDe(t, nova)["10.88.0.50"]; !ok {
		t.Fatal("endereço parado na estação antiga (troca de DHCP) não passou para a nova")
	}
	if _, ficou := enderecosDe(t, antiga)["10.88.0.50"]; ficou {
		t.Error("o endereço ficou com dois donos depois da troca")
	}
}

func TestHostIPDeAgenteParadoNaoSeguraOEndereco(t *testing.T) {
	setupEnderecoDB(t)
	limparAvisosDeRecusa()
	parada := servidorDeColeta(t, "m5-agente-parado", "10.99.0.60", "agent", nil)
	viva := servidorDeColeta(t, "m5-agente-vivo", "203.0.113.48", "agent", nil)

	RegistrarEnderecos(viva, []string{"10.99.0.60"})
	if _, tomou := enderecosDe(t, viva)["10.99.0.60"]; tomou {
		t.Fatal("host_ip de agente que acabou de reportar foi tomado")
	}

	DB.Model(&Server{}).Where("id = ?", parada).
		UpdateColumn("updated_at", time.Now().UTC().Add(-2*JanelaDeDonoColetado))
	limparAvisosDeRecusa()

	RegistrarEnderecos(viva, []string{"10.99.0.60"})
	if _, ok := enderecosDe(t, viva)["10.99.0.60"]; !ok {
		t.Fatal("host_ip observado de agente parado continuou segurando o endereço")
	}
}

func TestHostIPDeServidorSSHNuncaCaduca(t *testing.T) {
	setupEnderecoDB(t)
	limparAvisosDeRecusa()
	vps := servidorDeColeta(t, "m5-vps-antiga", "203.0.113.49", "ssh", nil)
	DB.Model(&Server{}).Where("id = ?", vps).
		UpdateColumn("updated_at", time.Now().UTC().Add(-30*24*time.Hour))
	estacao := servidorDeColeta(t, "m5-estacao-oportunista", "203.0.113.50", "agent", nil)

	RegistrarEnderecos(estacao, []string{"203.0.113.49"})

	if _, tomou := enderecosDe(t, estacao)["203.0.113.49"]; tomou {
		t.Fatal("host_ip cadastrado por administrador caducou e foi tomado por uma estação")
	}
}

func TestFaixaPrivadaNaColetaEPorUnidade(t *testing.T) {
	setupEnderecoDB(t)
	limparAvisosDeRecusa()
	matriz := unidadeDeColeta(t, "m5-coleta-matriz")
	filial := unidadeDeColeta(t, "m5-coleta-filial")
	servidorDeColeta(t, "m5-servidor-matriz", "192.168.0.10", "ssh", matriz)
	naFilial := servidorDeColeta(t, "m5-estacao-filial", "203.0.113.51", "agent", filial)
	naMatriz := servidorDeColeta(t, "m5-estacao-matriz", "203.0.113.52", "agent", matriz)

	RegistrarEnderecos(naFilial, []string{"192.168.0.10"})
	if _, ok := enderecosDe(t, naFilial)["192.168.0.10"]; !ok {
		t.Error("faixa privada igual em outra unidade foi recusada na coleta")
	}

	RegistrarEnderecos(naMatriz, []string{"192.168.0.10"})
	if _, tomou := enderecosDe(t, naMatriz)["192.168.0.10"]; tomou {
		t.Error("faixa privada do servidor da mesma unidade foi tomada na coleta")
	}
}

func TestRecusaAvisaUmaVezPorJanela(t *testing.T) {
	setupEnderecoDB(t)
	limparAvisosDeRecusa()
	servidorDeColeta(t, "m5-dona-do-aviso", "203.0.113.53", "ssh", nil)
	estacao := servidorDeColeta(t, "m5-estacao-insistente", "203.0.113.54", "agent", nil)

	var avisos [][]EnderecoRecusado
	anterior := AoRecusarEndereco
	AoRecusarEndereco = func(serverID string, recusados []EnderecoRecusado) {
		if serverID == estacao {
			avisos = append(avisos, recusados)
		}
	}
	t.Cleanup(func() { AoRecusarEndereco = anterior })

	for i := 0; i < 5; i++ {
		RegistrarEnderecos(estacao, []string{"203.0.113.53"})
	}

	if len(avisos) != 1 {
		t.Fatalf("cinco envios com o mesmo endereço alheio geraram %d avisos, esperado 1", len(avisos))
	}
	if len(avisos[0]) != 1 || avisos[0][0].Endereco != "203.0.113.53" || avisos[0][0].Dono.Nome != "m5-dona-do-aviso" {
		t.Errorf("aviso não diz o endereço nem o dono: %+v", avisos[0])
	}
}

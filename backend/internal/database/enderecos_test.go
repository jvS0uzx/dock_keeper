package database

import (
	"os"
	"testing"
	"time"
)

func setupEnderecoDB(t *testing.T) {
	t.Helper()

	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("DATABASE_URL não definido; pulando teste de endereços")
	}
	if DB == nil {
		if err := Connect(); err != nil {
			t.Skipf("banco indisponível: %v", err)
		}
	}
}

func servidorParaEndereco(t *testing.T, nome, ip string) string {
	t.Helper()

	DB.Unscoped().Where("host_ip = ?", ip).Delete(&Server{})
	s := Server{Name: nome, HostIP: ip, User: "root", Port: 22}
	if err := DB.Create(&s).Error; err != nil {
		t.Fatalf("criar servidor: %v", err)
	}
	t.Cleanup(func() {
		DB.Where("server_id = ?", s.ID).Delete(&ServerAddress{})
		DB.Unscoped().Where("host_ip = ?", ip).Delete(&Server{})
	})
	return s.ID
}

func enderecosDe(t *testing.T, serverID string) map[string]ServerAddress {
	t.Helper()

	var linhas []ServerAddress
	if err := DB.Where("server_id = ?", serverID).Find(&linhas).Error; err != nil {
		t.Fatalf("ler endereços: %v", err)
	}
	fora := make(map[string]ServerAddress, len(linhas))
	for _, l := range linhas {
		fora[l.Address] = l
	}
	return fora
}

func TestRegistrarEnderecosGravaEAtualizaUltimoVisto(t *testing.T) {
	setupEnderecoDB(t)
	id := servidorParaEndereco(t, "vps-enderecos", "203.0.113.230")

	RegistrarEnderecos(id, []string{"100.100.0.11", "10.0.0.5"})
	depoisDoPrimeiro := enderecosDe(t, id)
	if len(depoisDoPrimeiro) != 2 {
		t.Fatalf("gravou %d endereços, esperado 2", len(depoisDoPrimeiro))
	}
	primeiro := depoisDoPrimeiro["100.100.0.11"]
	if primeiro.Origem != OrigemColetado {
		t.Errorf("origem gravada = %q, esperado %q", primeiro.Origem, OrigemColetado)
	}

	antes := primeiro.UltimoVisto
	time.Sleep(10 * time.Millisecond)
	RegistrarEnderecos(id, []string{"100.100.0.11"})

	depois := enderecosDe(t, id)
	if len(depois) != 2 {
		t.Errorf("segunda coleta mudou a contagem para %d; endereço ausente não se apaga na hora", len(depois))
	}
	if !depois["100.100.0.11"].UltimoVisto.After(antes) {
		t.Errorf("ultimo_visto não avançou: %v não é depois de %v", depois["100.100.0.11"].UltimoVisto, antes)
	}
	if !depois["100.100.0.11"].PrimeiroVisto.Equal(primeiro.PrimeiroVisto) {
		t.Errorf("primeiro_visto mudou de %v para %v", primeiro.PrimeiroVisto, depois["100.100.0.11"].PrimeiroVisto)
	}
}

func TestEnderecoInvalidoNaoEntra(t *testing.T) {
	setupEnderecoDB(t)
	id := servidorParaEndereco(t, "vps-invalido", "203.0.113.231")

	RegistrarEnderecos(id, []string{"nao-e-ip", "", "999.1.1.1", "127.0.0.1", "::1", "10.1.2.3"})

	gravados := enderecosDe(t, id)
	if len(gravados) != 1 {
		t.Fatalf("gravou %d endereços, esperado só o 10.1.2.3: %v", len(gravados), gravados)
	}
	if _, ok := gravados["10.1.2.3"]; !ok {
		t.Errorf("o endereço válido não foi gravado")
	}
}

func TestPodaTiraColetadoVelhoEMantemManual(t *testing.T) {
	setupEnderecoDB(t)
	id := servidorParaEndereco(t, "vps-poda", "203.0.113.232")

	RegistrarEnderecos(id, []string{"10.2.0.1", "10.2.0.2"})
	RegistrarAliases(id, []string{"100.100.0.10"})

	velho := time.Now().UTC().Add(-40 * 24 * time.Hour)
	if err := DB.Model(&ServerAddress{}).
		Where("server_id = ? AND address IN ?", id, []string{"10.2.0.1", "100.100.0.10"}).
		Update("ultimo_visto", velho).Error; err != nil {
		t.Fatalf("envelhecer endereços: %v", err)
	}

	PodarEnderecos(30 * 24 * time.Hour)

	restaram := enderecosDe(t, id)
	if _, ok := restaram["10.2.0.1"]; ok {
		t.Errorf("endereço coletado com 40 dias sobreviveu à poda de 30")
	}
	if _, ok := restaram["10.2.0.2"]; !ok {
		t.Errorf("endereço coletado recente foi podado")
	}
	if _, ok := restaram["100.100.0.10"]; !ok {
		t.Errorf("alias manual foi podado; manual nunca some por idade")
	}
}

func TestAliasManualNaoViraColetado(t *testing.T) {
	setupEnderecoDB(t)
	id := servidorParaEndereco(t, "vps-alias", "203.0.113.233")

	RegistrarAliases(id, []string{"100.100.0.11"})
	RegistrarEnderecos(id, []string{"100.100.0.11"})

	gravados := enderecosDe(t, id)
	if gravados["100.100.0.11"].Origem != OrigemManual {
		t.Errorf("origem virou %q depois da coleta; alias manual não pode ser rebaixado",
			gravados["100.100.0.11"].Origem)
	}
	if gravados["100.100.0.11"].UltimoVisto.IsZero() {
		t.Errorf("ultimo_visto do alias não foi atualizado pela coleta")
	}
}

func TestEnderecosPorServidorDevolveUniao(t *testing.T) {
	setupEnderecoDB(t)
	id := servidorParaEndereco(t, "vps-uniao", "203.0.113.234")

	RegistrarEnderecos(id, []string{"10.3.0.1"})
	RegistrarAliases(id, []string{"100.100.0.19"})

	porServidor, err := EnderecosPorServidor([]string{id})
	if err != nil {
		t.Fatalf("EnderecosPorServidor: %v", err)
	}
	lista := porServidor[id]
	if len(lista) != 2 {
		t.Fatalf("devolveu %v, esperado os dois endereços", lista)
	}
}

package ssh

import (
	"strings"
	"testing"
	"time"

	"github.com/jvS0uzx/dock_keeper/internal/database"
)

func sondaCompleta() NginxProbePayload {
	return NginxProbePayload{
		Instalado: true, Ativo: true, ConfigLida: true, LogLegivel: true,
		LogCaminho: "/var/log/nginx/access.log", Usuario: "deploy",
		Upstreams: []NginxUpstreamPayload{
			{Bloco: "api", Destinos: []string{"203.0.113.10:8080", "203.0.113.11:8080"}},
		},
	}
}

func TestClassificarNginxCobreOsQuatroEstados(t *testing.T) {
	casos := []struct {
		nome    string
		ajustar func(*NginxProbePayload)
		estado  string
	}{
		{"sem nginx", func(p *NginxProbePayload) { p.Instalado = false; p.Ativo = false }, database.NginxAusente},
		{"instalado e parado", func(p *NginxProbePayload) { p.Ativo = false }, database.NginxInativo},
		{"site sem upstream", func(p *NginxProbePayload) { p.Upstreams = nil }, database.NginxSemUpstream},
		{"proxy reverso", func(p *NginxProbePayload) {}, database.NginxCandidato},
	}

	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			p := sondaCompleta()
			caso.ajustar(&p)
			estado, motivo := classificarNginx(p)
			if estado != caso.estado {
				t.Errorf("estado = %q, esperado %q", estado, caso.estado)
			}
			if motivo != "" {
				t.Errorf("motivo = %q, esperado vazio: nada ficou por verificar", motivo)
			}
		})
	}
}

func TestClassificarSemPermissaoNoLogExplicaOUsuario(t *testing.T) {
	p := sondaCompleta()
	p.LogLegivel = false
	p.LogMotivo = "arquivo existe e nao e legivel"

	estado, motivo := classificarNginx(p)
	if estado != database.NginxCandidato {
		t.Errorf("estado = %q, esperado candidato: a máquina continua sendo proxy reverso", estado)
	}
	if !strings.Contains(motivo, "deploy") || !strings.Contains(motivo, "/var/log/nginx/access.log") {
		t.Errorf("motivo = %q, esperado citar o usuário e o caminho do log", motivo)
	}
}

func TestClassificarSemLerConfiguracaoNaoAfirmaQueNaoHaUpstream(t *testing.T) {
	p := sondaCompleta()
	p.ConfigLida = false
	p.ConfigMotivo = "nginx: (13: Permission denied)"
	p.Upstreams = nil

	estado, motivo := classificarNginx(p)
	if estado != database.NginxDesconhecido {
		t.Errorf("estado = %q, esperado desconhecido: configuração não lida não prova ausência de upstream", estado)
	}
	if !strings.Contains(motivo, "sudo") {
		t.Errorf("motivo = %q, esperado apontar o caminho da correção", motivo)
	}
}

func TestPortaoDaColetaSegueASondaEASobreposicao(t *testing.T) {
	alvo := Target{ID: "portao-teste-1"}
	t.Cleanup(func() { esquecerColeta(alvo.ID) })

	if coletaDeNginxLiberada(alvo) {
		t.Error("coleta liberada sem sonda: ausência de dado não é permissão")
	}

	registrarColeta(alvo.ID, false)
	if coletaDeNginxLiberada(alvo) {
		t.Error("coleta liberada com sonda negando")
	}

	registrarColeta(alvo.ID, true)
	if !coletaDeNginxLiberada(alvo) {
		t.Error("coleta bloqueada com sonda liberando")
	}

	esquecerColeta(alvo.ID)
	manual := Target{ID: alvo.ID, CollectNginx: true}
	if !coletaDeNginxLiberada(manual) {
		t.Error("sobreposição manual ignorada")
	}
}

func TestGravarSondaNginxAtualizaEstadoETopologia(t *testing.T) {
	srv := servidorDeTeste(t)

	agora := time.Now().UTC()
	if err := gravarSondaNginx(srv.ID, sondaCompleta(), agora); err != nil {
		t.Fatalf("gravar sonda: %v", err)
	}

	var gravado database.Server
	if err := database.DB.Where("id = ?", srv.ID).Take(&gravado).Error; err != nil {
		t.Fatalf("reler servidor: %v", err)
	}
	if gravado.NginxEstado != database.NginxCandidato {
		t.Errorf("nginx_estado = %q, esperado candidato", gravado.NginxEstado)
	}
	if gravado.NginxChecadoEm == nil {
		t.Error("nginx_checado_em nulo depois da sonda")
	}

	var destinos []database.NginxUpstream
	database.DB.Where("server_id = ?", srv.ID).Order("destino ASC").Find(&destinos)
	if len(destinos) != 2 {
		t.Fatalf("destinos = %d, esperado 2", len(destinos))
	}
}

func TestGravarSondaNginxRemoveDestinoQueSumiuDaConfiguracao(t *testing.T) {
	srv := servidorDeTeste(t)

	if err := gravarSondaNginx(srv.ID, sondaCompleta(), time.Now().UTC()); err != nil {
		t.Fatalf("primeira sonda: %v", err)
	}

	menor := sondaCompleta()
	menor.Upstreams[0].Destinos = []string{"203.0.113.10:8080"}
	if err := gravarSondaNginx(srv.ID, menor, time.Now().UTC().Add(time.Second)); err != nil {
		t.Fatalf("segunda sonda: %v", err)
	}

	var destinos []database.NginxUpstream
	database.DB.Where("server_id = ?", srv.ID).Find(&destinos)
	if len(destinos) != 1 || destinos[0].Destino != "203.0.113.10:8080" {
		t.Fatalf("destinos = %+v, esperado só o que a configuração ainda declara", destinos)
	}
}

func TestGravarSondaSemLerConfiguracaoPreservaATopologia(t *testing.T) {
	srv := servidorDeTeste(t)

	if err := gravarSondaNginx(srv.ID, sondaCompleta(), time.Now().UTC()); err != nil {
		t.Fatalf("primeira sonda: %v", err)
	}

	cega := sondaCompleta()
	cega.ConfigLida = false
	cega.Upstreams = nil
	if err := gravarSondaNginx(srv.ID, cega, time.Now().UTC().Add(time.Second)); err != nil {
		t.Fatalf("segunda sonda: %v", err)
	}

	var destinos []database.NginxUpstream
	database.DB.Where("server_id = ?", srv.ID).Find(&destinos)
	if len(destinos) != 2 {
		t.Errorf("destinos = %d, esperado 2: sonda cega não apaga topologia observada antes", len(destinos))
	}
}

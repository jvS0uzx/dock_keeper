package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/joaov/vd_stats/internal/auth"
	"github.com/joaov/vd_stats/internal/database"
)

// A ação de container é auditada pelo próprio handler, com o verbo, o servidor
// alvo, a unidade e o nome do container, gravando antes de o comando sair. O
// middleware genérico só enxerga método, rota e status.
//
// Por isso /api/containers/action sai do wrapper que aplica o middleware: com
// os dois na cadeia, cada ação gerava duas linhas dizendo a mesma coisa, e a
// menos informativa das duas aparecia junto na consulta do administrador.
//
// O teste roda pelo mux montado, não chamando o handler direto, porque o que se
// mede aqui é a fiação — chamar o handler nunca passaria pelo middleware e
// daria verde mesmo com a duplicação de pé.
func TestAcaoDeContainerGeraUmaLinhaSo(t *testing.T) {
	setupAuditAPI(t)

	global := sessaoDeTeste(t, "operador-global-container", []auth.Access{{SiteID: nil, Role: auth.RoleOperator}})

	corpo := `{"server_id":"00000000-0000-0000-0000-0000000000ff","container_name":"nginx_proxy","action":"stop"}`
	req := httptest.NewRequest(http.MethodPost, "/api/containers/action", strings.NewReader(corpo))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+global.Token)

	rec := httptest.NewRecorder()
	Routes(testConfig()).ServeHTTP(rec, req)

	// O servidor não existe, então a recusa por alcance é o desfecho — e é
	// justamente um dos casos em que as duas linhas apareciam.
	var linhas []database.AuditLog
	if err := database.DB.Where("action LIKE ?", "container.%").
		Or("action = ?", "desconhecido.create").
		Find(&linhas).Error; err != nil {
		t.Fatalf("consultar auditoria: %v", err)
	}

	if len(linhas) != 1 {
		nomes := make([]string, 0, len(linhas))
		for _, l := range linhas {
			nomes = append(nomes, l.Action+"/"+l.Result)
		}
		t.Fatalf("linhas gravadas = %d (%s), esperada 1: o middleware genérico está duplicando o registro do handler",
			len(linhas), strings.Join(nomes, ", "))
	}
	if linhas[0].Action != "container.stop" {
		t.Errorf("ação = %q, esperada container.stop: sobrou a linha genérica em vez da específica", linhas[0].Action)
	}
}

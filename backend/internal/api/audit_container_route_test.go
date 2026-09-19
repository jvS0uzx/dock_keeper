package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jvS0uzx/dock_keeper/internal/auth"
	"github.com/jvS0uzx/dock_keeper/internal/database"
)

func TestAcaoDeContainerGeraUmaLinhaSo(t *testing.T) {
	setupAuditAPI(t)

	global := sessaoDeTeste(t, "operador-global-container", []auth.Access{{SiteID: nil, Role: auth.RoleOperator}})

	corpo := `{"server_id":"00000000-0000-0000-0000-0000000000ff","container_name":"nginx_proxy","action":"stop"}`
	req := httptest.NewRequest(http.MethodPost, "/api/containers/action", strings.NewReader(corpo))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+global.Token)

	rec := httptest.NewRecorder()
	Routes(testConfig()).ServeHTTP(rec, req)

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

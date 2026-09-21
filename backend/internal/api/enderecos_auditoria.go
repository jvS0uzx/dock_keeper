package api

import (
	"github.com/jvS0uzx/dock_keeper/internal/audit"
	"github.com/jvS0uzx/dock_keeper/internal/database"
)

func init() {
	database.AoRecusarEndereco = auditarEnderecoRecusado
}

func auditarEnderecoRecusado(serverID string, recusados []database.EnderecoRecusado) {
	if database.DB == nil || len(recusados) == 0 {
		return
	}

	var quemDeclarou database.Server
	database.DB.Unscoped().Select("id", "name", "site_id", "kind").Where("id = ?", serverID).Take(&quemDeclarou)

	enderecos := make([]map[string]any, 0, len(recusados))
	for _, r := range recusados {
		enderecos = append(enderecos, map[string]any{
			"endereco": r.Endereco,
			"dono":     r.Dono.Nome,
			"dono_id":  r.Dono.ServerID,
			"unidade":  r.Dono.Unidade,
		})
	}

	audit.Record(audit.Entry{
		Action:      "server.address_refused",
		TargetType:  "server",
		TargetID:    serverID,
		TargetLabel: quemDeclarou.Name,
		SiteID:      quemDeclarou.SiteID,
		Result:      audit.ResultDenied,
		Detail: map[string]any{
			"origem":    quemDeclarou.Kind,
			"enderecos": enderecos,
		},
	})
}

package api

import (
	"net/http"

	"github.com/joaov/vd_stats/internal/audit"
	"github.com/joaov/vd_stats/internal/database"
)

// Ações dos streams que expõem dado sensível do host remoto.
const (
	actionAuthLogOpen       = "auth-log.open"
	actionContainerLogsOpen = "container-logs.open"
)

// auditStreamOpen registra a ABERTURA de um stream SSE que lê dado sensível do
// host por SSH.
//
// Streams são leitura, e leitura não é auditada nas outras rotas — auditar GET
// encheria a tabela sem informação. Estes dois são a exceção porque o que se lê
// é /var/log/auth.log e a saída da aplicação monitorada, como root, numa máquina
// de produção, por decisão de uma pessoa. "Quem leu o log de autenticação do
// servidor X" é exatamente o tipo de pergunta que a auditoria existe para
// responder.
//
// Uma linha por abertura, nunca por evento transmitido: o stream fica horas no
// ar cuspindo linha, e registrar o conteúdo transformaria a auditoria numa
// segunda cópia do log — inclusive do que houver de sensível nele.
func (c Config) auditStreamOpen(r *http.Request, action string, server database.Server, detail map[string]any) {
	entry := c.auditActor(r)
	entry.Action = action
	entry.TargetType = "server"
	entry.TargetID = server.ID
	entry.TargetLabel = server.Name
	entry.SiteID = server.SiteID
	entry.Result = audit.ResultOK
	entry.Detail = detail
	audit.Record(entry)
}

// auditStreamDenied registra a recusa de abertura, que é o sinal mais direto de
// alguém tentando ler host de unidade alheia. lookupServer responde 404 e não
// 403 para não confirmar existência, então sem esta linha a tentativa não
// deixaria rastro nenhum.
func (c Config) auditStreamDenied(r *http.Request, action, serverID string) {
	entry := c.auditActor(r)
	entry.Action = action
	entry.TargetType = "server"
	entry.TargetID = serverID
	entry.Result = audit.ResultDenied
	audit.Record(entry)
}

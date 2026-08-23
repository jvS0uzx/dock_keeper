package api

import (
	"context"
	"net/http"

	"github.com/joaov/vd_stats/internal/audit"
	"github.com/joaov/vd_stats/internal/auth"
)

// auditActor preenche os campos de ator de uma linha de auditoria a partir da
// requisição.
//
// Fica em arquivo próprio porque as duas metades do rastro precisam do mesmo
// preenchimento: o middleware das rotas de escrita e os handlers que disparam
// comando por SSH. Duplicar isso faria as duas metades divergirem justamente no
// campo que as une, e o rastro deixaria de fechar.
//
// Nome de usuário vazio significa "sem sessão": tentativa de login recusada, ou
// rota máquina-a-máquina autenticada por token de dispositivo. Quem audita esses
// casos preenche ActorUsername explicitamente.
func (c Config) auditActor(r *http.Request) audit.Entry {
	return c.auditActorOf(sessionFrom(r), r)
}

// auditActorOf preenche o ator a partir de uma sessão já resolvida.
//
// Existe separada porque o middleware de auditoria não consegue usar
// sessionFrom: ele corre ANTES do gate de credencial, de propósito, para que a
// recusa por falta de permissão também vire linha. Nesse ponto a requisição
// dele ainda não carrega sessão nenhuma, e o ator sairia vazio em toda escrita
// autenticada — metade da pergunta que a auditoria existe para responder.
func (c Config) auditActorOf(sess auth.Session, r *http.Request) audit.Entry {
	e := audit.Entry{
		ActorUsername: sess.Username,
		ActorRole:     sess.Role,
		SourceIP:      clientIP(r, c.TrustProxyHeaders),
		UserAgent:     r.UserAgent(),
	}
	if sess.UserID != 0 {
		id := sess.UserID
		e.ActorUserID = &id
	}
	return e
}

// auditTargetInfo é o alvo da ação como só o handler pode conhecê-lo.
//
// O middleware enxerga método, rota e query — o suficiente para saber QUE uma
// escrita aconteceu, não o suficiente para dizer EM QUE. O rótulo do alvo só
// existe depois de o handler carregar o registro, e a unidade quase sempre vem
// do corpo, que o middleware não lê nem pode ler.
type auditTargetInfo struct {
	typ    string
	id     string
	label  string
	siteID *uint

	// sess é preenchida por withSession quando o gate de credencial resolve
	// quem está chamando. O middleware corre antes desse gate, então esta é a
	// única forma de ele saber o ator sem abrir mão de auditar a recusa.
	sess    auth.Session
	sessSet bool

	// set distingue "o handler não disse nada" de "o handler disse que não há
	// unidade". Sem essa marca, uma ação sem unidade — criar usuário — apagaria
	// silenciosamente o site_id que o middleware tinha derivado da query, e uma
	// ação com unidade nula seria indistinguível de handler que esqueceu.
	set bool
}

type auditTargetCtxKey struct{}

// withAuditTarget prepara o espaço que o handler preenche durante a requisição.
//
// Só o middleware de auditoria chama isto. Handler alcançado sem passar por ele
// — rota isenta, ou chamada direta num teste — encontra o contexto vazio, e
// auditTarget vira no-op em vez de estourar.
func withAuditTarget(r *http.Request) (*http.Request, *auditTargetInfo) {
	info := &auditTargetInfo{}
	return r.WithContext(context.WithValue(r.Context(), auditTargetCtxKey{}, info)), info
}

// auditTarget registra o alvo real da ação na linha que o middleware já vai
// gravar.
//
// É deliberadamente o handler que chama, e não um audit.Record próprio: a
// cobertura da auditoria vem de o registro morar no wrapper, então handler novo
// que esqueça de auditar não existe. Aqui o handler só ENRIQUECE uma linha que
// sairia de qualquer jeito — esquecer de chamar perde o rótulo, nunca o rastro.
//
// Chamar sempre define a unidade, inclusive para nil: quem chama afirma que
// esta é a unidade da ação, e essa afirmação vence a que o middleware adivinhou
// pela query string.
//
// Preencha a partir do registro JÁ RESOLVIDO pelo handler, nunca do corpo cru:
// o corpo é o que o cliente pediu, e o rastro precisa dizer o que de fato
// aconteceu. Numa exclusão, leia o rótulo antes de a linha sumir.
func auditTarget(r *http.Request, targetType, id, label string, siteID *uint) {
	info, ok := r.Context().Value(auditTargetCtxKey{}).(*auditTargetInfo)
	if !ok {
		return
	}
	info.typ = targetType
	info.id = id
	info.label = label
	info.siteID = siteID
	info.set = true
}

# 006 — A auditoria mora na fronteira, não nos handlers

## Contexto

Nenhuma escrita do painel deixava rastro atribuível. Parar um container, remover
uma unidade ou criar um administrador eram indistinguíveis de qualquer outra
linha do stdout.

Havia duas formas de corrigir: chamar `audit.Record` em cada handler de escrita,
ou registrar num middleware na cadeia de rota.

## Decisão

Middleware `audit(...)` nos wrappers de `server.go`, cobrindo toda rota não-`GET`.

Handlers **enriquecem** a linha por um valor mutável no contexto
(`auditTarget(...)`), mas **nunca gravam** por conta própria.

## Consequência

**A favor.** Handler novo que esqueça de auditar não existe, porque o registro
está no wrapper que ele já usa. Cobertura por construção, não por disciplina.

**Contra.** O middleware enxerga método, rota e status — o suficiente para saber
*que* uma escrita aconteceu, não *em quê*. O rótulo do alvo só existe depois de o
handler carregar o registro, e a unidade quase sempre vem do corpo, que o
middleware não lê nem pode ler.

**Mitigação.** O enriquecimento é opcional por desenho: quem não chama
`auditTarget` gera exatamente a linha de antes. Esquecer perde o rótulo, nunca o
rastro. Há teste que trava isso.

**Detalhe que custou caro.** O middleware corre **antes** do gate de credencial —
de propósito, para a recusa também virar linha. Nesse ponto a sessão ainda não foi
resolvida, e `withSession` cria uma requisição *nova* que o middleware nunca vê.
O resultado era ator vazio em **todas** as linhas de escrita autenticada: "quem
fez" é metade da pergunta que a auditoria existe para responder, e ela estava em
branco. Hoje `withSession` avisa o espaço que o middleware deixou no contexto.

**A exceção.** `/api/containers/action` sai do middleware genérico e audita a si
próprio, porque precisa gravar `pending` **antes** de o comando sair — gravar só
no fim perderia justamente o caso que mais importa, o comando que travou a
máquina e nunca retornou. Com os dois na cadeia, cada ação gerava duas linhas
dizendo a mesma coisa. Há teste que roda pelo mux montado para travar isso;
chamar o handler direto nunca passaria pelo middleware e daria verde com a
duplicação de pé.

**Onde o pacote mora.** `internal/audit`, e não dentro de `internal/api`, porque
`internal/ssh` também precisa escrever — e `internal/api` já importa
`internal/ssh`. Um pacote de auditoria dentro do HTTP fecharia o ciclo.

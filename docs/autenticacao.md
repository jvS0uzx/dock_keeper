# Autenticação e controle de acesso

O painel tem **dois eixos independentes** de permissão, e confundi-los é a fonte
de quase todo mal-entendido sobre o assunto:

- **`User.Role`** diz *o que* a pessoa pode fazer;
- **`UserSiteAccess`** diz *onde* ela pode fazer.

O modelo é o do Zabbix, reduzido ao que este painel usa.

## Papéis

Três, do menor para o maior privilégio (`internal/auth/auth.go`):

| Papel | Na interface | O que alcança |
|---|---|---|
| `viewer` | Visualizador | Só leitura. **Não cadastra nada** |
| `operator` | **Suporte TI** | Leitura + ações em container, varredura, cadastro |
| `admin` | Administrador | Tudo, mais usuários e servidores |

Os papéis são ordenados: `Allows(has, needs)` compara postos, então `admin`
satisfaz qualquer exigência de `operator` ou `viewer`.

Papel desconhecido, e o vazio, ficam **abaixo** de qualquer papel válido. Isso não
é detalhe: como consulta a mapa devolve zero para chave ausente, sem o
tratamento explícito o `viewer` — que tem posto zero — nunca venceria o vazio numa
comparação.

## Alcance por unidade

`UserSiteAccess` é uma linha por concessão:

```go
type Access struct {
    SiteID *uint   // nulo = concessão GLOBAL
    Role   string
}
```

| Situação | Significado |
|---|---|
| `SiteID` nulo | **Concessão global**: vale para todas as unidades **e** para o que não tem unidade nenhuma (VPS, painel Dev) |
| `SiteID` preenchido | Vale só naquela unidade |
| Usuário **sem nenhuma linha** | O papel da conta vale globalmente — é o comportamento antigo, preservado |

Uma pessoa pode ter várias concessões: `operator` na filial A, `viewer` na filial
B, e nada em C. No alvo, **vence a maior**: `RoleForSite` parte do papel global e
sobe se houver concessão específica melhor para aquela unidade.

Alvo **sem** unidade (uma VPS, o painel Dev) só é alcançado por concessão global.
Não há como conceder "a VPS" a alguém de uma filial — ela não pertence a filial
nenhuma.

## O que "concessão global" muda na prática

Três gates diferentes protegem as rotas, e a diferença entre eles é onde as
surpresas moram.

### `requireRoleByMethod(viewer, operator)`

Leitura para todos os papéis, escrita a partir de `operator`. É a regra
"Visualizador não cadastra nada". `GET` e `HEAD` pedem o papel de leitura, o
resto pede o de escrita.

### `requireGlobalWrite(operator)`

Leitura livre para qualquer papel, mas a **escrita exige o papel em concessão
global**.

Protege infraestrutura sem unidade: ação em container de VPS, SSL, regras de
alerta, varredura do painel. Sem ele, um Suporte TI restrito a uma filial mandaria
parar container de uma VPS — o gate grosso só olha o maior papel do usuário,
não onde ele vale.

### `requireGlobalRole(admin)` — a consequência que surpreende

Exige `admin` em concessão global **em todos os métodos, inclusive na leitura**.

Protege `/api/servers`, `/api/users`, `/api/audit`, `/api/enroll/tokens` e
`/api/devices`.

> ⚠️ **Quem tem papel `admin` apenas numa unidade não acessa `/api/servers` nem
> `/api/users` — nem para ler.**

Isso é decisão, não efeito colateral. Cadastrar servidor entrega uma chave SSH
`root`, e gerir usuários permite se promover; nenhuma das duas coisas pertence a
uma filial. Sem o gate, o admin de uma filial listaria o parque inteiro,
cadastraria VPS com chave root e se promoveria a admin global — porque o gate
grosso olha o **maior** papel do usuário em qualquer escopo, não onde ele vale.

Se alguém precisa de verdade dessas rotas, a saída é dar-lhe concessão global. O
produto **não tem** um conceito de "admin de filial" com poderes intermediários,
e criá-lo seria trabalho de produto, não de configuração.

O frontend usa a mesma régua (`hasGlobalAdmin` em `src/lib/panels.ts`): as abas
de servidores, usuários e auditoria só aparecem para quem tem `admin` global.
Antes elas apareciam para admin de filial, que recebia `403` ao clicar.

## Sessões

Login por usuário e senha (`POST /api/auth/login`), bcrypt de custo 10, TTL de
**12 horas** — curto o bastante para uma aba esquecida não virar acesso
permanente, longo o bastante para um turno.

A sessão vive na tabela `user_sessions`, e três decisões a governam:

**Só o hash do token é gravado.** A chave primária é o SHA-256 do token, nunca o
token. Um vazamento da tabela — por backup, por réplica de leitura, por `SELECT`
de quem só deveria consultar — não pode entregar sessão ativa de ninguém.

**As concessões são resolvidas a cada requisição**, não copiadas para a sessão no
login. Congelá-las faria revogar acesso não ter efeito até a pessoa sair e
entrar de novo. O custo é uma leitura por chave primária e uma por índice, por
requisição autenticada.

**Banco indisponível é falha de autenticação, nunca sucesso.** A sessão não é
apagada nesse caso: a indisponibilidade passa e ela precisa existir quando o
banco voltar. Só `record not found` e conta desativada apagam.

Conta desativada perde a sessão na requisição seguinte, mesmo que ninguém chame a
revogação explícita.

Sessões vencidas são podadas no login seguinte, no mesmo padrão do `ticketStore`:
elas só se acumulam enquanto gente entra, então quem entra paga a conta.

## Token de máquina

`API_TOKEN` continua existindo ao lado do login, e não em vez dele. Ele serve
tráfego máquina-a-máquina — script, integração — e é tratado como **admin
global**. Trocar tudo de uma vez derrubaria as integrações instaladas.

É por isso que `VITE_API_TOKEN` no frontend é perigoso e só existe em
desenvolvimento: em build de produção o valor iria para o bundle e daria admin
global a qualquer visitante. Ver [`../frontend/README.md`](../frontend/README.md).

## Tickets de SSE

`EventSource` não permite definir cabeçalho. Mandar o token permanente na query
string o deixaria no access log do proxy e no histórico do navegador.

A saída é `POST /api/stream-ticket`: um ticket de **uso único**, válido por 30
segundos, que **carrega a sessão de quem o pediu**. O stream corre com o papel e o
alcance dessa pessoa, não com privilégio de máquina — foi assim que as rotas de
SSE deixaram de conceder admin global.

## Login: o que não vaza

A rota é pública e recebe força bruta. Duas propriedades foram construídas de
propósito e não podem ser perdidas numa refatoração:

**Custo simétrico.** Usuário inexistente compara a senha contra um hash de
descarte, para gastar o mesmo tempo. Sem isso, o relógio diria quais nomes estão
cadastrados.

**Conta desativada não responde mais rápido.** O bcrypt roda **antes** da
conferência de `Active`. Recusar a conta desativada primeiro respondia em
microssegundos enquanto as outras duas gastavam 60 a 100 ms, e esse intervalo
entrega a existência da conta. Os dois erros já colapsavam na mesma resposta
HTTP, então a ordem não muda nada para o cliente legítimo.

O limite de tentativa responde `429` **antes** de conferir a senha, o que também
tira o custo de CPU do atacante. Ver [`configuracao.md`](configuracao.md).

## Primeiro usuário

Só um admin cria outro, então instalação nova precisa de um ponto de partida.
`ADMIN_USER` e `ADMIN_PASSWORD` criam o primeiro administrador
(`internal/auth/bootstrap.go`), com estas regras:

- roda **só se a tabela de usuários estiver vazia**;
- havendo qualquer usuário, não faz nada — as variáveis podem ficar no `.env` sem
  recriar nem sobrescrever ninguém;
- senha com menos de 10 caracteres é recusada, e **nenhum usuário é criado**;
- o usuário nasce `admin` com concessão global.

Remova `ADMIN_PASSWORD` do `.env` depois do primeiro acesso.

## Auditoria

Toda escrita e todo comando remoto deixam rastro com ator, papel, alvo, unidade e
resultado. O registro fica na **fronteira** — no wrapper de rota, não em cada
handler — porque assim handler novo que esqueça de auditar não existe.

Detalhes em [`operacao.md`](operacao.md) e nos
[ADRs](adr/).

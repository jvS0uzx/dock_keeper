# Política de segurança

O DockKeeper guarda credenciais de acesso SSH a servidores e roda comandos neles.
Uma falha aqui pode virar acesso à frota inteira, então o reporte é privado.

## Versões suportadas

O projeto ainda não publica versões numeradas. Só a branch `main` recebe
correção de segurança.

| Versão | Suportada |
|---|---|
| `main` | Sim |
| Qualquer commit anterior | Não |

## Como reportar

Use o reporte privado do GitHub: aba **Security** do repositório, botão
**Report a vulnerability**, ou direto em
<https://github.com/jvS0uzx/dock_keeper/security/advisories/new>.

**Não** abra issue, não comente em pull request e não publique detalhe em outro
lugar antes da correção. Não há canal por e-mail.

Ajuda muito incluir:

- o commit da `main` em que você reproduziu;
- o componente afetado (API, frontend, agente, script de coleta, instalador);
- os passos para reproduzir e o que um atacante consegue com a falha;
- uma prova de conceito mínima, sem dado real de servidor, IP ou segredo.

A conversa acontece dentro do próprio advisory. A correção sai num pull request
que referencia o advisory, e o advisory é publicado depois que a correção está na
`main`, com crédito a quem reportou, se a pessoa quiser.

## Escopo

Dentro do escopo:

- o painel em `backend/`, incluindo o agente de estação em `backend/cmd/agent`;
- o frontend em `frontend/`;
- os scripts e instaladores em `backend/scripts/` e `backend/deploy/`.

O coletor de inventário tem repositório próprio,
[`dockkeeper_collector`](https://github.com/jvS0uzx/dockkeeper_collector). Reporte
lá o que for dele.

Fora do escopo:

- configuração insegura ligada de propósito e documentada como tal, como
  `SSH_INSECURE_HOST_KEY=true` ou `ALLOW_LEGACY_INGEST_TOKEN=true`;
- vulnerabilidade de dependência sem caminho de exploração pelo DockKeeper;
- negação de serviço por volume de tráfego.

O modelo de acesso está em [`docs/autenticacao.md`](docs/autenticacao.md), e os
riscos operacionais conhecidos, em [`docs/operacao.md`](docs/operacao.md).

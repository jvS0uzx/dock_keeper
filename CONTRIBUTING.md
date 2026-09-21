# Contribuindo com o DockKeeper

Obrigado pelo interesse. Este guia diz como subir o projeto, como rodar a suíte
inteira e quais regras um pull request precisa cumprir para ser aceito.

Vulnerabilidade **não** entra por issue nem por pull request público. Siga o
[`SECURITY.md`](SECURITY.md).

## O que há no repositório

| Pasta | Conteúdo |
|---|---|
| `backend/` | Painel em Go: API, streams SSE, coleta por SSH, workers, agente de estação em `cmd/agent` |
| `frontend/` | React + TypeScript + Vite |
| `docs/` | Documentação técnica e as decisões de arquitetura em `docs/adr/` |

O coletor de inventário por unidade mora em outro repositório,
[`dockkeeper_collector`](https://github.com/jvS0uzx/dockkeeper_collector). Mudança
no contrato de ingestão (`/api/ingest/*`, `/api/enroll`) precisa sair nos dois ao
mesmo tempo.

## Pré-requisitos

- Go na versão do `backend/go.mod`
- Node 22 e npm
- Docker, para o Postgres de desenvolvimento e o de teste
- `bash`, usado pelos testes que executam os scripts de coleta

## Subindo para desenvolver

```bash
cp .env.example .env
docker compose up -d postgres

cd backend
go run ./cmd/dockkeeper

cd ../frontend
npm install
npm run dev
```

O `README.md` explica as variáveis obrigatórias e como criar o primeiro usuário.
A lista completa de variáveis está em [`docs/configuracao.md`](docs/configuracao.md).

## Testes

Os testes de integração do backend pulam sozinhos sem `DATABASE_URL`, e pular é
exatamente o que deixa passar uma regressão de recorte por unidade. Rode contra um
Postgres descartável, nunca contra o banco que você usa no dia a dia: os testes
criam e apagam linhas.

```bash
docker run --rm -d --name dockkeeper-test-pg \
  -e POSTGRES_PASSWORD=ci -e POSTGRES_DB=dockkeeper_test \
  -p 127.0.0.1:55432:5432 postgres:15-alpine

cd backend
export DATABASE_URL="postgres://postgres:ci@127.0.0.1:55432/dockkeeper_test?sslmode=disable"
gofmt -l .
go vet ./...
go test -race -count=1 -p 1 ./...

docker stop dockkeeper-test-pg
```

`gofmt -l .` precisa sair vazio. O `-p 1` roda um pacote de cada vez, porque todos
usam o mesmo banco de teste.

```bash
cd frontend
npm run typecheck
npx oxlint
npm test
npm run build
```

`npx tsc --noEmit` não serve de verificação aqui: o `tsconfig.json` da raiz só tem
referências de projeto e o comando não checa arquivo nenhum. Use
`npm run typecheck`.

O CI roda tudo isso em todo pull request, com Postgres como serviço, mais o
`gitleaks` na árvore e no histórico.

## Dependências do CI

O workflow não leva comentário, então a versão de cada action fixada por SHA fica
registrada aqui. `.github/scripts/confere-ci-fixado.sh` roda no CI e recusa `uses:`
sem SHA de 40 caracteres e download de release sem conferência de SHA-256.

| Dependência | Versão | Fixada em |
|---|---|---|
| `actions/checkout` | v4.4.0 | `11d5960a326750d5838078e36cf38b85af677262` |
| `actions/setup-go` | v5.6.0 | `40f1582b2485089dde7abd97c1529aa768e1baff` |
| `actions/setup-node` | v4.4.0 | `49933ea5288caeca8642d1e84afbd3f7d6820020` |
| `gitleaks` (`linux_x64.tar.gz`) | 8.18.4 | SHA-256 `ba6dbb656933921c775ee5a2d1c13a91046e7952e9d919f9bac4cec61d628e7d` |

Para atualizar uma action, pegue o SHA da tag nova e troque no workflow e nesta
tabela, no mesmo commit:

```bash
gh api repos/actions/checkout/git/ref/tags/v4.4.0 --jq .object.sha
```

Se a resposta vier com `type` igual a `tag`, e não `commit`, a tag é anotada: resolva
o commit com `gh api repos/actions/checkout/git/tags/<sha> --jq .object.sha`.

Para atualizar o `gitleaks`, troque `VERSAO` e `SHA256` no workflow pelo valor da
linha `linux_x64` do arquivo `gitleaks_<versão>_checksums.txt` publicado no release.

## Regras do projeto

**Zero comentário em código.** Nenhum comentário em Go, TypeScript, CSS, shell,
PowerShell, YAML, Dockerfile ou configuração. O porquê de uma decisão vai para um
ADR em `docs/adr/`, para a mensagem de commit ou para a descrição do pull request.
Ficam de fora só as diretivas que mudam o build: `//go:embed`, shebang e
`# syntax=` do Dockerfile. A regra é conferida por teste:
`backend/internal/semcomentario` no `go test` e `frontend/src/semComentario.test.ts`
no `npm test`. Comentário novo deixa a suíte vermelha.

**Nenhum emoji em código.** Símbolo visual na interface é ícone do
`lucide-react`. Mensagem de alerta usa prefixo textual: `[CRITICO]`, `[ALERTA]`,
`[AVISO]`.

**Português do Brasil** em texto de interface, mensagem de erro da API, log e
documentação.

**Teste que falha antes.** Toda correção e todo comportamento novo entram com um
teste que falha sem a mudança e passa com ela. Cite no pull request a linha exata
da falha que o teste produzia antes. Teste que depende de rede real, de hora do dia
ou da ordem de execução não é aceito: injete relógio, servidor falso ou fixture.

**Nulo não é zero.** Métrica que a fonte não mediu é `NULL` no banco e `null` na
API, nunca `0`. Ver [`docs/metricas.md`](docs/metricas.md).

**Configuração nova** é variável de ambiente com padrão seguro, validada na
fronteira (valor inválido cai no padrão com aviso no log) e documentada em
`docs/configuracao.md` e no `.env.example`.

**Sem abstração prematura.** Três linhas parecidas são melhores que uma abstração
errada.

## Commits e pull requests

- Commits no formato Conventional Commits, em português:
  `feat(api): aceitar rede no ingest`, `fix(ssh): ...`, `docs: ...`.
- Um pull request por assunto, a partir de uma branch saída da `main`.
- Descreva o que muda, por que muda e como foi verificado.
- O CI precisa estar verde antes da revisão.

## Conduta

A participação no projeto segue o [`CODE_OF_CONDUCT.md`](CODE_OF_CONDUCT.md).

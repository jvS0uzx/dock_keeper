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
go mod download
go run ./cmd/dockkeeper

cd ../frontend
npm install
npm run dev
```

O `README.md` explica as variáveis obrigatórias e como criar o primeiro usuário.
A lista completa de variáveis está em [`docs/configuracao.md`](docs/configuracao.md).

## Testes

Os testes de integração do backend pulam sozinhos sem `DATABASE_URL`, e pular é
exatamente o que deixa passar uma regressão de recorte por unidade. Por isso o CI
liga `TEST_EXIGE_BANCO=1`: com ela, teste que precisa de banco e não o alcança
**falha** em vez de pular, e o pacote inteiro falha se não conseguir criar o
banco descartável.

Cada pacote com teste de banco cria, no `TestMain`, um banco próprio chamado
`dk_teste_<pid>_<nanos>` no mesmo servidor do `DATABASE_URL`, roda os testes nele
e o apaga no fim (`backend/internal/bancoteste`). O banco apontado na URL só serve
de porta de entrada, e o usuário precisa poder criar banco. Mesmo assim, aponte
para um Postgres descartável, nunca para o que você usa no dia a dia.

```bash
docker run --rm -d --name dockkeeper-test-pg \
  -e POSTGRES_PASSWORD=ci -e POSTGRES_DB=dockkeeper_test \
  -p 127.0.0.1:55432:5432 postgres:15-alpine

cd backend
export DATABASE_URL="postgres://postgres:ci@127.0.0.1:55432/dockkeeper_test?sslmode=disable"
export TEST_EXIGE_BANCO=1
gofmt -l .
go vet ./...
go test -race -count=1 -p 1 -timeout 25m ./...

docker stop dockkeeper-test-pg
```

`gofmt -l .` precisa sair vazio. O comando de teste é o mesmo do CI: um pacote de
cada vez (`-p 1`) e prazo explícito de 25 min (`-timeout 25m`). O pacote
`internal/api` leva de 5 a 8 min sob `-race`, perto dos 10 min padrão do
`go test`, e num runner lento a suíte morreria por tempo, não por defeito.

A sonda de MySQL e MariaDB tem um teste opcional contra containers reais, fora da
suíte padrão e do CI. Precisa de Docker e leva cerca de 1 minuto:

```bash
cd backend
TESTE_MYSQL_REAL=1 go test -run TestSondaMySQLContraBancosReais ./scripts/
```

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

O CI roda tudo isso em `ubuntu-24.04`, em todo pull request e em todo push para a
`main`, com Postgres como serviço, mais o `gitleaks` na árvore e no histórico. O
runner é fixado porque o `ubuntu-latest` muda de versão sem aviso.

## Dependências do CI

O workflow não leva comentário, então a versão de cada action fixada por SHA fica
registrada aqui. `.github/scripts/confere-ci-fixado.sh` roda no CI e recusa `uses:`
sem SHA de 40 caracteres e download de release sem conferência de SHA-256.

| Dependência | Versão | Fixada em |
|---|---|---|
| `actions/checkout` | v7.0.1 | `3d3c42e5aac5ba805825da76410c181273ba90b1` |
| `actions/setup-go` | v7.0.0 | `b7ad1dad31e06c5925ef5d2fc7ad053ef454303e` |
| `actions/setup-node` | v7.0.0 | `820762786026740c76f36085b0efc47a31fe5020` |
| `gitleaks` (`linux_x64.tar.gz`) | 8.18.4 | SHA-256 `ba6dbb656933921c775ee5a2d1c13a91046e7952e9d919f9bac4cec61d628e7d` |

Para atualizar uma action, pegue o SHA da tag nova e troque no workflow e nesta
tabela, no mesmo commit:

```bash
gh api repos/actions/checkout/git/ref/tags/v7.0.1 --jq .object.sha
```

Se a resposta vier com `type` igual a `tag`, e não `commit`, a tag é anotada: resolva
o commit com `gh api repos/actions/checkout/git/tags/<sha> --jq .object.sha`.

Para atualizar o `gitleaks`, troque `VERSAO` e `SHA256` no workflow pelo valor da
linha `linux_x64` do arquivo `gitleaks_<versão>_checksums.txt` publicado no release.

O Dependabot (`.github/dependabot.yml`) abre, uma vez por semana, pull request de
atualização para os módulos Go de `backend/`, o npm de `frontend/`, as actions e
as imagens base dos dois `Dockerfile`. O pull request de action troca só o SHA no
workflow: atualize a versão nesta tabela no mesmo pull request.

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

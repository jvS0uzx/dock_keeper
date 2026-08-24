# Frontend do DockKeeper

React 19 + TypeScript + Vite, com Tailwind, Recharts para as séries temporais e
`lucide-react` para ícones. Sem biblioteca de rotas: a navegação é estado, e as
abas visíveis dependem do papel de quem entrou.

## Desenvolvimento

```bash
npm install
npm run dev
```

O Vite serve em `http://localhost:5173`, que é o valor padrão de
`ALLOWED_ORIGINS` no backend. Suba o backend antes (`cd ../backend && go run
./cmd/vd_stats`), senão a tela de login responde erro de rede.

## Verificação

```bash
npm run typecheck   # tsc -b
npx oxlint
npx vitest run
npm run build
```

### ⚠️ `tsc --noEmit` não checa nada aqui

O `tsconfig.json` da raiz tem `"files": []` e apenas referências de projeto
(`tsconfig.app.json`, `tsconfig.node.json`). `tsc --noEmit` obedece a esse
arquivo ao pé da letra: percorre **0 dos 36 arquivos** de `src/` e sai com
sucesso, sempre.

```bash
npx tsc --noEmit --listFiles | grep -c 'src/'   # 0
```

Use `npm run typecheck`, que é `tsc -b` e resolve as referências. O `npm run
build` também usa `tsc -b`, então o build sempre foi verificado de verdade — só
o comando avulso é que não checava. Isto já passou despercebido uma vez; o CI em
`.github/workflows/ci.yml` roda `npm run typecheck` justamente por isso.

## URL da API: resolvida em tempo de execução

O Vite embute `import.meta.env.*` no bundle. Usar `VITE_API_URL` em produção
exigiria **um build por unidade instalada** — cada filial com um endereço de
painel diferente teria que compilar o próprio frontend.

`src/config.ts` resolve a URL lendo `public/config.json` antes do primeiro
render, com esta precedência:

1. **`/config.json`** — instalação empacotada; é o que o nginx da imagem serve;
2. **`VITE_API_URL`** — desenvolvimento, com o backend noutra porta;
3. **mesma origem** — proxy reverso repassando `/api`.

```json
{ "apiUrl": "https://painel.exemplo.com.br" }
```

`apiUrl` vazio significa mesma origem. Trocar o endereço da API é editar um
arquivo de uma linha, sem recompilar.

A carga acontece em `main.tsx`, **antes** do `createRoot`. Não é detalhe de
estilo: um componente que dispare requisição no `useEffect` inicial usaria a URL
antiga, e o sintoma seria uma falha de rede intermitente só na primeira carga da
página.

Ausência de `/config.json` não é erro — cai no item 2 ou 3. Falhar ali deixaria a
tela em branco por causa de um arquivo opcional.

## `VITE_API_TOKEN` só existe em desenvolvimento

```ts
export const API_TOKEN: string = import.meta.env.DEV ? import.meta.env.VITE_API_TOKEN || '' : '';
```

Isso é correção de segurança, não conveniência. O backend trata esse token como
**admin global**, acima de qualquer papel por unidade. Enquanto ele era lido sem
a guarda de `import.meta.env.DEV`, o Vite embutia o valor no bundle de produção:
o token de 64 caracteres aparecia em texto puro em `dist/assets/api-*.js` e
viajava para **todo navegador que abrisse o painel**. Quem abrisse o DevTools
saía de lá com acesso total ao cadastro de servidores e às chaves SSH.

Com a guarda, em build de produção a constante é string vazia, o ramo vira código
morto e o valor não chega ao bundle. A única entrada passa a ser o login, que
aplica papel por unidade.

O CI tem um passo dedicado a essa regressão: injeta um valor reconhecível em
`VITE_API_TOKEN`, roda o build e falha se ele aparecer em qualquer arquivo de
`dist/`.

Em desenvolvimento a variável existe só para o `npm run dev` não parar na tela de
login. Precisa bater com o `API_TOKEN` do backend.

## Estrutura

```
src/
  components/    uma tela por arquivo, mais ui/ com os primitivos
  lib/
    api.ts       cliente HTTP e SSE, e os tipos que espelham o backend
    session.ts   sessão no localStorage e o evento de sessão vencida
    panels.ts    quais abas cada papel enxerga
    metrics.ts   funções puras de formatação de métrica
    upstream.ts  ordenação dos nós do diagrama do balanceador
  config.ts      URL da API e token de desenvolvimento
```

Os testes (`vitest`) cobrem hoje apenas as funções puras de `lib/`. Não há teste
de componente nem de fluxo de login.

## Variáveis de ambiente

Copie de `.env.example`. Todas são de desenvolvimento — ver
[`../docs/configuracao.md`](../docs/configuracao.md).

| Variável | Efeito |
|---|---|
| `VITE_API_URL` | Base da API em desenvolvimento. Em produção, use `config.json` |
| `VITE_API_TOKEN` | Token de máquina, **ignorado no build de produção** |
| `VITE_TARGET_VPS_IPS` | Endereços dos backends como o Nginx os reporta em `upstream_addr`, só para ordenar o diagrama |
| `VITE_LB_IP` | IP do host que roda o Nginx balanceador, comparado com `servers.host_ip` |

# Sistema de design do frontend

Direção: **sala de controle sóbria**. O painel exibe estado de infraestrutura — a
interface informa primeiro e decora nunca. Este documento é o contrato que todas
as telas seguem; os tokens vivem em `frontend/src/index.css` (`@theme`).

## Princípios

1. **Nenhum hex inline em componente.** Toda cor vem de token (`text-text-mut`,
   `bg-ink-900`, `border-line`) ou de classe de componente (`.badge-ok`). Se uma
   cor nova parecer necessária, ela nasce no `@theme`, não no JSX.
2. **Um acento só.** Âmbar (`--color-accent`) marca ação primária, foco e a
   marca. Verde, vermelho e azul são **semânticos** (`ok`/`crit`/`info`) e nunca
   aparecem como decoração — verde significa "saudável", não "bonito".
3. **Tipografia com hierarquia, não com caixa alta.** Título usa peso e
   `tracking-tight`; caps espaçado existe apenas em `.eyebrow` (rótulo de seção)
   e no cabeçalho de tabela. `tracking-widest uppercase` fora disso é defeito.
4. **Dado técnico é mono.** IP, hostname, hash, nome de container, valor de
   métrica: `.mono-data` (Geist Mono, `tabular-nums`). Texto corrente é Inter.
5. **Profundidade por camada, não por brilho.** Superfícies `ink-950 → 700`
   sobem uma elevação por nível; borda `line`/`line-hi` e `--shadow-panel` dão o
   relevo. Glass só no `.topbar-glass`; `text-glow-*` está aposentado.
6. **Semântica de nulo preservada.** "Sem medição" continua cinza
   (`text-text-faint`) com a causa no `title`; zero medido continua `0`. O
   redesign nunca troca essa distinção — ela foi conquistada no QA.

## Tokens

| Grupo | Tokens |
|---|---|
| Superfície | `ink-950` (página) · `ink-900` (painel) · `ink-850` (campo/linha hover) · `ink-800` · `ink-750` · `ink-700` |
| Borda | `line` (padrão) · `line-hi` (hover/ativa) |
| Texto | `text-hi` (título/valor) · `text` (corpo) · `text-mut` (secundário) · `text-faint` (rótulo/desabilitado) |
| Acento | `accent` · `accent-hi` (hover) |
| Estado | `ok` · `warn` · `crit` · `info` — fundos suaves via `color-mix` nas classes `.badge-*` |
| Raio | `rounded-card` (14px, painéis) · `rounded-ctrl` (10px, controles) |
| Fonte | `font-sans` (Inter Variable) · `font-mono` (Geist Mono Variable) |

Aliases `vd-*` seguem funcionando durante a migração; código novo não os usa.

## Padrões de componente

- **Cabeçalho de página:** `.page-header` com `.page-title` + `.page-desc` à
  esquerda e ações à direita. Toda tela começa assim — sem banner, sem hero.
- **Painel:** `.panel` (com `.panel-hover` quando é clicável). Cartão de métrica:
  `.stat-card` com `.eyebrow` no rótulo e `.stat-value` no número.
- **Tabela:** `.table-base` dentro de um `.panel` com `overflow-x-auto`;
  cabeçalho já vem em caps pequeno; célula numérica ganha `.mono-data`.
- **Botões:** `.btn .btn-primary` (uma ação primária por tela), `.btn-ghost`
  para o resto, `.btn-danger` para destruição — sempre precedida de confirmação
  via `dialog`.
- **Formulários:** `.input-base` em tudo; rótulo em `.eyebrow` acima do campo.
- **Badges:** `.badge .badge-{ok|warn|crit|info|muted}`. Ponto de cor sozinho
  não basta — estado crítico carrega texto, não só cor (daltonismo).
- **Ícones:** lucide-react, `size={16}` em linha e `size={18}` em cabeçalho,
  `strokeWidth={1.75}`. Nunca emoji.
- **Motion:** `.anim-rise` no container da tela; `.stagger` em grades de cartão.
  Hover é transição de borda/fundo em 150ms — sem `scale`, sem bounce.
  `prefers-reduced-motion` já é respeitado pelas classes.

## Gráficos (recharts)

- Grid: `stroke="var(--color-line)"` com `strokeDasharray="3 3"` e opacidade 0.5.
- Eixos: `tick={{ fill: 'var(--color-text-faint)', fontSize: 11 }}`, sem linha de eixo.
- Séries: primeira `var(--color-accent)`, segunda `var(--color-info)`, terceira
  `var(--color-ok)`; área com `fillOpacity` ≤ 0.12.
- Tooltip: fundo `var(--color-ink-800)`, borda `var(--color-line-hi)`, raio 10px,
  valores em `font-mono`.
- Limiar (ex.: 70 °C) é `ReferenceLine` em `var(--color-warn)` tracejada.

## Acessibilidade (WCAG 2.1 AA)

- Foco visível global já configurado (`:focus-visible`, anel âmbar).
- Alvo de clique mínimo 36px de altura (`.btn` garante).
- Contraste: `text-mut` sobre `ink-900` ≥ 4.5:1; `text-faint` só em rótulo
  grande ou redundante.
- Navegação por teclado nos componentes `ui/Select` e `ui/DialogProvider` é
  requisito, não extra.

## Referência visual

Direção inspirada na curadoria do acervo (`Designer Fonts/20 fonts designer.md`):
densidade e tipografia de **Siteinspire/Minimal Gallery**, cartões e hierarquia
de **Bento Grids** aplicados com contenção — dashboards da Vercel e do Linear
como norte de "escuro sem neon".

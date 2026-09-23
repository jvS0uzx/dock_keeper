import { describe, expect, it } from 'vitest';

import {
  LARGURA_DO_NO,
  calcularLayout,
  idDaTabela,
  schemaPadrao,
  schemasDoEsquema,
  vizinhancaDeTabelas,
  type ColunaDoEsquema,
  type EsquemaDaBase,
  type RelacaoDoEsquema,
  type TabelaDoEsquema,
} from './erd';

const coluna = (nome: string, primaria = false): ColunaDoEsquema => ({
  nome,
  tipo: primaria ? 'bigint' : 'integer',
  nulo: !primaria,
  chave_primaria: primaria,
});

const tabela = (
  nome: string,
  colunas: ColunaDoEsquema[] = [coluna('id', true)],
  schema = 'public',
): TabelaDoEsquema => ({
  schema,
  nome,
  linhas_estimadas: null,
  tamanho_bytes: null,
  colunas_chave: colunas.filter((c) => c.chave_primaria).map((c) => c.nome),
  colunas,
});

const relacao = (
  deTabela: string,
  deColuna: string,
  paraTabela: string,
  schema = 'public',
): RelacaoDoEsquema => ({
  nome: `fk_${deTabela}_${deColuna}`,
  de_schema: schema,
  de_tabela: deTabela,
  de_colunas: [deColuna],
  para_schema: schema,
  para_tabela: paraTabela,
  para_colunas: ['id'],
  ao_apagar: 'CASCADE',
});

const esquemaDe = (
  tabelas: TabelaDoEsquema[],
  relacoes: RelacaoDoEsquema[],
  extra: Partial<EsquemaDaBase> = {},
): EsquemaDaBase => ({
  instancia_id: 1,
  base: 'dockkeeper',
  motor: 'postgres',
  coletado_em: '2026-09-21T18:00:00Z',
  suporta_diagrama: true,
  schemas: [...new Set(tabelas.map((t) => t.schema))],
  tabelas,
  relacoes,
  truncado: false,
  total_tabelas: tabelas.length,
  ...extra,
});

const nivelPorTabela = (esquema: EsquemaDaBase): Record<string, number> =>
  Object.fromEntries(calcularLayout(esquema).nos.map((no) => [no.tabela, no.nivel]));

const cadeia = esquemaDe(
  [
    tabela('itens', [coluna('id', true), coluna('pedido_id')]),
    tabela('pedidos', [coluna('id', true), coluna('cliente_id')]),
    tabela('clientes', [coluna('id', true)]),
  ],
  [relacao('itens', 'pedido_id', 'pedidos'), relacao('pedidos', 'cliente_id', 'clientes')],
);

describe('níveis do diagrama', () => {
  it('põe no nível 0 quem não aponta para ninguém e empilha quem depende', () => {
    expect(nivelPorTabela(cadeia)).toEqual({ clientes: 0, pedidos: 1, itens: 2 });
    expect(calcularLayout(cadeia).niveis).toBe(3);
  });

  it('desloca o nível seguinte para a direita, com largura fixa por nó', () => {
    const layout = calcularLayout(cadeia);
    const porNome = new Map(layout.nos.map((no) => [no.tabela, no]));
    const clientes = porNome.get('clientes');
    const pedidos = porNome.get('pedidos');
    const itens = porNome.get('itens');

    expect(clientes && pedidos && itens).toBeTruthy();
    expect(pedidos?.x).toBeGreaterThan(clientes?.x ?? 0);
    expect(itens?.x).toBeGreaterThan(pedidos?.x ?? 0);
    expect(layout.nos.every((no) => no.largura === LARGURA_DO_NO)).toBe(true);
    expect(layout.largura).toBeGreaterThan((itens?.x ?? 0) + LARGURA_DO_NO);
  });

  it('a altura do nó cresce com o número de colunas de chave', () => {
    const muitas = esquemaDe(
      [
        tabela('a', [coluna('id', true), coluna('b_id'), coluna('c_id')]),
        tabela('b'),
        tabela('c'),
      ],
      [relacao('a', 'b_id', 'b'), relacao('a', 'c_id', 'c')],
    );
    const layout = calcularLayout(muitas);
    const a = layout.nos.find((no) => no.tabela === 'a');
    const b = layout.nos.find((no) => no.tabela === 'b');

    expect(a?.colunas.map((c) => c.nome)).toEqual(['id', 'b_id', 'c_id']);
    expect(a?.colunas[0].primaria).toBe(true);
    expect(a?.colunas[1].estrangeira).toBe(true);
    expect(a?.altura).toBeGreaterThan(b?.altura ?? 0);
  });

  it('empilha sem sobrepor dentro do mesmo nível', () => {
    const layout = calcularLayout(
      esquemaDe(
        [tabela('raiz'), tabela('a', [coluna('id', true), coluna('raiz_id')]), tabela('b', [coluna('id', true), coluna('raiz_id')])],
        [relacao('a', 'raiz_id', 'raiz'), relacao('b', 'raiz_id', 'raiz')],
      ),
    );
    const doNivel = layout.nos.filter((no) => no.nivel === 1).sort((x, y) => x.y - y.y);

    expect(doNivel.map((no) => no.tabela)).toEqual(['a', 'b']);
    expect(doNivel[1].y).toBeGreaterThanOrEqual(doNivel[0].y + doNivel[0].altura);
    expect(layout.altura).toBeGreaterThan(doNivel[1].y + doNivel[1].altura);
  });
});

describe('ciclo no grafo de chaves', () => {
  const ciclo = esquemaDe(
    [
      tabela('a', [coluna('id', true), coluna('b_id')]),
      tabela('b', [coluna('id', true), coluna('c_id')]),
      tabela('c', [coluna('id', true), coluna('a_id')]),
    ],
    [relacao('a', 'b_id', 'b'), relacao('b', 'c_id', 'c'), relacao('c', 'a_id', 'a')],
  );

  it('não trava e posiciona todos os nós', () => {
    const layout = calcularLayout(ciclo);

    expect(layout.nos.map((no) => no.tabela).sort()).toEqual(['a', 'b', 'c']);
    expect(layout.nos.every((no) => Number.isFinite(no.x) && Number.isFinite(no.y))).toBe(true);
    expect(layout.arestas).toHaveLength(3);
    expect(layout.orfas).toEqual([]);
  });

  it('desenha também a aresta de volta, que sobe de nível', () => {
    const layout = calcularLayout(ciclo);
    const porId = new Map(layout.nos.map((no) => [no.id, no]));

    for (const aresta of layout.arestas) {
      expect(aresta.pontos).toHaveLength(4);
      expect(aresta.pontos.every((p) => Number.isFinite(p.x) && Number.isFinite(p.y))).toBe(true);
    }

    const deVolta = layout.arestas.filter(
      (aresta) => (porId.get(aresta.de)?.nivel ?? 0) <= (porId.get(aresta.para)?.nivel ?? 0),
    );
    expect(deVolta.length).toBeGreaterThan(0);
  });

  it('cadeia longa não estoura a pilha', () => {
    const total = 600;
    const tabelas = Array.from({ length: total }, (_, i) =>
      tabela(`t${String(i).padStart(4, '0')}`, [coluna('id', true), coluna('proximo_id')]),
    );
    const relacoes = Array.from({ length: total - 1 }, (_, i) =>
      relacao(`t${String(i).padStart(4, '0')}`, 'proximo_id', `t${String(i + 1).padStart(4, '0')}`),
    );

    const layout = calcularLayout(esquemaDe(tabelas, relacoes));

    expect(layout.nos).toHaveLength(total);
    expect(layout.niveis).toBe(total);
  });
});

describe('auto-relacionamento', () => {
  const comAlca = esquemaDe(
    [tabela('funcionarios', [coluna('id', true), coluna('chefe_id')])],
    [relacao('funcionarios', 'chefe_id', 'funcionarios')],
  );

  it('mantém a tabela desenhada e marca a aresta como alça', () => {
    const layout = calcularLayout(comAlca);

    expect(layout.nos.map((no) => no.tabela)).toEqual(['funcionarios']);
    expect(layout.orfas).toEqual([]);
    expect(layout.arestas).toHaveLength(1);
    expect(layout.arestas[0].auto).toBe(true);
    expect(layout.arestas[0].de).toBe(layout.arestas[0].para);
    expect(layout.arestas[0].rotulo).toBe('chefe_id');
  });

  it('a alça sai e volta no mesmo nó, à direita da caixa', () => {
    const layout = calcularLayout(comAlca);
    const no = layout.nos[0];
    const [inicio, , , fim] = layout.arestas[0].pontos;

    expect(inicio.x).toBe(no.x + no.largura);
    expect(fim.x).toBe(no.x + no.largura);
    expect(fim.y).toBeGreaterThan(inicio.y);
    expect(layout.arestas[0].rotuloX).toBeGreaterThan(no.x + no.largura);
    expect(layout.largura).toBeGreaterThan(no.x + no.largura);
  });

  it('o auto-relacionamento não empurra a tabela para outro nível', () => {
    expect(calcularLayout(comAlca).nos[0].nivel).toBe(0);
  });
});

describe('tabelas sem relação', () => {
  it('ficam fora do desenho e voltam na lista', () => {
    const layout = calcularLayout(
      esquemaDe(
        [...(cadeia.tabelas ?? []), tabela('logs_de_auditoria'), tabela('cache_avulso')],
        cadeia.relacoes ?? [],
      ),
    );

    expect(layout.nos.map((no) => no.tabela).sort()).toEqual(['clientes', 'itens', 'pedidos']);
    expect(layout.orfas.map((t) => t.nome)).toEqual(['cache_avulso', 'logs_de_auditoria']);
  });

  it('a raiz escolhida continua desenhada mesmo sem nenhuma relação', () => {
    const layout = calcularLayout(
      esquemaDe([...(cadeia.tabelas ?? []), tabela('logs')], cadeia.relacoes ?? []),
      { raiz: 'public.logs', saltos: 2 },
    );

    expect(layout.nos.map((no) => no.tabela)).toEqual(['logs']);
    expect(layout.orfas).toEqual([]);
    expect(layout.arestas).toEqual([]);
  });
});

describe('determinismo do layout', () => {
  it('a mesma entrada em ordem diferente produz o mesmo layout', () => {
    const tabelas = [
      tabela('itens', [coluna('id', true), coluna('pedido_id')]),
      tabela('pedidos', [coluna('id', true), coluna('cliente_id')]),
      tabela('clientes', [coluna('id', true)]),
      tabela('avulsa'),
    ];
    const relacoes = [
      relacao('itens', 'pedido_id', 'pedidos'),
      relacao('pedidos', 'cliente_id', 'clientes'),
    ];

    const direto = calcularLayout(esquemaDe(tabelas, relacoes));
    const invertido = calcularLayout(esquemaDe([...tabelas].reverse(), [...relacoes].reverse()));

    expect(JSON.stringify(invertido)).toBe(JSON.stringify(direto));
  });

  it('o ciclo também cai sempre na mesma forma', () => {
    const tabelas = [
      tabela('a', [coluna('id', true), coluna('b_id')]),
      tabela('b', [coluna('id', true), coluna('c_id')]),
      tabela('c', [coluna('id', true), coluna('a_id')]),
    ];
    const relacoes = [
      relacao('a', 'b_id', 'b'),
      relacao('b', 'c_id', 'c'),
      relacao('c', 'a_id', 'a'),
    ];

    const primeiro = calcularLayout(esquemaDe(tabelas, relacoes));
    const segundo = calcularLayout(
      esquemaDe([tabelas[2], tabelas[0], tabelas[1]], [relacoes[1], relacoes[2], relacoes[0]]),
    );

    expect(JSON.stringify(segundo)).toBe(JSON.stringify(primeiro));
  });
});

describe('modo vizinhança', () => {
  const malha = esquemaDe(
    [
      tabela('itens', [coluna('id', true), coluna('pedido_id')]),
      tabela('pedidos', [coluna('id', true), coluna('cliente_id')]),
      tabela('clientes', [coluna('id', true)]),
      tabela('enderecos', [coluna('id', true), coluna('cliente_id')]),
      tabela('avulsa'),
    ],
    [
      relacao('itens', 'pedido_id', 'pedidos'),
      relacao('pedidos', 'cliente_id', 'clientes'),
      relacao('enderecos', 'cliente_id', 'clientes'),
    ],
  );

  const nomes = (raiz: string, saltos: number): string[] =>
    calcularLayout(malha, { raiz, saltos })
      .nos.map((no) => no.tabela)
      .sort();

  it('um salto alcança as duas direções da aresta', () => {
    expect(nomes('public.pedidos', 1)).toEqual(['clientes', 'itens', 'pedidos']);
  });

  it('dois saltos alcançam o vizinho do vizinho', () => {
    expect(nomes('public.pedidos', 2)).toEqual(['clientes', 'enderecos', 'itens', 'pedidos']);
  });

  it('partindo da ponta, um salto só traz quem aponta para ela', () => {
    expect(nomes('public.clientes', 1)).toEqual(['clientes', 'enderecos', 'pedidos']);
    expect(nomes('public.clientes', 2)).toEqual(['clientes', 'enderecos', 'itens', 'pedidos']);
  });

  it('guarda a raiz e os saltos no resultado e ignora raiz inexistente', () => {
    const layout = calcularLayout(malha, { raiz: 'public.nao_existe', saltos: 2 });

    expect(layout.raiz).toBeNull();
    expect(layout.nos).toHaveLength(4);
  });

  it('prende os saltos entre um e dois', () => {
    expect(calcularLayout(malha, { raiz: 'public.pedidos', saltos: 0 }).saltos).toBe(1);
    expect(calcularLayout(malha, { raiz: 'public.pedidos', saltos: 9 }).saltos).toBe(2);
    expect(nomes('public.pedidos', 9)).toEqual(nomes('public.pedidos', 2));
  });

  it('a busca de vizinhança devolve só o alcançável', () => {
    const presentes = new Set((malha.tabelas ?? []).map((t) => idDaTabela(t.schema, t.nome)));
    const um = vizinhancaDeTabelas(presentes, malha.relacoes ?? [], 'public.itens', 1);

    expect([...um].sort()).toEqual(['public.itens', 'public.pedidos']);
    expect(vizinhancaDeTabelas(presentes, malha.relacoes ?? [], 'public.avulsa', 2)).toEqual(
      new Set(['public.avulsa']),
    );
    expect(vizinhancaDeTabelas(presentes, malha.relacoes ?? [], 'public.fantasma', 2).size).toBe(0);
  });
});

describe('schemas', () => {
  const doisSchemas = esquemaDe(
    [
      tabela('itens', [coluna('id', true), coluna('pedido_id')]),
      tabela('pedidos', [coluna('id', true)]),
      tabela('sessoes', [coluna('id', true)], 'auth'),
    ],
    [relacao('itens', 'pedido_id', 'pedidos')],
  );

  it('lista os schemas em ordem, juntando os declarados e os das tabelas', () => {
    expect(schemasDoEsquema(doisSchemas)).toEqual(['auth', 'public']);
    expect(schemasDoEsquema({ ...doisSchemas, schemas: ['relatorio'] })).toEqual([
      'auth',
      'public',
      'relatorio',
    ]);
  });

  it('o schema padrão é o que tem mais tabelas', () => {
    expect(schemaPadrao(doisSchemas)).toBe('public');
  });

  it('o layout desenha só o schema escolhido', () => {
    const layout = calcularLayout(doisSchemas, { schema: 'auth' });

    expect(layout.nos).toEqual([]);
    expect(layout.orfas.map((t) => t.nome)).toEqual(['sessoes']);
    expect(calcularLayout(doisSchemas, { schema: 'public' }).nos.map((no) => no.tabela)).toEqual([
      'itens',
      'pedidos',
    ]);
  });
});

describe('arestas', () => {
  it('saem de quem tem a chave estrangeira e levam o nome da coluna', () => {
    const layout = calcularLayout(cadeia);
    const aresta = layout.arestas.find((a) => a.rotulo === 'pedido_id');

    expect(aresta?.de).toBe('public.itens');
    expect(aresta?.para).toBe('public.pedidos');
    expect(aresta?.auto).toBe(false);
    expect(aresta?.aoApagar).toBe('CASCADE');
  });

  it('o rótulo fica entre as pontas da curva', () => {
    const layout = calcularLayout(cadeia);

    for (const aresta of layout.arestas) {
      const xs = aresta.pontos.map((p) => p.x);
      const ys = aresta.pontos.map((p) => p.y);
      expect(aresta.rotuloX).toBeGreaterThanOrEqual(Math.min(...xs));
      expect(aresta.rotuloX).toBeLessThanOrEqual(Math.max(...xs));
      expect(aresta.rotuloY).toBeGreaterThanOrEqual(Math.min(...ys));
      expect(aresta.rotuloY).toBeLessThanOrEqual(Math.max(...ys));
    }
  });

  it('ignora relação cujo alvo não veio na resposta e não repete a mesma constraint', () => {
    const layout = calcularLayout(
      esquemaDe(
        [tabela('itens', [coluna('id', true), coluna('pedido_id')]), tabela('pedidos')],
        [
          relacao('itens', 'pedido_id', 'pedidos'),
          relacao('itens', 'pedido_id', 'pedidos'),
          relacao('itens', 'fantasma_id', 'tabela_ausente'),
        ],
      ),
    );

    expect(layout.arestas).toHaveLength(1);
    expect(layout.nos).toHaveLength(2);
  });
});

describe('esquema sem tabela nenhuma', () => {
  it('devolve layout vazio sem quebrar', () => {
    const layout = calcularLayout(esquemaDe([], []));

    expect(layout.nos).toEqual([]);
    expect(layout.arestas).toEqual([]);
    expect(layout.orfas).toEqual([]);
    expect(layout.niveis).toBe(0);
    expect(layout.largura).toBeGreaterThan(0);
    expect(layout.altura).toBeGreaterThan(0);
  });

  it('aguenta resposta com listas nulas', () => {
    const cru = {
      instancia_id: 1,
      base: 'x',
      motor: 'postgres',
      coletado_em: '',
      suporta_diagrama: true,
      schemas: null,
      tabelas: null,
      relacoes: null,
      truncado: false,
      total_tabelas: 0,
    } satisfies EsquemaDaBase;

    expect(calcularLayout(cru).nos).toEqual([]);
    expect(schemasDoEsquema(cru)).toEqual([]);
    expect(schemaPadrao(cru)).toBeNull();
  });
});

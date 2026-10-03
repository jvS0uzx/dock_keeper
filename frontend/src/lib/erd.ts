export interface ColunaDoEsquema {
  nome: string;
  tipo: string;
  nulo: boolean;
  chave_primaria: boolean;
}

export interface TabelaDoEsquema {
  schema: string;
  nome: string;
  linhas_estimadas: number | null;
  tamanho_bytes: number | null;
  colunas_chave: string[] | null;
  colunas: ColunaDoEsquema[] | null;
}

export interface RelacaoDoEsquema {
  nome: string;
  de_schema: string;
  de_tabela: string;
  de_colunas: string[] | null;
  para_schema: string;
  para_tabela: string;
  para_colunas: string[] | null;
  ao_apagar: string;
}

export interface EsquemaDaBase {
  instancia_id: number;
  base: string;
  motor: string;
  coletado_em: string;
  suporta_diagrama: boolean;
  motivo?: string;
  schemas: string[] | null;
  tabelas: TabelaDoEsquema[] | null;
  relacoes: RelacaoDoEsquema[] | null;
  truncado: boolean;
  total_tabelas: number;
}

export const LARGURA_DO_NO = 208;
export const ALTURA_DO_CABECALHO = 34;
export const ALTURA_DA_COLUNA = 18;
export const RESPIRO_DO_NO = 12;
export const ESPACO_ENTRE_NIVEIS = 132;
export const ESPACO_ENTRE_NOS = 28;
export const MARGEM = 32;
export const RAIO_DA_ALCA = 40;
export const DESVIO_LATERAL = 52;
export const MAXIMO_DE_COLUNAS_NO_NO = 8;
export const SALTOS_MINIMOS = 1;
export const SALTOS_MAXIMOS = 2;

export interface PontoDoDiagrama {
  x: number;
  y: number;
}

export interface ColunaDoNo {
  nome: string;
  tipo: string;
  primaria: boolean;
  estrangeira: boolean;
}

export interface NoDoDiagrama {
  id: string;
  schema: string;
  tabela: string;
  nivel: number;
  x: number;
  y: number;
  largura: number;
  altura: number;
  colunas: ColunaDoNo[];
  colunasOcultas: number;
  linhasEstimadas: number | null;
  tamanhoBytes: number | null;
}

export interface ArestaDoDiagrama {
  id: string;
  nome: string;
  de: string;
  para: string;
  rotulo: string;
  aoApagar: string;
  auto: boolean;
  pontos: PontoDoDiagrama[];
  rotuloX: number;
  rotuloY: number;
}

export interface LayoutDoDiagrama {
  schema: string | null;
  raiz: string | null;
  saltos: number;
  nos: NoDoDiagrama[];
  arestas: ArestaDoDiagrama[];
  orfas: TabelaDoEsquema[];
  niveis: number;
  largura: number;
  altura: number;
}

export interface OpcoesDoLayout {
  schema?: string | null;
  raiz?: string | null;
  saltos?: number;
}

export const idDaTabela = (schema: string, nome: string): string => `${schema}.${nome}`;

const comparar = (a: string, b: string): number => (a < b ? -1 : a > b ? 1 : 0);

const arredondar = (valor: number): number => Math.round(valor * 10) / 10;

const idDeOrigem = (relacao: RelacaoDoEsquema): string =>
  idDaTabela(relacao.de_schema, relacao.de_tabela);

const idDeDestino = (relacao: RelacaoDoEsquema): string =>
  idDaTabela(relacao.para_schema, relacao.para_tabela);

const chaveDaRelacao = (relacao: RelacaoDoEsquema): string =>
  [
    idDeOrigem(relacao),
    idDeDestino(relacao),
    (relacao.de_colunas ?? []).join(','),
    (relacao.para_colunas ?? []).join(','),
    relacao.nome,
  ].join('|');

export const schemasDoEsquema = (esquema: EsquemaDaBase): string[] => {
  const nomes = new Set<string>();
  for (const schema of esquema.schemas ?? []) nomes.add(schema);
  for (const tabela of esquema.tabelas ?? []) nomes.add(tabela.schema);
  return [...nomes].sort(comparar);
};

export const schemaPadrao = (esquema: EsquemaDaBase): string | null => {
  const contagem = new Map<string, number>();
  for (const schema of schemasDoEsquema(esquema)) contagem.set(schema, 0);
  for (const tabela of esquema.tabelas ?? []) {
    contagem.set(tabela.schema, (contagem.get(tabela.schema) ?? 0) + 1);
  }

  let escolhido: string | null = null;
  let maior = -1;
  for (const [schema, total] of [...contagem.entries()].sort((a, b) => comparar(a[0], b[0]))) {
    if (total > maior) {
      maior = total;
      escolhido = schema;
    }
  }
  return escolhido;
};

export const vizinhancaDeTabelas = (
  presentes: Set<string>,
  relacoes: RelacaoDoEsquema[],
  raiz: string,
  saltos: number,
): Set<string> => {
  const alcancados = new Set<string>();
  if (!presentes.has(raiz)) return alcancados;

  const vizinhos = new Map<string, Set<string>>();
  const ligar = (de: string, para: string) => {
    if (!vizinhos.has(de)) vizinhos.set(de, new Set());
    vizinhos.get(de)?.add(para);
  };
  for (const relacao of relacoes) {
    const de = idDeOrigem(relacao);
    const para = idDeDestino(relacao);
    if (!presentes.has(de) || !presentes.has(para)) continue;
    ligar(de, para);
    ligar(para, de);
  }

  alcancados.add(raiz);
  let fronteira = [raiz];
  for (let salto = 0; salto < saltos; salto += 1) {
    const proxima: string[] = [];
    for (const atual of fronteira) {
      for (const vizinho of [...(vizinhos.get(atual) ?? [])].sort(comparar)) {
        if (alcancados.has(vizinho)) continue;
        alcancados.add(vizinho);
        proxima.push(vizinho);
      }
    }
    fronteira = proxima;
    if (fronteira.length === 0) break;
  }
  return alcancados;
};

const nivelDosNos = (ids: string[], saidas: Map<string, string[]>): Map<string, number> => {
  const nivel = new Map<string, number>();
  const estado = new Map<string, 'visitando' | 'pronto'>();

  for (const raiz of ids) {
    if (estado.has(raiz)) continue;
    estado.set(raiz, 'visitando');
    nivel.set(raiz, 0);
    const pilha: { id: string; cursor: number }[] = [{ id: raiz, cursor: 0 }];

    while (pilha.length > 0) {
      const topo = pilha[pilha.length - 1];
      const alvos = saidas.get(topo.id) ?? [];

      if (topo.cursor < alvos.length) {
        const alvo = alvos[topo.cursor];
        topo.cursor += 1;
        if (alvo === topo.id || !saidas.has(alvo)) continue;

        const situacao = estado.get(alvo);
        if (situacao === 'pronto') {
          nivel.set(topo.id, Math.max(nivel.get(topo.id) ?? 0, (nivel.get(alvo) ?? 0) + 1));
        } else if (situacao === undefined) {
          estado.set(alvo, 'visitando');
          nivel.set(alvo, 0);
          pilha.push({ id: alvo, cursor: 0 });
        }
        continue;
      }

      pilha.pop();
      estado.set(topo.id, 'pronto');
      const pai = pilha[pilha.length - 1];
      if (pai) {
        nivel.set(pai.id, Math.max(nivel.get(pai.id) ?? 0, (nivel.get(topo.id) ?? 0) + 1));
      }
    }
  }

  return nivel;
};

const colunasDoNo = (tabela: TabelaDoEsquema, estrangeiras: Set<string>): ColunaDoNo[] => {
  const primarias = new Set(tabela.colunas_chave ?? []);
  const doCatalogo: ColunaDoNo[] = [];
  const vistas = new Set<string>();

  for (const coluna of tabela.colunas ?? []) {
    const primaria = coluna.chave_primaria || primarias.has(coluna.nome);
    if (!primaria && !estrangeiras.has(coluna.nome)) continue;
    vistas.add(coluna.nome);
    doCatalogo.push({
      nome: coluna.nome,
      tipo: coluna.tipo,
      primaria,
      estrangeira: estrangeiras.has(coluna.nome),
    });
  }

  const faltantes = [...new Set([...primarias, ...estrangeiras])]
    .filter((nome) => !vistas.has(nome))
    .sort(comparar)
    .map((nome) => ({
      nome,
      tipo: '',
      primaria: primarias.has(nome),
      estrangeira: estrangeiras.has(nome),
    }));

  return [...doCatalogo, ...faltantes];
};

const alturaDoNo = (colunas: number): number =>
  ALTURA_DO_CABECALHO + colunas * ALTURA_DA_COLUNA + RESPIRO_DO_NO;

const pontosDaAresta = (de: NoDoDiagrama, para: NoDoDiagrama): PontoDoDiagrama[] => {
  if (de.id === para.id) {
    const direita = de.x + de.largura;
    const alto = de.y + de.altura * 0.3;
    const baixo = de.y + de.altura * 0.7;
    return [
      { x: direita, y: alto },
      { x: direita + RAIO_DA_ALCA, y: alto - RAIO_DA_ALCA * 0.5 },
      { x: direita + RAIO_DA_ALCA, y: baixo + RAIO_DA_ALCA * 0.5 },
      { x: direita, y: baixo },
    ];
  }

  const deY = de.y + de.altura / 2;
  const paraY = para.y + para.altura / 2;

  if (de.nivel === para.nivel) {
    const saida = de.x + de.largura;
    const chegada = para.x + para.largura;
    return [
      { x: saida, y: deY },
      { x: saida + DESVIO_LATERAL, y: deY },
      { x: chegada + DESVIO_LATERAL, y: paraY },
      { x: chegada, y: paraY },
    ];
  }

  const saida = de.nivel > para.nivel ? de.x : de.x + de.largura;
  const chegada = de.nivel > para.nivel ? para.x + para.largura : para.x;
  const meio = (saida + chegada) / 2;
  return [
    { x: saida, y: deY },
    { x: meio, y: deY },
    { x: meio, y: paraY },
    { x: chegada, y: paraY },
  ];
};

const meioDaCurva = (pontos: PontoDoDiagrama[]): PontoDoDiagrama => ({
  x: arredondar((pontos[0].x + 3 * pontos[1].x + 3 * pontos[2].x + pontos[3].x) / 8),
  y: arredondar((pontos[0].y + 3 * pontos[1].y + 3 * pontos[2].y + pontos[3].y) / 8),
});

export const calcularLayout = (
  esquema: EsquemaDaBase,
  opcoes: OpcoesDoLayout = {},
): LayoutDoDiagrama => {
  const schema = opcoes.schema ?? null;
  const saltos = Math.min(
    SALTOS_MAXIMOS,
    Math.max(SALTOS_MINIMOS, Math.trunc(opcoes.saltos ?? SALTOS_MINIMOS)),
  );

  const doSchema = (esquema.tabelas ?? [])
    .filter((tabela) => schema === null || tabela.schema === schema)
    .sort((a, b) => comparar(idDaTabela(a.schema, a.nome), idDaTabela(b.schema, b.nome)));

  const presentes = new Set(doSchema.map((tabela) => idDaTabela(tabela.schema, tabela.nome)));

  const unicas = new Map<string, RelacaoDoEsquema>();
  for (const relacao of esquema.relacoes ?? []) {
    if (!presentes.has(idDeOrigem(relacao)) || !presentes.has(idDeDestino(relacao))) continue;
    unicas.set(chaveDaRelacao(relacao), relacao);
  }
  const doEsquema = [...unicas.entries()]
    .sort((a, b) => comparar(a[0], b[0]))
    .map(([, relacao]) => relacao);

  const raiz = opcoes.raiz && presentes.has(opcoes.raiz) ? opcoes.raiz : null;
  const visiveis = raiz ? vizinhancaDeTabelas(presentes, doEsquema, raiz, saltos) : presentes;

  const tabelas = doSchema.filter((tabela) => visiveis.has(idDaTabela(tabela.schema, tabela.nome)));
  const relacoes = doEsquema.filter(
    (relacao) => visiveis.has(idDeOrigem(relacao)) && visiveis.has(idDeDestino(relacao)),
  );

  const grau = new Map<string, number>();
  const estrangeiras = new Map<string, Set<string>>();
  const registrar = (id: string, colunas: string[]) => {
    grau.set(id, (grau.get(id) ?? 0) + 1);
    if (!estrangeiras.has(id)) estrangeiras.set(id, new Set());
    for (const coluna of colunas) estrangeiras.get(id)?.add(coluna);
  };
  for (const relacao of relacoes) {
    registrar(idDeOrigem(relacao), relacao.de_colunas ?? []);
    registrar(idDeDestino(relacao), []);
  }

  const orfas = tabelas.filter((tabela) => {
    const id = idDaTabela(tabela.schema, tabela.nome);
    return (grau.get(id) ?? 0) === 0 && id !== raiz;
  });
  const desenhadas = tabelas.filter((tabela) => {
    const id = idDaTabela(tabela.schema, tabela.nome);
    return (grau.get(id) ?? 0) > 0 || id === raiz;
  });

  const ids = desenhadas.map((tabela) => idDaTabela(tabela.schema, tabela.nome));
  const saidas = new Map<string, string[]>(ids.map((id) => [id, []]));
  for (const relacao of relacoes) {
    saidas.get(idDeOrigem(relacao))?.push(idDeDestino(relacao));
  }
  for (const [, alvos] of saidas) alvos.sort(comparar);

  const nivel = nivelDosNos(ids, saidas);
  const pilhas = new Map<number, NoDoDiagrama[]>();
  const nos: NoDoDiagrama[] = [];

  for (const tabela of desenhadas) {
    const id = idDaTabela(tabela.schema, tabela.nome);
    const chaves = colunasDoNo(tabela, estrangeiras.get(id) ?? new Set());
    const mostradas = chaves.slice(0, MAXIMO_DE_COLUNAS_NO_NO);
    const posicao = nivel.get(id) ?? 0;
    const empilhados = pilhas.get(posicao) ?? [];
    const alturaAcumulada = empilhados.reduce(
      (total, outro) => total + outro.altura + ESPACO_ENTRE_NOS,
      0,
    );

    const no: NoDoDiagrama = {
      id,
      schema: tabela.schema,
      tabela: tabela.nome,
      nivel: posicao,
      x: MARGEM + posicao * (LARGURA_DO_NO + ESPACO_ENTRE_NIVEIS),
      y: MARGEM + alturaAcumulada,
      largura: LARGURA_DO_NO,
      altura: alturaDoNo(mostradas.length),
      colunas: mostradas,
      colunasOcultas: chaves.length - mostradas.length,
      linhasEstimadas: tabela.linhas_estimadas,
      tamanhoBytes: tabela.tamanho_bytes,
    };

    empilhados.push(no);
    pilhas.set(posicao, empilhados);
    nos.push(no);
  }

  const porId = new Map(nos.map((no) => [no.id, no]));
  const arestas: ArestaDoDiagrama[] = relacoes.flatMap((relacao, indice) => {
    const de = porId.get(idDeOrigem(relacao));
    const para = porId.get(idDeDestino(relacao));
    if (!de || !para) return [];

    const pontos = pontosDaAresta(de, para);
    const meio = meioDaCurva(pontos);
    return [
      {
        id: `${indice}-${relacao.nome}`,
        nome: relacao.nome,
        de: de.id,
        para: para.id,
        rotulo: (relacao.de_colunas ?? []).join(', '),
        aoApagar: relacao.ao_apagar,
        auto: de.id === para.id,
        pontos,
        rotuloX: meio.x,
        rotuloY: meio.y,
      },
    ];
  });

  const niveis = pilhas.size;
  const larguraDasColunas =
    niveis === 0 ? 0 : (niveis - 1) * (LARGURA_DO_NO + ESPACO_ENTRE_NIVEIS) + LARGURA_DO_NO;
  const alturaDasColunas = [...pilhas.values()].reduce((maior, pilha) => {
    const fim = pilha.reduce((total, no) => total + no.altura + ESPACO_ENTRE_NOS, 0) - ESPACO_ENTRE_NOS;
    return Math.max(maior, fim);
  }, 0);

  return {
    schema,
    raiz,
    saltos,
    nos,
    arestas,
    orfas,
    niveis,
    largura: MARGEM * 2 + larguraDasColunas + (niveis === 0 ? 0 : RAIO_DA_ALCA),
    altura: MARGEM * 2 + alturaDasColunas,
  };
};

import { describe, expect, it } from 'vitest';
import { render, screen, within } from '@testing-library/react';
import { semEspera } from '../test/usuario';

import EsquemaDiagrama from './EsquemaDiagrama';
import type { EsquemaDaBase } from '../lib/erd';

const base: EsquemaDaBase = {
  instancia_id: 1,
  base: 'dockkeeper',
  motor: 'postgres',
  coletado_em: new Date().toISOString(),
  suporta_diagrama: true,
  schemas: ['public'],
  tabelas: [
    {
      schema: 'public',
      nome: 'clientes',
      linhas_estimadas: 42,
      tamanho_bytes: 81920,
      colunas_chave: ['id'],
      colunas: [{ nome: 'id', tipo: 'bigint', nulo: false, chave_primaria: true }],
    },
    {
      schema: 'public',
      nome: 'pedidos',
      linhas_estimadas: 900,
      tamanho_bytes: 163840,
      colunas_chave: ['id'],
      colunas: [
        { nome: 'id', tipo: 'bigint', nulo: false, chave_primaria: true },
        { nome: 'cliente_id', tipo: 'bigint', nulo: false, chave_primaria: false },
      ],
    },
    {
      schema: 'public',
      nome: 'itens',
      linhas_estimadas: 5000,
      tamanho_bytes: 327680,
      colunas_chave: ['id'],
      colunas: [
        { nome: 'id', tipo: 'bigint', nulo: false, chave_primaria: true },
        { nome: 'pedido_id', tipo: 'bigint', nulo: false, chave_primaria: false },
      ],
    },
    {
      schema: 'public',
      nome: 'funcionarios',
      linhas_estimadas: null,
      tamanho_bytes: null,
      colunas_chave: ['id'],
      colunas: [
        { nome: 'id', tipo: 'bigint', nulo: false, chave_primaria: true },
        { nome: 'chefe_id', tipo: 'bigint', nulo: true, chave_primaria: false },
      ],
    },
    {
      schema: 'public',
      nome: 'logs_de_auditoria',
      linhas_estimadas: null,
      tamanho_bytes: null,
      colunas_chave: [],
      colunas: [],
    },
  ],
  relacoes: [
    {
      nome: 'fk_pedidos_cliente',
      de_schema: 'public',
      de_tabela: 'pedidos',
      de_colunas: ['cliente_id'],
      para_schema: 'public',
      para_tabela: 'clientes',
      para_colunas: ['id'],
      ao_apagar: 'CASCADE',
    },
    {
      nome: 'fk_itens_pedido',
      de_schema: 'public',
      de_tabela: 'itens',
      de_colunas: ['pedido_id'],
      para_schema: 'public',
      para_tabela: 'pedidos',
      para_colunas: ['id'],
      ao_apagar: 'CASCADE',
    },
    {
      nome: 'fk_funcionarios_chefe',
      de_schema: 'public',
      de_tabela: 'funcionarios',
      de_colunas: ['chefe_id'],
      para_schema: 'public',
      para_tabela: 'funcionarios',
      para_colunas: ['id'],
      ao_apagar: 'SET NULL',
    },
  ],
  truncado: false,
  total_tabelas: 5,
};

const comSchemaExtra = (): EsquemaDaBase => ({
  ...base,
  schemas: ['public', 'auth'],
  tabelas: [
    ...(base.tabelas ?? []),
    {
      schema: 'auth',
      nome: 'sessoes',
      linhas_estimadas: 3,
      tamanho_bytes: 4096,
      colunas_chave: ['id'],
      colunas: [{ nome: 'id', tipo: 'uuid', nulo: false, chave_primaria: true }],
    },
  ],
});

const nomesDesenhados = (): string[] =>
  screen.getAllByTestId('no-do-diagrama').map((no) => no.getAttribute('data-tabela') ?? '');

describe('diagrama de esquema', () => {
  it('desenha um nó por tabela com relação e uma aresta por chave estrangeira', () => {
    render(<EsquemaDiagrama esquema={base} />);

    expect(nomesDesenhados().sort()).toEqual([
      'public.clientes',
      'public.funcionarios',
      'public.itens',
      'public.pedidos',
    ]);
    expect(screen.getAllByTestId('aresta-do-diagrama')).toHaveLength(3);

    const tela = screen.getByTestId('tela-do-diagrama');
    expect(within(tela).getByText('clientes')).toBeTruthy();
    expect(within(tela).getAllByText('cliente_id').length).toBeGreaterThan(0);
    expect(within(tela).getAllByText('PK').length).toBeGreaterThan(0);
    expect(within(tela).getAllByText('FK').length).toBeGreaterThan(0);
  });

  it('mantém o auto-relacionamento como alça visível', () => {
    render(<EsquemaDiagrama esquema={base} />);

    const alcas = screen
      .getAllByTestId('aresta-do-diagrama')
      .filter((aresta) => aresta.getAttribute('data-auto') === 'true');

    expect(alcas).toHaveLength(1);
    expect(within(alcas[0]).getByText('chefe_id')).toBeTruthy();
  });

  it('lista ao lado a tabela sem nenhuma relação, com travessão no que não foi medido', () => {
    render(<EsquemaDiagrama esquema={base} />);

    const lado = screen.getByTestId('tabelas-sem-relacao');
    const orfas = within(lado).getAllByTestId('tabela-orfa');

    expect(orfas).toHaveLength(1);
    expect(orfas[0].textContent).toMatch(/logs_de_auditoria/);
    expect(orfas[0].textContent).toMatch(/—/);
    expect(orfas[0].textContent).not.toMatch(/\b0 B\b/);
    expect(nomesDesenhados()).not.toContain('public.logs_de_auditoria');
  });

  it('avisa quanto a coleta truncou, dizendo quantas existem e quantas desenhou', () => {
    render(<EsquemaDiagrama esquema={{ ...base, truncado: true, total_tabelas: 137 }} />);

    const aviso = screen.getByTestId('aviso-de-truncamento');
    expect(aviso.textContent).toMatch(/137/);
    expect(aviso.textContent).toMatch(/desenha 4/);
  });

  it('não mostra aviso de truncamento quando a coleta veio inteira', () => {
    render(<EsquemaDiagrama esquema={base} />);

    expect(screen.queryByTestId('aviso-de-truncamento')).toBeNull();
  });

  it('diz que a base não declara chave estrangeira em vez de mostrar tela vazia', () => {
    render(<EsquemaDiagrama esquema={{ ...base, relacoes: [] }} />);

    expect(screen.getByTestId('sem-chave-estrangeira').textContent).toMatch(
      /nenhuma chave estrangeira declarada nesta base/i,
    );
    expect(screen.queryByTestId('tela-do-diagrama')).toBeNull();
    expect(screen.getAllByTestId('tabela-listada')).toHaveLength(5);
    expect(screen.getByTestId('lista-de-tabelas').textContent).toMatch(/logs_de_auditoria/);
  });

  it('não desenha quando o motor não declara relacionamento', () => {
    render(
      <EsquemaDiagrama
        esquema={{ ...base, motor: 'mongo', suporta_diagrama: false, relacoes: [] }}
      />,
    );

    expect(screen.queryByTestId('tela-do-diagrama')).toBeNull();
    expect(screen.queryByTestId('sem-chave-estrangeira')).toBeNull();
    expect(screen.getByTestId('motor-sem-diagrama').textContent).toMatch(
      /não declara relacionamento entre tabelas/i,
    );
    expect(screen.getAllByTestId('tabela-listada')).toHaveLength(5);
  });

  it('usa o motivo do servidor quando o motor ainda não tem diagrama', () => {
    render(
      <EsquemaDiagrama
        esquema={{
          ...base,
          motor: 'mysql',
          suporta_diagrama: false,
          motivo: 'O diagrama ainda não está disponível para MySQL.',
          tabelas: [],
          relacoes: [],
        }}
      />,
    );

    expect(screen.queryByTestId('tela-do-diagrama')).toBeNull();
    expect(screen.getByTestId('motor-sem-diagrama').textContent).toBe(
      'O diagrama ainda não está disponível para MySQL.',
    );
  });

  it('a linha sem medida aparece com travessão, nunca com zero', () => {
    render(<EsquemaDiagrama esquema={{ ...base, relacoes: [] }} />);

    const linha = screen
      .getAllByTestId('tabela-listada')
      .find((tr) => tr.textContent?.includes('funcionarios'));

    expect(linha?.textContent).toMatch(/—/);
    expect(linha?.textContent).not.toMatch(/0 B/);
  });

  it('clicar num nó reduz o desenho à vizinhança e clicar de novo volta', async () => {
    const usuario = semEspera();
    render(<EsquemaDiagrama esquema={base} />);

    await usuario.click(screen.getByRole('button', { name: 'Ver a vizinhança de pedidos' }));
    expect(nomesDesenhados().sort()).toEqual(['public.clientes', 'public.itens', 'public.pedidos']);

    await usuario.click(screen.getByRole('button', { name: 'Ver a vizinhança de pedidos' }));
    expect(nomesDesenhados()).toHaveLength(4);
  });

  it('dois saltos alcançam o vizinho do vizinho', async () => {
    const usuario = semEspera();
    render(<EsquemaDiagrama esquema={base} />);

    await usuario.click(screen.getByRole('button', { name: 'Ver a vizinhança de itens' }));
    expect(nomesDesenhados().sort()).toEqual(['public.itens', 'public.pedidos']);

    await usuario.click(screen.getByRole('combobox', { name: 'Saltos a partir da tabela' }));
    await usuario.click(screen.getByRole('option', { name: '2 saltos' }));

    expect(nomesDesenhados().sort()).toEqual(['public.clientes', 'public.itens', 'public.pedidos']);
  });

  it('o seletor de schema só aparece quando há mais de um', async () => {
    const { unmount } = render(<EsquemaDiagrama esquema={base} />);
    expect(screen.queryByRole('combobox', { name: 'Schema' })).toBeNull();
    unmount();

    const usuario = semEspera();
    render(<EsquemaDiagrama esquema={comSchemaExtra()} />);
    const seletor = screen.getByRole('combobox', { name: 'Schema' });
    expect(seletor.textContent).toMatch(/public/);

    await usuario.click(seletor);
    await usuario.click(screen.getByRole('option', { name: 'auth' }));

    expect(screen.queryByTestId('tela-do-diagrama')).toBeNull();
    expect(screen.getByTestId('schema-sem-relacao').textContent).toMatch(
      /nenhuma chave estrangeira entre as tabelas deste schema/i,
    );
  });

  it('mostra a contagem do que está desenhado e fecha quando o pai pede', async () => {
    let fechou = false;
    const usuario = semEspera();
    render(<EsquemaDiagrama esquema={base} aoFechar={() => { fechou = true; }} />);

    expect(screen.getByTestId('contagem-desenhada').textContent).toMatch(/4 tabela\(s\)/);
    expect(screen.getByTestId('contagem-desenhada').textContent).toMatch(/3 relação\(ões\)/);

    await usuario.click(screen.getByRole('button', { name: /fechar/i }));
    expect(fechou).toBe(true);
  });

  it('não escreve emoji, linha decorativa nem cursor de texto na tela', () => {
    const { container } = render(<EsquemaDiagrama esquema={base} />);
    const texto = container.textContent ?? '';

    expect(texto.length).toBeGreaterThan(0);
    expect(texto).not.toMatch(/\p{Extended_Pictographic}/u);
    expect(texto).not.toMatch(/\|/);
    expect(container.innerHTML).not.toMatch(/underline|overline|line-through|text-decoration/);
  });

  it('a tela em estado vazio também não inventa símbolo', () => {
    const { container } = render(
      <EsquemaDiagrama esquema={{ ...base, suporta_diagrama: false, relacoes: [] }} />,
    );

    expect(container.textContent ?? '').not.toMatch(/\p{Extended_Pictographic}/u);
    expect(container.innerHTML).not.toMatch(/underline|overline|line-through|text-decoration/);
  });
});

import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, within } from '@testing-library/react';
import type { UserEvent } from '@testing-library/user-event';
import { semEspera } from '../test/usuario';

import BancosView from './BancosView';
import type { EsquemaDaBaseRecord, PostgresBaseRecord, PostgresInstanciaRecord } from '../lib/api';
import { SiteScopeContext, type SiteScopeState } from './ui/site-scope-context';

const { bancos, esquemaDaBase } = vi.hoisted(() => ({ bancos: vi.fn(), esquemaDaBase: vi.fn() }));

vi.mock('../lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../lib/api')>()),
  api: { bancos, esquemaDaBase },
}));

const OBSERVADO = '2026-09-21T18:00:00Z';

const base = (extra: Partial<PostgresBaseRecord>): PostgresBaseRecord => ({
  nome: 'dockkeeper',
  dono: 'postgres',
  encoding: 'UTF8',
  tamanho_bytes: 41943040,
  conexoes: 3,
  observado_em: OBSERVADO,
  ...extra,
});

const instancia = (extra: Partial<PostgresInstanciaRecord>): PostgresInstanciaRecord => ({
  id: 1,
  server_id: 's1',
  servidor_nome: 'VPS-1',
  site_id: null,
  porta: 5432,
  em_container: true,
  container_nome: 'gestao-ativos-postgres',
  motor: 'postgres',
  versao: '18.0',
  papel: 'primario',
  wal_level: 'replica',
  max_wal_senders: 10,
  archive_mode: 'off',
  estado: 'ativo',
  motivo: '',
  observado_em: OBSERVADO,
  total_bases: 1,
  tamanho_total_bytes: 41943040,
  bases: [base({})],
  ...extra,
});

const outroMotor = (extra: Partial<PostgresInstanciaRecord> = {}): PostgresInstanciaRecord =>
  instancia({
    id: 2,
    server_id: 's2',
    servidor_nome: 'VPS-2',
    motor: 'mysql',
    versao: '8.4',
    porta: 3306,
    em_container: false,
    container_nome: '',
    wal_level: '',
    max_wal_senders: null,
    archive_mode: '',
    estado: 'sem_acesso',
    motivo: 'Sem credencial para o usuário de monitoramento.',
    tamanho_total_bytes: null,
    total_bases: 1,
    bases: [base({ nome: 'loja', dono: 'loja', encoding: 'utf8mb4' })],
    ...extra,
  });

const escopo: SiteScopeState = {
  siteId: 'all',
  numericSiteId: null,
  setSiteId: vi.fn(),
  sites: [],
  siteName: () => '',
  reloadSites: vi.fn(),
};

const renderizar = () =>
  render(
    <SiteScopeContext.Provider value={escopo}>
      <BancosView />
    </SiteScopeContext.Provider>,
  );

const esquema = (extra: Partial<EsquemaDaBaseRecord> = {}): EsquemaDaBaseRecord => ({
  instancia_id: 1,
  base: 'dockkeeper',
  motor: 'postgres',
  coletado_em: OBSERVADO,
  suporta_diagrama: true,
  schemas: ['public'],
  tabelas: [
    {
      schema: 'public',
      nome: 'servers',
      linhas_estimadas: 4,
      tamanho_bytes: 81920,
      colunas_chave: ['id'],
      colunas: [{ nome: 'id', tipo: 'uuid', nulo: false, chave_primaria: true }],
    },
    {
      schema: 'public',
      nome: 'server_addresses',
      linhas_estimadas: 9,
      tamanho_bytes: 40960,
      colunas_chave: ['server_id'],
      colunas: [{ nome: 'server_id', tipo: 'uuid', nulo: false, chave_primaria: false }],
    },
  ],
  relacoes: [
    {
      nome: 'fk_server_addresses_server',
      de_schema: 'public',
      de_tabela: 'server_addresses',
      de_colunas: ['server_id'],
      para_schema: 'public',
      para_tabela: 'servers',
      para_colunas: ['id'],
      ao_apagar: 'CASCADE',
    },
  ],
  truncado: false,
  total_tabelas: 2,
  ...extra,
});

const abrirDetalhe = async (user: UserEvent, identificacao: string) =>
  user.click(await screen.findByRole('button', { name: `Ver detalhe de ${identificacao}` }));

const filtrar = async (user: UserEvent, filtro: string, opcao: string) => {
  await user.click(screen.getByRole('combobox', { name: filtro }));
  await user.click(await screen.findByRole('option', { name: opcao }));
};

const falhaDoPainel = () => new Error(JSON.stringify({ error: 'painel fora do ar' }));

beforeEach(() => {
  bancos.mockReset();
  bancos.mockResolvedValue([instancia({})]);
  esquemaDaBase.mockReset();
  esquemaDaBase.mockResolvedValue(esquema());
});

describe('BancosView — faixa de resumo', () => {
  it('conta instâncias, motores distintos, tamanho somado e quantas estão fora de ativo', async () => {
    bancos.mockResolvedValue([
      instancia({}),
      outroMotor(),
      instancia({ id: 3, servidor_nome: 'VPS-3', versao: '16.2', estado: 'inativo', motivo: 'Não responde.', total_bases: 2 }),
    ]);
    renderizar();

    await screen.findAllByTestId('instancia');
    expect(screen.getByTestId('resumo-instancias').textContent).toBe('3');
    expect(screen.getByTestId('resumo-motores').textContent).toBe('2');
    expect(screen.getByTestId('resumo-tamanho').textContent).toBe('80 MB');
    expect(screen.getByTestId('resumo-fora-de-ativo').textContent).toBe('2');
  });

  it('sem nenhum tamanho medido o somado é travessão, nunca 0 B', async () => {
    bancos.mockResolvedValue([instancia({ tamanho_total_bytes: null })]);
    renderizar();

    await screen.findByTestId('instancia');
    expect(screen.getByTestId('resumo-tamanho').textContent).toBe('—');
    expect(screen.queryByText('0 B')).toBeNull();
  });
});

describe('BancosView — tabela enxuta', () => {
  it('junta motor e versão, empilha endereço e mostra tamanho com o número de bases', async () => {
    bancos.mockResolvedValue([instancia({ total_bases: 4 })]);
    renderizar();

    const linha = within(await screen.findByTestId('instancia'));
    expect(linha.getByTestId('instancia-motor').textContent).toBe('PostgreSQL 18.0');
    expect(linha.getByText('VPS-1')).toBeTruthy();
    expect(linha.getByTestId('instancia-endereco').textContent).toBe('gestao-ativos-postgres · :5432');
    expect(linha.getByTestId('instancia-papel').textContent).toBe('Primário');
    expect(linha.getByTestId('instancia-tamanho').textContent).toBe('40 MB');
    expect(linha.getByTestId('instancia-bases').textContent).toBe('4 bases');
  });

  it('versão ausente deixa só o nome do motor, e fora de container o endereço é o servidor', async () => {
    bancos.mockResolvedValue([instancia({ versao: '', em_container: false, container_nome: '' })]);
    renderizar();

    const linha = within(await screen.findByTestId('instancia'));
    expect(linha.getByTestId('instancia-motor').textContent).toBe('PostgreSQL');
    expect(linha.getByTestId('instancia-endereco').textContent).toBe('VPS-1 · :5432');
  });

  it('instância fora de ativo mostra o motivo abaixo do estado', async () => {
    bancos.mockResolvedValue([outroMotor()]);
    renderizar();

    const estado = await screen.findByTestId('instancia-estado');
    expect(within(estado).getByText('Sem acesso')).toBeTruthy();
    expect(within(estado).getByTestId('instancia-motivo').textContent).toBe(
      'Sem credencial para o usuário de monitoramento.',
    );
  });

  it('tamanho não medido vira travessão na linha, nunca 0 B', async () => {
    bancos.mockResolvedValue([instancia({ tamanho_total_bytes: null })]);
    renderizar();

    const linha = within(await screen.findByTestId('instancia'));
    expect(linha.getByTestId('instancia-tamanho').textContent).toBe('—');
    expect(linha.queryByText('0 B')).toBeNull();
  });
});

describe('BancosView — filtros', () => {
  const tres = () => [
    instancia({}),
    outroMotor(),
    instancia({ id: 3, servidor_nome: 'VPS-3', estado: 'inativo', motivo: 'Não responde.' }),
  ];

  it('o filtro de servidor reduz a lista', async () => {
    const user = semEspera();
    bancos.mockResolvedValue(tres());
    renderizar();
    expect(await screen.findAllByTestId('instancia')).toHaveLength(3);

    await filtrar(user, 'Servidor', 'VPS-2');

    const linhas = screen.getAllByTestId('instancia');
    expect(linhas).toHaveLength(1);
    expect(within(linhas[0]).getByText('VPS-2')).toBeTruthy();
  });

  it('o filtro de motor reduz a lista', async () => {
    const user = semEspera();
    bancos.mockResolvedValue(tres());
    renderizar();
    await screen.findAllByTestId('instancia');

    await filtrar(user, 'Motor', 'mysql');

    const linhas = screen.getAllByTestId('instancia');
    expect(linhas).toHaveLength(1);
    expect(within(linhas[0]).getByTestId('instancia-motor').textContent).toBe('mysql 8.4');
    expect(screen.getByTestId('resumo-instancias').textContent).toBe('1');
  });

  it('o filtro de estado reduz a lista', async () => {
    const user = semEspera();
    bancos.mockResolvedValue(tres());
    renderizar();
    await screen.findAllByTestId('instancia');

    await filtrar(user, 'Estado', 'Ativo');

    expect(screen.getAllByTestId('instancia')).toHaveLength(1);
    expect(screen.getByTestId('resumo-fora-de-ativo').textContent).toBe('0');
  });

  it('cruzamento sem resultado não se confunde com lista vazia', async () => {
    const user = semEspera();
    bancos.mockResolvedValue([instancia({}), outroMotor()]);
    renderizar();
    await screen.findAllByTestId('instancia');

    await filtrar(user, 'Servidor', 'VPS-2');
    await filtrar(user, 'Motor', 'PostgreSQL');

    expect(screen.queryAllByTestId('instancia')).toHaveLength(0);
    expect(screen.getByText('Nenhuma instância corresponde aos filtros.')).toBeTruthy();
    expect(screen.queryByText('Nenhuma instância de banco descoberta.')).toBeNull();
  });
});

describe('BancosView — painel lateral', () => {
  it('abre ao clicar na linha, traz as três seções e fecha pelo botão', async () => {
    const user = semEspera();
    renderizar();

    expect(screen.queryByTestId('painel-detalhe')).toBeNull();
    await abrirDetalhe(user, 'VPS-1:5432');

    const painel = within(screen.getByTestId('painel-detalhe'));
    expect(painel.getByRole('heading', { name: 'Identificação' })).toBeTruthy();
    expect(painel.getByRole('heading', { name: 'Configuração' })).toBeTruthy();
    expect(painel.getByRole('heading', { name: 'Bases' })).toBeTruthy();
    expect(screen.getByRole('dialog', { name: 'Detalhe da instância VPS-1:5432' })).toBeTruthy();
    const linha = screen.getByRole('button', { name: 'Ver detalhe de VPS-1:5432' });
    expect(linha.getAttribute('aria-expanded')).toBe('true');

    await user.click(screen.getByRole('button', { name: 'Fechar detalhe' }));
    expect(screen.queryByTestId('painel-detalhe')).toBeNull();
  });

  it('fecha com Esc', async () => {
    const user = semEspera();
    renderizar();
    await abrirDetalhe(user, 'VPS-1:5432');
    expect(screen.getByTestId('painel-detalhe')).toBeTruthy();

    await user.keyboard('{Escape}');

    expect(screen.queryByTestId('painel-detalhe')).toBeNull();
  });

  it('a identificação traz servidor, endereço, porta, container e a coleta', async () => {
    const user = semEspera();
    renderizar();
    await abrirDetalhe(user, 'VPS-1:5432');

    const identificacao = within(screen.getByTestId('identificacao'));
    expect(identificacao.getByText('Servidor')).toBeTruthy();
    expect(identificacao.getByText('VPS-1')).toBeTruthy();
    expect(identificacao.getByText('gestao-ativos-postgres · :5432')).toBeTruthy();
    expect(identificacao.getByText('5432')).toBeTruthy();
    expect(identificacao.getByText('gestao-ativos-postgres')).toBeTruthy();
    expect(identificacao.getByText('Coletado')).toBeTruthy();
  });

  it('a configuração lista os pares presentes e omite os ausentes', async () => {
    const user = semEspera();
    bancos.mockResolvedValue([instancia({ archive_mode: '', max_wal_senders: null })]);
    renderizar();
    await abrirDetalhe(user, 'VPS-1:5432');

    const configuracao = within(screen.getByTestId('configuracao'));
    expect(configuracao.getByText('wal_level')).toBeTruthy();
    expect(configuracao.getByText('replica')).toBeTruthy();
    expect(configuracao.queryByText('archive_mode')).toBeNull();
    expect(configuracao.queryByText('max_wal_senders')).toBeNull();
    expect(configuracao.queryByText('—')).toBeNull();
  });

  it('motor sem parâmetros conhecidos diz isso em vez de inventar campos', async () => {
    const user = semEspera();
    bancos.mockResolvedValue([outroMotor()]);
    renderizar();
    await abrirDetalhe(user, 'VPS-2:3306');

    expect(screen.queryByTestId('configuracao')).toBeNull();
    expect(screen.getByTestId('configuracao-vazia').textContent).toContain('mysql');
  });

  it('base sem medida aparece como travessão, nunca como 0 B', async () => {
    const user = semEspera();
    bancos.mockResolvedValue([
      instancia({ bases: [base({ nome: 'sem_medida', tamanho_bytes: null, conexoes: null })] }),
    ]);
    renderizar();
    await abrirDetalhe(user, 'VPS-1:5432');

    const cartao = within(screen.getByTestId('base'));
    expect(cartao.getByTestId('base-tamanho').textContent).toBe('—');
    expect(cartao.getByTestId('base-conexoes').textContent).toBe('—');
    expect(screen.queryByText('0 B')).toBeNull();
  });

  it('instância sem base visível diz isso', async () => {
    const user = semEspera();
    bancos.mockResolvedValue([instancia({ total_bases: 0, bases: [] })]);
    renderizar();
    await abrirDetalhe(user, 'VPS-1:5432');

    expect(
      screen.getByText('Nenhuma base visível ao usuário de monitoramento nesta instância.'),
    ).toBeTruthy();
  });
});

describe('BancosView — esquema da base', () => {
  it('clicar numa base busca o esquema dela e desenha o diagrama', async () => {
    const user = semEspera();
    renderizar();
    await abrirDetalhe(user, 'VPS-1:5432');

    expect(screen.queryByTestId('painel-esquema')).toBeNull();
    await user.click(screen.getByRole('button', { name: 'Ver o esquema da base dockkeeper' }));

    expect(esquemaDaBase).toHaveBeenCalledWith(1, 'dockkeeper', expect.anything());
    expect(await screen.findByTestId('esquema-diagrama')).toBeTruthy();
    expect(screen.getByRole('dialog', { name: 'Esquema da base dockkeeper' })).toBeTruthy();
  });

  it('o diagrama fecha por Esc sem fechar o painel lateral', async () => {
    const user = semEspera();
    renderizar();
    await abrirDetalhe(user, 'VPS-1:5432');
    await user.click(screen.getByRole('button', { name: 'Ver o esquema da base dockkeeper' }));
    await screen.findByTestId('esquema-diagrama');

    await user.keyboard('{Escape}');

    expect(screen.queryByTestId('painel-esquema')).toBeNull();
    expect(screen.getByTestId('painel-detalhe')).toBeTruthy();
  });

  it('falha ao ler o esquema aparece como erro, não como diagrama vazio', async () => {
    const user = semEspera();
    esquemaDaBase.mockRejectedValue(new Error(JSON.stringify({ error: 'catálogo inacessível' })));
    renderizar();
    await abrirDetalhe(user, 'VPS-1:5432');
    await user.click(screen.getByRole('button', { name: 'Ver o esquema da base dockkeeper' }));

    expect(await screen.findByText(/catálogo inacessível/)).toBeTruthy();
    expect(screen.queryByTestId('esquema-diagrama')).toBeNull();
  });

  it('base de motor sem diagrama não é clicável e diz o porquê', async () => {
    const user = semEspera();
    bancos.mockResolvedValue([outroMotor()]);
    renderizar();
    await abrirDetalhe(user, 'VPS-2:3306');

    expect(screen.getByTestId('base-nome').textContent).toBe('loja');
    expect(screen.queryByRole('button', { name: 'Ver o esquema da base loja' })).toBeNull();
    expect(screen.getByTestId('base-sem-diagrama').textContent).toContain('mysql');
    expect(esquemaDaBase).not.toHaveBeenCalled();
  });
});

describe('BancosView — vazio não é falha', () => {
  it('lista vazia mostra o estado vazio', async () => {
    bancos.mockResolvedValue([]);
    renderizar();

    expect(await screen.findByText('Nenhuma instância de banco descoberta.')).toBeTruthy();
  });

  it('falha na primeira leitura mostra a mensagem, não o estado vazio', async () => {
    bancos.mockRejectedValue(falhaDoPainel());
    renderizar();

    expect(await screen.findByText(/painel fora do ar/)).toBeTruthy();
    expect(screen.queryByText('Nenhuma instância de banco descoberta.')).toBeNull();
  });
});

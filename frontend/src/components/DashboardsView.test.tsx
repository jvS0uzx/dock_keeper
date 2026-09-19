import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import DashboardsView from './DashboardsView';
import { DialogContext, type DialogApi } from './ui/dialog-context';
import { responder } from '../test/recharts';

vi.mock('recharts', async (importOriginal) => {
  const { comLarguraFixa } = await import('../test/recharts');
  return comLarguraFixa(await importOriginal());
});

interface Chamada {
  method: string;
  url: URL;
  body: unknown;
}

let chamadas: Chamada[];
let listagem: { status: number; corpo: unknown };
let erroDasAnotacoes: string | null;
let erroDosServidores: string | null;

const PAINEL_EXISTENTE = {
  id: 7,
  name: 'Operação',
  updated_at: '2026-09-18T10:00:00Z',
  panels: [
    { id: 1, position: 0, title: 'CPU web', server_id: 'srv-1', metric: 'cpu', range: '24h', width: 2 },
  ],
};

beforeEach(() => {
  chamadas = [];
  listagem = { status: 200, corpo: [PAINEL_EXISTENTE] };
  erroDasAnotacoes = null;
  erroDosServidores = null;
  vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input), 'http://painel');
    const method = init?.method ?? 'GET';
    const body = init?.body ? JSON.parse(String(init.body)) : undefined;
    chamadas.push({ method, url, body });

    if (url.pathname === '/api/metrics/live') {
      if (erroDosServidores) return responder(500, { error: erroDosServidores });
      return responder(200, {
        servers: [{ id: 'srv-1', name: 'web-01' }, { id: 'srv-2', name: 'db-01' }],
        containers: [],
        load_balancing: [],
      });
    }
    if (url.pathname === '/api/dashboards' && method === 'GET') return responder(listagem.status, listagem.corpo);
    if (url.pathname === '/api/dashboards' && method === 'POST') {
      return responder(201, { id: 8, updated_at: '2026-09-18T11:00:00Z', ...(body as object) });
    }
    if (url.pathname === '/api/dashboards' && (method === 'PUT' || method === 'DELETE')) {
      return responder(200, { id: 7, updated_at: '2026-09-18T11:00:00Z', ...(body as object) });
    }
    if (url.pathname === '/api/metrics/history') return responder(200, []);
    if (url.pathname === '/api/annotations') {
      if (erroDasAnotacoes) return responder(403, { error: erroDasAnotacoes });
      return responder(200, []);
    }
    return responder(404, { error: 'rota inesperada' });
  }));
});

afterEach(() => vi.unstubAllGlobals());

const renderizar = (dialogo: Partial<DialogApi> = {}) => {
  const api: DialogApi = {
    confirm: vi.fn(async () => true),
    prompt: vi.fn(async () => null),
    notify: vi.fn(),
    ...dialogo,
  };
  render(
    <DialogContext.Provider value={api}>
      <DashboardsView />
    </DialogContext.Provider>,
  );
  return api;
};

const escolher = async (user: ReturnType<typeof userEvent.setup>, rotulo: string, opcao: string) => {
  await user.click(screen.getByRole('combobox', { name: rotulo }));
  await user.click(screen.getByRole('option', { name: opcao }));
};

describe('DashboardsView', () => {
  it('mostra o painel existente com a largura de cada gráfico', async () => {
    renderizar();

    const cartao = await screen.findByTestId('grafico-1');
    expect(within(cartao).getByText('CPU web')).toBeTruthy();
    expect(cartao.className).toContain('lg:col-span-2');
  });

  it('cria um painel com dois gráficos no formato do contrato', async () => {
    const user = userEvent.setup();
    renderizar({ prompt: vi.fn(async () => 'Rede') });
    await screen.findByTestId('grafico-1');

    await user.click(screen.getByRole('button', { name: 'Novo painel' }));
    await user.click(await screen.findByRole('button', { name: 'Adicionar gráfico' }));
    await user.click(screen.getByRole('button', { name: 'Adicionar gráfico' }));

    await user.type(screen.getByLabelText('Título do gráfico 1'), 'Entrada');
    await escolher(user, 'Métrica do gráfico 1', 'Rede RX');

    await user.type(screen.getByLabelText('Título do gráfico 2'), 'CPU banco');
    await escolher(user, 'Servidor do gráfico 2', 'db-01');
    await escolher(user, 'Janela do gráfico 2', '7d');
    await user.click(within(screen.getByTestId('editor-grafico-2')).getByRole('button', { name: '2 colunas' }));

    await user.click(screen.getByRole('button', { name: 'Salvar painel' }));

    await waitFor(() => expect(chamadas.some((c) => c.method === 'POST')).toBe(true));
    const criacao = chamadas.find((c) => c.method === 'POST');
    expect(criacao?.url.pathname).toBe('/api/dashboards');
    expect(criacao?.body).toEqual({
      name: 'Rede',
      panels: [
        { title: 'Entrada', server_id: 'srv-1', metric: 'net_rx', range: '24h', width: 1 },
        { title: 'CPU banco', server_id: 'srv-2', metric: 'cpu', range: '7d', width: 2 },
      ],
    });
  }, 15000);

  it('renomeia mantendo os gráficos', async () => {
    const user = userEvent.setup();
    renderizar({ prompt: vi.fn(async () => 'Operação diária') });
    await screen.findByTestId('grafico-1');

    await user.click(screen.getByRole('button', { name: 'Renomear' }));

    await waitFor(() => expect(chamadas.some((c) => c.method === 'PUT')).toBe(true));
    const edicao = chamadas.find((c) => c.method === 'PUT');
    expect(edicao?.url.search).toBe('?id=7');
    expect(edicao?.body).toEqual({
      name: 'Operação diária',
      panels: [{ title: 'CPU web', server_id: 'srv-1', metric: 'cpu', range: '24h', width: 2 }],
    });
  });

  it('apaga com confirmação', async () => {
    const user = userEvent.setup();
    const dialogo = renderizar();
    await screen.findByTestId('grafico-1');

    await user.click(screen.getByRole('button', { name: 'Apagar' }));

    expect(dialogo.confirm).toHaveBeenCalledWith(expect.objectContaining({ danger: true }));
    await waitFor(() => expect(chamadas.some((c) => c.method === 'DELETE')).toBe(true));
    expect(chamadas.find((c) => c.method === 'DELETE')?.url.search).toBe('?id=7');
  });

  it('não apaga quando a confirmação é negada', async () => {
    const user = userEvent.setup();
    renderizar({ confirm: vi.fn(async () => false) });
    await screen.findByTestId('grafico-1');

    await user.click(screen.getByRole('button', { name: 'Apagar' }));

    expect(chamadas.some((c) => c.method === 'DELETE')).toBe(false);
  });

  it('sessão de máquina vê a mensagem do painel, sem quebrar a tela', async () => {
    listagem = { status: 403, corpo: { error: 'painéis exigem sessão de usuário' } };
    renderizar();

    expect(await screen.findByText('painéis exigem sessão de usuário')).toBeTruthy();
    expect(screen.getByRole('heading', { name: 'Painéis' })).toBeTruthy();
  });
});

describe('DashboardsView — falha não vira vazio', () => {
  it('anotações recusadas aparecem como aviso no gráfico', async () => {
    erroDasAnotacoes = 'anotações exigem sessão de usuário';
    renderizar();

    const cartao = await screen.findByTestId('grafico-1');
    expect(await within(cartao).findByText(/anotações exigem sessão de usuário/)).toBeTruthy();
  });

  it('falha ao listar servidores não vira "cadastre um servidor"', async () => {
    erroDosServidores = 'banco indisponível';
    const user = userEvent.setup();
    renderizar({ prompt: vi.fn(async () => 'Rede') });
    await screen.findByTestId('grafico-1');

    await user.click(screen.getByRole('button', { name: 'Novo painel' }));

    expect(await screen.findByText(/banco indisponível/)).toBeTruthy();
    expect(screen.queryByText(/Cadastre um servidor/)).toBeNull();
  });
});

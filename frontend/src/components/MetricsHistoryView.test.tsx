import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import MetricsHistoryView from './MetricsHistoryView';
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
let anotacoes: unknown[];
let erroDoHistorico: string | null;
let erroDasAnotacoes: string | null;
let erroDosServidores: string | null;

const minutosAtras = (m: number) => new Date(Date.now() - m * 60 * 1000);
const localDe = (d: Date) => new Date(d.getTime() - d.getTimezoneOffset() * 60000).toISOString().slice(0, 16);

beforeEach(() => {
  chamadas = [];
  anotacoes = [];
  erroDoHistorico = null;
  erroDasAnotacoes = null;
  erroDosServidores = null;
  vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input), 'http://painel');
    const method = init?.method ?? 'GET';
    const body = init?.body ? JSON.parse(String(init.body)) : undefined;
    chamadas.push({ method, url, body });

    if (url.pathname === '/api/metrics/live') {
      if (erroDosServidores) return responder(500, { error: erroDosServidores });
      return responder(200, { servers: [{ id: 'srv-1', name: 'estacao-01' }], containers: [], load_balancing: [] });
    }
    if (url.pathname === '/api/metrics/history') {
      if (erroDoHistorico && url.searchParams.has('from')) return responder(400, { error: erroDoHistorico });
      return responder(200, [50, 30, 10].map((m, i) => ({ ts: minutosAtras(m).toISOString(), value: 10 + i })));
    }
    if (url.pathname === '/api/annotations' && method === 'GET') {
      if (erroDasAnotacoes) return responder(403, { error: erroDasAnotacoes });
      return responder(200, anotacoes);
    }
    if (url.pathname === '/api/annotations' && method === 'POST') {
      const criada = { id: anotacoes.length + 1, author: 'pessoa', created_at: new Date().toISOString(), ...(body as object) };
      anotacoes = [...anotacoes, criada];
      return responder(201, criada);
    }
    return responder(404, { error: 'rota inesperada' });
  }));
});

afterEach(() => vi.unstubAllGlobals());

const renderizar = () => {
  const dialogo: DialogApi = { confirm: vi.fn(async () => true), prompt: vi.fn(), notify: vi.fn() };
  render(
    <DialogContext.Provider value={dialogo}>
      <MetricsHistoryView />
    </DialogContext.Provider>,
  );
};

const consultasDoHistorico = () => chamadas.filter((c) => c.url.pathname === '/api/metrics/history');
const ultimaConsulta = () => consultasDoHistorico().at(-1)!.url.searchParams;

describe('MetricsHistoryView — janelas', () => {
  it('oferece 30d e 90d e manda como range', async () => {
    const user = userEvent.setup();
    renderizar();
    await waitFor(() => expect(consultasDoHistorico().length).toBeGreaterThan(0));
    expect(ultimaConsulta().get('range')).toBe('1h');

    await user.click(screen.getByRole('button', { name: '30d' }));
    await waitFor(() => expect(ultimaConsulta().get('range')).toBe('30d'));

    await user.click(screen.getByRole('button', { name: '90d' }));
    await waitFor(() => expect(ultimaConsulta().get('range')).toBe('90d'));
  }, 15000);

  it('período personalizado manda from e to, nunca junto de range', async () => {
    const user = userEvent.setup();
    renderizar();
    await waitFor(() => expect(consultasDoHistorico().length).toBeGreaterThan(0));

    await user.click(screen.getByRole('button', { name: 'Personalizado' }));
    fireEvent.change(screen.getByLabelText('De'), { target: { value: '2026-09-01T08:00' } });
    fireEvent.change(screen.getByLabelText('Até'), { target: { value: '2026-09-03T18:30' } });
    await user.click(screen.getByRole('button', { name: 'Aplicar' }));

    await waitFor(() => expect(ultimaConsulta().has('from')).toBe(true));
    expect(ultimaConsulta().get('from')).toBe(new Date('2026-09-01T08:00').toISOString());
    expect(ultimaConsulta().get('to')).toBe(new Date('2026-09-03T18:30').toISOString());
    expect(consultasDoHistorico().some((c) => c.url.searchParams.has('from') && c.url.searchParams.has('range'))).toBe(false);
  });

  it('mostra a mensagem do painel quando o período é recusado', async () => {
    erroDoHistorico = 'o período personalizado não pode passar de 400 dias';
    const user = userEvent.setup();
    renderizar();
    await waitFor(() => expect(consultasDoHistorico().length).toBeGreaterThan(0));

    await user.click(screen.getByRole('button', { name: 'Personalizado' }));
    fireEvent.change(screen.getByLabelText('De'), { target: { value: '2020-01-01T00:00' } });
    await user.click(screen.getByRole('button', { name: 'Aplicar' }));

    expect(await screen.findByText('o período personalizado não pode passar de 400 dias')).toBeTruthy();
  });
});

describe('MetricsHistoryView — anotações', () => {
  it('anotação criada aparece como marcador no gráfico', async () => {
    const user = userEvent.setup();
    renderizar();
    await waitFor(() => expect(document.querySelector('.recharts-area')).not.toBeNull());
    expect(document.querySelector('.recharts-reference-line')).toBeNull();

    const quando = minutosAtras(30);
    await user.type(screen.getByLabelText('Texto da anotação'), 'deploy 2.3');
    fireEvent.change(screen.getByLabelText('Horário da anotação'), { target: { value: localDe(quando) } });
    await user.click(screen.getByRole('button', { name: 'Anotar' }));

    const criacao = chamadas.find((c) => c.url.pathname === '/api/annotations' && c.method === 'POST');
    expect(criacao?.body).toEqual({ server_id: 'srv-1', at: new Date(localDe(quando)).toISOString(), text: 'deploy 2.3' });

    await waitFor(() => expect(document.querySelector('.recharts-reference-line')).not.toBeNull());
    const marcador = document.querySelector('.recharts-reference-line title');
    expect(marcador?.textContent).toContain('deploy 2.3');
  });

  it('anotação global vai com server_id nulo', { timeout: 15000 }, async () => {
    const user = userEvent.setup();
    renderizar();
    await waitFor(() => expect(consultasDoHistorico().length).toBeGreaterThan(0));

    await user.type(screen.getByLabelText('Texto da anotação'), 'janela de manutenção');
    await user.click(screen.getByRole('checkbox', { name: 'Global' }));
    await user.click(screen.getByRole('button', { name: 'Anotar' }));

    await waitFor(() => {
      const criacao = chamadas.find((c) => c.url.pathname === '/api/annotations' && c.method === 'POST');
      expect(criacao?.body).toMatchObject({ server_id: null, text: 'janela de manutenção' });
    });
  });
});

describe('MetricsHistoryView — falha não vira vazio', () => {
  it('leitura de anotações recusada mostra a mensagem e o gráfico segue sem marcadores', async () => {
    erroDasAnotacoes = 'anotações exigem sessão de usuário';
    renderizar();

    expect(await screen.findByText('anotações exigem sessão de usuário')).toBeTruthy();
    expect(screen.queryByText(/Nenhuma anotação neste período/)).toBeNull();
    await waitFor(() => expect(document.querySelector('.recharts-area')).not.toBeNull());
    expect(document.querySelector('.recharts-reference-line')).toBeNull();
  });

  it('falha ao listar servidores mostra a mensagem, não "nenhum servidor"', async () => {
    erroDosServidores = 'banco indisponível';
    renderizar();

    expect(await screen.findByText('banco indisponível')).toBeTruthy();
  });
});

describe('MetricsHistoryView — latência', () => {
  it('consulta o histórico de latência com metric=rtt', async () => {
    const user = userEvent.setup();
    renderizar();
    await waitFor(() => expect(consultasDoHistorico().length).toBeGreaterThan(0));

    await user.click(screen.getByRole('combobox', { name: 'Métrica' }));
    await user.click(screen.getByRole('option', { name: 'Latência (ms)' }));

    await waitFor(() => expect(ultimaConsulta().get('metric')).toBe('rtt'));
  });
});

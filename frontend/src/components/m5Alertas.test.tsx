import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, within } from '@testing-library/react';
import { semEspera } from '../test/usuario';

import AlertsView from './AlertsView';
import LogsView from './LogsView';
import Sidebar from './Sidebar';
import { DialogContext, type DialogApi } from './ui/dialog-context';
import { SessionContext, type SessionState } from './ui/session-context';
import { SiteScopeContext, type SiteScopeState } from './ui/site-scope-context';
import { podeOperarNaUnidade } from '../lib/session';

const base = {
  id: 1,
  key: 'cpu:srv-1',
  severity: 'critical',
  text: 'CPU acima de 90% em vps-loja',
  status: 'open',
  server_id: '5293e967-0952-4ea3-a4a6-20d0b1442053',
  site_id: 3,
  rule_id: 7,
  created_at: '2026-09-18T12:00:00Z',
  acked_at: null,
  acked_by: null,
  resolved_at: null,
  delivery: 'enviado',
  attempts: 1,
  next_attempt_at: null,
  last_attempt_at: null,
  last_error: '',
  renotify_count: 0,
  last_notified_at: null,
  last_seen_at: null,
  server_name: null,
  site_name: null,
};

const api = vi.hoisted(() => ({
  alerts: vi.fn(),
  alertsSummary: vi.fn(),
  ackAlert: vi.fn(),
  resolveAlert: vi.fn(),
  liveMetrics: vi.fn(),
  searchLogs: vi.fn(),
}));

vi.mock('../lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../lib/api')>()),
  api,
}));

const dialogo: DialogApi = { confirm: vi.fn(), prompt: vi.fn(), notify: vi.fn() };

const admin: SessionState = {
  username: 'p',
  role: 'admin',
  accesses: [{ site_id: null, role: 'admin' }],
  isToken: false,
  logout: vi.fn(),
};

const escopoBase: SiteScopeState = {
  siteId: 'all',
  numericSiteId: null,
  setSiteId: vi.fn(),
  sites: [{ id: 3, name: 'Filial Norte', code: 'norte' }] as SiteScopeState['sites'],
  siteName: (id) => (id === 3 ? 'Filial Norte' : '—'),
  reloadSites: vi.fn(),
};

const renderizar = (tela: React.ReactElement, sessao = admin, escopo = escopoBase) =>
  render(
    <SessionContext.Provider value={sessao}>
      <SiteScopeContext.Provider value={escopo}>
        <DialogContext.Provider value={dialogo}>{tela}</DialogContext.Provider>
      </SiteScopeContext.Provider>
    </SessionContext.Provider>,
  );

const itemDe = async (texto: string) => {
  const item = (await screen.findByText(texto)).closest('li');
  if (!item) throw new Error(`item de ${texto} não encontrado`);
  return within(item);
};

beforeEach(() => {
  api.alerts.mockReset().mockResolvedValue([base]);
  api.alertsSummary.mockReset().mockResolvedValue({ open: 1, acked: 0, falhou: 0 });
  api.ackAlert.mockReset().mockResolvedValue({ ...base, status: 'acked' });
  api.resolveAlert.mockReset().mockResolvedValue({ ...base, status: 'resolved' });
  api.liveMetrics.mockReset().mockResolvedValue({ servers: [], containers: [], load_balancing: [] });
  api.searchLogs.mockReset().mockResolvedValue([]);
  (dialogo.confirm as ReturnType<typeof vi.fn>).mockReset().mockResolvedValue(true);
  (dialogo.notify as ReturnType<typeof vi.fn>).mockReset();
});

describe('filtro de unidade da barra lateral', () => {
  it('Alertas pede só a unidade escolhida', async () => {
    renderizar(<AlertsView />, admin, { ...escopoBase, siteId: '3', numericSiteId: 3 });
    await screen.findByText(base.text);
    expect(api.alerts).toHaveBeenCalledWith({ status: 'open', site_id: 3 }, expect.anything());
  });

  it('sem unidade escolhida, Alertas não manda site_id', async () => {
    renderizar(<AlertsView />);
    await screen.findByText(base.text);
    expect(api.alerts).toHaveBeenCalledWith({ status: 'open' }, expect.anything());
  });

  it('o contador do menu segue a unidade escolhida', async () => {
    renderizar(
      <Sidebar activeTab="alerts" setActiveTab={vi.fn()} panel="suporte" setPanel={vi.fn()} />,
      admin,
      { ...escopoBase, siteId: '3', numericSiteId: 3 },
    );
    await vi.waitFor(() => expect(api.alertsSummary).toHaveBeenCalledWith(3));
  });

  it('Logs busca só na unidade escolhida', async () => {
    const usuario = semEspera();
    renderizar(<LogsView />, admin, { ...escopoBase, siteId: '3', numericSiteId: 3 });
    await usuario.click(await screen.findByRole('button', { name: /buscar/i }));
    await vi.waitFor(() =>
      expect(api.searchLogs).toHaveBeenCalledWith(expect.objectContaining({ site_id: '3' })),
    );
  });
});

describe('origem do alerta', () => {
  it('mostra o nome do servidor e da unidade, não o UUID', async () => {
    api.alerts.mockResolvedValue([{ ...base, server_name: 'VPS-1', site_name: 'Filial Norte' }]);
    renderizar(<AlertsView />);
    const item = await itemDe(base.text);
    expect(item.getByText(/VPS-1 · Filial Norte/)).toBeTruthy();
    expect(item.queryByText(/5293e967/)).toBeNull();
  });

  it('cai no UUID quando o nome vem nulo, e nunca escreve "null"', async () => {
    api.alerts.mockResolvedValue([{ ...base, server_name: null, site_name: null, site_id: null }]);
    renderizar(<AlertsView />);
    const item = await itemDe(base.text);
    expect(item.getByText(/5293e967/)).toBeTruthy();
    expect(item.queryByText(/null/)).toBeNull();
  });
});

describe('ciclo do alerta', () => {
  it('mostra quantas vezes renotificou e há quanto tempo', async () => {
    api.alerts.mockResolvedValue([
      { ...base, renotify_count: 3, last_notified_at: new Date(Date.now() - 5 * 60000).toISOString() },
    ]);
    renderizar(<AlertsView />);
    const item = await itemDe(base.text);
    expect(item.getByText(/renotificado 3 vezes, última há 5min/)).toBeTruthy();
  });

  it('sem os campos novos, a linha de renotificação não aparece e nada quebra', async () => {
    renderizar(<AlertsView />);
    const item = await itemDe(base.text);
    expect(item.queryByText(/renotificado/)).toBeNull();
  });
});

describe('reconhecer e resolver só para quem opera', () => {
  const viewer: SessionState = { ...admin, role: 'viewer', accesses: [{ site_id: 3, role: 'viewer' }] };
  const operadorDaFilial: SessionState = {
    ...admin,
    role: 'operator',
    accesses: [{ site_id: 3, role: 'operator' }],
  };

  it('viewer não vê os botões', async () => {
    renderizar(<AlertsView />, viewer);
    const item = await itemDe(base.text);
    expect(item.queryByRole('button', { name: 'Reconhecer' })).toBeNull();
    expect(item.queryByRole('button', { name: 'Resolver' })).toBeNull();
  });

  it('operador da unidade vê; em alerta de outra unidade, não', async () => {
    api.alerts.mockResolvedValue([base, { ...base, id: 2, text: 'Disco cheio em outra filial', site_id: 8 }]);
    renderizar(<AlertsView />, operadorDaFilial);
    expect((await itemDe(base.text)).getByRole('button', { name: 'Reconhecer' })).toBeTruthy();
    expect((await itemDe('Disco cheio em outra filial')).queryByRole('button', { name: 'Reconhecer' })).toBeNull();
  });

  it('mostra o erro do backend quando ele recusa', async () => {
    const usuario = semEspera();
    api.ackAlert.mockRejectedValue(new Error('{"error":"reconhecer alerta exige papel de operador"}'));
    renderizar(<AlertsView />);
    await usuario.click((await itemDe(base.text)).getByRole('button', { name: 'Reconhecer' }));
    await vi.waitFor(() =>
      expect(dialogo.notify).toHaveBeenCalledWith('reconhecer alerta exige papel de operador', 'error'),
    );
  });

  it('erro que não é JSON não derruba a tela', async () => {
    const usuario = semEspera();
    api.ackAlert.mockRejectedValue(new Error('502 Bad Gateway'));
    renderizar(<AlertsView />);
    await usuario.click((await itemDe(base.text)).getByRole('button', { name: 'Reconhecer' }));
    await vi.waitFor(() => expect(dialogo.notify).toHaveBeenCalledWith('Falha ao reconhecer.', 'error'));
  });

  it('papel na unidade: global vale em toda unidade, local só na própria', () => {
    expect(podeOperarNaUnidade([{ site_id: null, role: 'operator' }], 8)).toBe(true);
    expect(podeOperarNaUnidade([{ site_id: 3, role: 'operator' }], 3)).toBe(true);
    expect(podeOperarNaUnidade([{ site_id: 3, role: 'operator' }], 8)).toBe(false);
    expect(podeOperarNaUnidade([{ site_id: 3, role: 'operator' }], null)).toBe(false);
    expect(podeOperarNaUnidade([{ site_id: 3, role: 'viewer' }], 3)).toBe(false);
  });
});

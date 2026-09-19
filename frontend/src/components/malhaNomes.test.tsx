import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import Dashboard from './Dashboard';
import { DialogContext, type DialogApi } from './ui/dialog-context';
import { NavigationContext } from './ui/navigation-context';
import { SessionContext, type SessionState } from './ui/session-context';
import { SiteScopeContext, type SiteScopeState } from './ui/site-scope-context';

const base = {
  uptime: 10,
  disk_used: 1,
  disk_total: 2,
  mem_used: 1,
  mem_total: 2,
  cpu: 10,
  load1: 0.5,
  online: true,
  ssh_handshake_ms: null,
  kind: 'ssh',
  site_id: null,
  os: '',
  platform: '',
  arch: '',
  last_user: '',
  agent_version: '',
  temperature_c: null,
  collect_nginx: false,
  net_rx_bps: null,
  net_tx_bps: null,
  rtt_ms: null,
};

const servidores = [
  { ...base, id: 'lb', name: 'balanceador', host_ip: '10.0.0.9', collect_nginx: true },
  { ...base, id: 'a', name: 'vps-app', host_ip: '10.0.0.1' },
  { ...base, id: 'b', name: 'vps-banco', host_ip: '10.0.0.2' },
];

const trafego = [
  { upstream_addr: '10.0.0.1:80', server_name: 'app.exemplo', status: '200', requests_count: 12, server_id: 'lb' },
  { upstream_addr: '10.0.0.2:80', server_name: 'app.exemplo', status: '200', requests_count: 7, server_id: 'lb' },
  { upstream_addr: '198.51.100.9:80', server_name: 'app.exemplo', status: '200', requests_count: 3, server_id: 'lb' },
];

const api = vi.hoisted(() => ({
  liveMetrics: vi.fn(),
  servers: vi.fn(),
  history: vi.fn(),
  annotations: vi.fn(),
  readiness: vi.fn(),
}));

vi.mock('../lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../lib/api')>()),
  api,
  openStream: () => Promise.reject(new Error('sem stream no teste')),
}));

beforeEach(() => {
  api.liveMetrics.mockReset().mockResolvedValue({
    servers: servidores,
    containers: [],
    load_balancing: trafego,
  });
  api.servers.mockReset().mockResolvedValue([]);
  api.history.mockReset().mockResolvedValue([]);
  api.annotations.mockReset().mockResolvedValue([]);
  api.readiness.mockReset().mockResolvedValue({ status: 'ok' });
});

const sessao: SessionState = {
  username: 'p',
  role: 'admin',
  accesses: [{ site_id: null, role: 'admin' }],
  isToken: false,
  logout: vi.fn(),
};

const dialogo: DialogApi = { confirm: vi.fn(), prompt: vi.fn(), notify: vi.fn() };

const escopo: SiteScopeState = {
  siteId: 'all',
  setSiteId: vi.fn(),
  numericSiteId: null,
  sites: [],
  siteName: () => 'Matriz',
  reloadSites: vi.fn(),
  sitesError: null,
};

const renderizar = () =>
  render(
    <SessionContext.Provider value={sessao}>
      <SiteScopeContext.Provider value={escopo}>
        <NavigationContext.Provider value={{ openSite: vi.fn(), openMachine: vi.fn(), goBack: vi.fn() }}>
          <DialogContext.Provider value={dialogo}>
            <Dashboard />
          </DialogContext.Provider>
        </NavigationContext.Provider>
      </SiteScopeContext.Provider>
    </SessionContext.Provider>,
  );

describe('malha de roteamento — nome do servidor cadastrado', () => {
  it('mostra o nome cadastrado em cada caixa da malha', async () => {
    renderizar();

    expect((await screen.findAllByText('vps-app')).length).toBeGreaterThan(0);
    expect(screen.getAllByText('vps-banco').length).toBeGreaterThan(0);
    expect(screen.getAllByText('balanceador').length).toBeGreaterThan(0);
  });

  it('upstream sem cadastro aparece com o endereço, nunca como Node N', async () => {
    renderizar();

    await screen.findAllByText('vps-app');
    expect(screen.getAllByText('198.51.100.9:80').length).toBeGreaterThan(0);
    expect(screen.queryAllByText(/^Node \d/)).toHaveLength(0);
  });

  it('a tabela por sistema usa o nome cadastrado no cabeçalho', async () => {
    renderizar();

    const filtro = await screen.findByLabelText('Filtrar por IP');
    await userEvent.click(filtro);
    await userEvent.click(await screen.findByRole('option', { name: /balanceador/ }));

    const tabela = await screen.findByRole('table');
    expect(within(tabela).getByRole('columnheader', { name: 'vps-app' })).toBeTruthy();
    expect(within(tabela).getByRole('columnheader', { name: 'vps-banco' })).toBeTruthy();
    expect(within(tabela).queryByRole('columnheader', { name: /^Node \d/ })).toBeNull();
  });
});

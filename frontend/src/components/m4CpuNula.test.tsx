import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, within } from '@testing-library/react';

import Dashboard from './Dashboard';
import ServersView from './ServersView';
import StationsView from './StationsView';
import { DialogContext, type DialogApi } from './ui/dialog-context';
import { NavigationContext } from './ui/navigation-context';
import { SessionContext, type SessionState } from './ui/session-context';
import { SiteScopeContext, type SiteScopeState } from './ui/site-scope-context';

const base = {
  host_ip: '192.0.2.1',
  uptime: 10,
  disk_used: 1,
  disk_total: 2,
  mem_used: 1,
  mem_total: 2,
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
  { ...base, id: 'a', name: 'vps-medida', cpu: 40, load1: 1.25 },
  { ...base, id: 'b', name: 'vps-sem-medida', cpu: null, load1: null },
  { ...base, id: 'c', name: 'vps-ociosa', cpu: 0, load1: 0 },
];

const api = vi.hoisted(() => ({
  liveMetrics: vi.fn(),
  servers: vi.fn(),
  sites: vi.fn(),
  networkHosts: vi.fn(),
  history: vi.fn(),
  alertRules: vi.fn(),
  sslDomains: vi.fn(),
  searchLogs: vi.fn(),
  annotations: vi.fn(),
}));

vi.mock('../lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../lib/api')>()),
  api,
  openStream: () => Promise.reject(new Error('sem stream no teste')),
}));

beforeEach(() => {
  api.liveMetrics.mockReset().mockResolvedValue({ servers: servidores, containers: [], load_balancing: [] });
  api.servers.mockReset().mockResolvedValue(
    servidores.map((s) => ({ id: s.id, name: s.name, host_ip: s.host_ip, user: 'root', port: 22, created_at: '' })),
  );
  api.sites.mockReset().mockResolvedValue([]);
  api.networkHosts.mockReset().mockResolvedValue({ hosts: [] });
  api.history.mockReset().mockResolvedValue([]);
  api.alertRules.mockReset().mockResolvedValue([]);
  api.sslDomains.mockReset().mockResolvedValue([]);
  api.searchLogs.mockReset().mockResolvedValue([]);
  api.annotations.mockReset().mockResolvedValue([]);
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

const renderizar = (tela: React.ReactElement) =>
  render(
    <SessionContext.Provider value={sessao}>
      <SiteScopeContext.Provider value={escopo}>
        <NavigationContext.Provider value={{ openSite: vi.fn(), openMachine: vi.fn(), goBack: vi.fn() }}>
          <DialogContext.Provider value={dialogo}>{tela}</DialogContext.Provider>
        </NavigationContext.Provider>
      </SiteScopeContext.Provider>
    </SessionContext.Provider>,
  );

const linhaDe = async (nome: string) => {
  const celula = await screen.findByText(nome);
  const linha = celula.closest('tr');
  if (!linha) throw new Error(`linha de ${nome} não encontrada`);
  return within(linha);
};

describe('CPU e load sem medição', () => {
  it('ServersView mostra travessão para o host sem medida e 0% para o ocioso', async () => {
    renderizar(<ServersView />);

    const semMedida = await linhaDe('vps-sem-medida');
    expect(semMedida.getByTestId('cpu').textContent).toBe('—');
    expect(semMedida.getByTestId('load').textContent).toBe('—');

    const ociosa = await linhaDe('vps-ociosa');
    expect(ociosa.getByTestId('cpu').textContent).toBe('0%');
    expect(ociosa.getByTestId('load').textContent).toBe('0.00');
  });

  it('Dashboard tira o host sem medida da média e diz de quantos ela veio', async () => {
    renderizar(<Dashboard />);

    expect(await screen.findByText('20.0%')).toBeTruthy();
    expect(await screen.findByText(/média de 2 de 3 hosts/i)).toBeTruthy();
  });

  it('Dashboard sem nenhum host online mostra travessão nos três medidores, não 0.0%', async () => {
    api.liveMetrics.mockResolvedValue({
      servers: servidores.map((s) => ({ ...s, online: false })),
      containers: [],
      load_balancing: [],
    });

    renderizar(<Dashboard />);

    await screen.findByText('CPU do host');
    for (const titulo of ['CPU do host', 'Memória', 'Disco']) {
      const medidor = screen
        .getAllByText(titulo)
        .map((el) => el.closest('.stat-card'))
        .find((cartao): cartao is HTMLElement => cartao instanceof HTMLElement);
      if (!medidor) throw new Error(`medidor ${titulo} não encontrado`);
      expect(within(medidor).getByText('—')).toBeTruthy();
      expect(within(medidor).queryByText('0.0%')).toBeNull();
    }
  });

  it('StationsView não conta o host sem medida como pressionado', async () => {
    api.liveMetrics.mockResolvedValue({
      servers: [{ ...base, id: 'd', name: 'estacao-sem-medida', kind: 'agent', cpu: null, load1: null }],
      containers: [],
      load_balancing: [],
    });

    renderizar(<StationsView />);

    const linha = await linhaDe('estacao-sem-medida');
    expect(linha.getByTestId('cpu-estacao').textContent).toBe('—');
    expect(screen.getByTestId('estacoes-pressionadas').textContent).toBe('0');
  });
});

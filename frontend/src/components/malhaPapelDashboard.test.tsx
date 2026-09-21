import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, within } from '@testing-library/react';

import Dashboard from './Dashboard';
import { DialogContext, type DialogApi } from './ui/dialog-context';
import { NavigationContext } from './ui/navigation-context';
import { SessionContext, type SessionState } from './ui/session-context';
import { SiteScopeContext, type SiteScopeState } from './ui/site-scope-context';

const base = {
  uptime: 10,
  disk_used: 1,
  disk_total: 2,
  mem_used: 2_000_000_000,
  mem_total: 8_000_000_000,
  cpu: 12,
  load1: 0.4,
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
  addresses: [] as string[],
  net_rx_bps: null,
  net_tx_bps: null,
  rtt_ms: null,
};

const principal = {
  ...base,
  id: 'lb',
  name: 'Balanceador Principal',
  host_ip: '203.0.113.38',
  addresses: ['203.0.113.38'],
  nginx_estado: 'candidato' as const,
  nginx_papel: 'principal' as const,
  nginx_motivo: '',
};

const reserva = {
  ...base,
  id: 'lb2',
  name: 'Balanceador Reserva',
  host_ip: '203.0.113.37',
  addresses: ['203.0.113.37'],
  nginx_estado: 'candidato' as const,
  nginx_papel: 'reserva' as const,
  nginx_motivo: '',
};

const vps1 = {
  ...base,
  id: 'v1',
  name: 'VPS-1',
  host_ip: '203.0.113.25',
  addresses: ['203.0.113.25'],
  behind_lb: true,
  nginx_estado: 'ausente' as const,
  nginx_papel: 'nenhum' as const,
};

const topologia = [
  { server_id: 'lb', bloco: 'api', destino: '203.0.113.25:8080', observado_em: '2026-09-20T10:00:00Z' },
  { server_id: 'lb2', bloco: 'api', destino: '203.0.113.25:8080', observado_em: '2026-09-20T10:00:00Z' },
];

const api = vi.hoisted(() => ({
  liveMetrics: vi.fn(),
  servers: vi.fn(),
  updateServerAliases: vi.fn(),
  history: vi.fn(),
  annotations: vi.fn(),
}));

vi.mock('../lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../lib/api')>()),
  api,
  openStream: () => Promise.reject(new Error('sem stream no teste')),
}));

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

const sessao: SessionState = {
  username: 'p',
  role: 'admin',
  accesses: [{ site_id: null, role: 'admin' }],
  isToken: false,
  logout: vi.fn(),
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

const arestasComEstado = (estado: string) =>
  screen.getAllByTestId('malha-aresta').filter((el) => el.getAttribute('data-estado') === estado);

const balanceadorChamado = async (nome: string) => {
  const rotulo = await screen.findByText(nome);
  const caixa = rotulo.closest('[data-testid="malha-lb"]');
  if (!(caixa instanceof HTMLElement)) throw new Error(`balanceador ${nome} não encontrado`);
  return caixa;
};

beforeEach(() => {
  localStorage.clear();
  api.servers.mockReset().mockResolvedValue([]);
  api.history.mockReset().mockResolvedValue([]);
  api.annotations.mockReset().mockResolvedValue([]);
});

describe('malha do painel com papel descoberto', () => {
  it('principal e reserva aparecem os dois, rotulados', async () => {
    api.liveMetrics.mockReset().mockResolvedValue({
      servers: [principal, reserva, vps1],
      containers: [],
      load_balancing: [],
      lb_window_sec: 5,
      nginx_topologia: topologia,
    });
    renderizar();

    await vi.waitFor(() => expect(screen.getAllByTestId('malha-lb')).toHaveLength(2));

    expect(within(await balanceadorChamado('Balanceador Principal')).getByText('principal')).toBeTruthy();
    expect(within(await balanceadorChamado('Balanceador Reserva')).getByText('reserva')).toBeTruthy();
  });

  it('aresta declarada sem requisição é desenhada parada, não some', async () => {
    api.liveMetrics.mockReset().mockResolvedValue({
      servers: [principal, vps1],
      containers: [],
      load_balancing: [],
      lb_window_sec: 5,
      nginx_topologia: [topologia[0]],
    });
    renderizar();

    await screen.findAllByTestId('malha-no');
    await vi.waitFor(() => expect(arestasComEstado('parada').length).toBeGreaterThan(0));
    expect(arestasComEstado('com-trafego')).toHaveLength(0);
  });

  it('aresta que sai da reserva é potencial e nunca anima', async () => {
    api.liveMetrics.mockReset().mockResolvedValue({
      servers: [principal, reserva, vps1],
      containers: [],
      load_balancing: [
        {
          upstream_addr: '203.0.113.25:8080',
          server_name: 'app.exemplo',
          status: '200',
          requests_count: 42,
          server_id: 'lb',
        },
      ],
      lb_window_sec: 5,
      nginx_topologia: topologia,
    });
    renderizar();

    await screen.findAllByTestId('malha-no');
    await vi.waitFor(() => expect(arestasComEstado('com-trafego').length).toBeGreaterThan(0));

    const potenciais = arestasComEstado('potencial');
    expect(potenciais.length).toBeGreaterThan(0);
    for (const aresta of potenciais) {
      expect(aresta.getAttribute('stroke-dasharray')).toBe('5 5');
      expect(aresta.parentElement?.querySelector('animateMotion')).toBeNull();
    }
  });

  it('sem os campos novos a malha continua de pé pela marcação antiga', async () => {
    api.liveMetrics.mockReset().mockResolvedValue({
      servers: [
        { ...base, id: 'lb', name: 'Load Balancer', host_ip: '203.0.113.38', addresses: ['203.0.113.38'], collect_nginx: true },
        { ...base, id: 'v1', name: 'VPS-1', host_ip: '203.0.113.25', addresses: ['203.0.113.25'], behind_lb: true },
      ],
      containers: [],
      load_balancing: [
        {
          upstream_addr: '203.0.113.25:80',
          server_name: 'app.exemplo',
          status: '200',
          requests_count: 9,
          server_id: 'lb',
        },
      ],
      lb_window_sec: 5,
    });
    renderizar();

    await screen.findAllByTestId('malha-no');
    await vi.waitFor(() => expect(screen.getAllByTestId('malha-lb')).toHaveLength(1));
    expect(arestasComEstado('com-trafego').length).toBeGreaterThan(0);
    expect(arestasComEstado('potencial')).toHaveLength(0);
    expect(screen.getByTestId('malha-lb').getAttribute('data-papel')).toBe('indefinido');
  });
});

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

const lb = { ...base, id: 'lb', name: 'Load Balancer', host_ip: '198.51.100.38', addresses: ['198.51.100.38'], collect_nginx: true };
const lb2 = { ...base, id: 'lb2', name: 'Load Balancer 2', host_ip: '198.51.100.37', addresses: ['198.51.100.37'], collect_nginx: true };
const vps1 = { ...base, id: 'v1', name: 'VPS-1', host_ip: '198.51.100.25', addresses: ['198.51.100.25'], behind_lb: true };
const vps2 = { ...base, id: 'v2', name: 'VPS-2', host_ip: '198.51.100.39', addresses: ['198.51.100.39'], behind_lb: true };
const vps2ComOverlay = { ...vps2, addresses: ['198.51.100.39', '100.100.0.2'] };

const trafego = [
  { upstream_addr: '198.51.100.25:80', server_name: 'app.exemplo', status: '200', requests_count: 68, server_id: 'lb' },
  { upstream_addr: '198.51.100.39:80', server_name: 'app.exemplo', status: '200', requests_count: 35, server_id: 'lb' },
  { upstream_addr: '100.100.0.2:80', server_name: 'app.exemplo', status: '200', requests_count: 11, server_id: 'lb' },
];

const doisBalanceadores = [
  { upstream_addr: '198.51.100.39:80', server_name: 'app.exemplo', status: '200', requests_count: 20, server_id: 'lb' },
  { upstream_addr: '100.100.0.2:80', server_name: 'app.exemplo', status: '200', requests_count: 6, server_id: 'lb2' },
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

const comoAdmin: SessionState = {
  username: 'p',
  role: 'admin',
  accesses: [{ site_id: null, role: 'admin' }],
  isToken: false,
  logout: vi.fn(),
};

const renderizar = (sessao: SessionState) =>
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

const noChamado = async (rotulo: string) => {
  const alvo = await screen.findByText(rotulo);
  const no = alvo.closest('[data-testid="malha-no"]');
  if (!(no instanceof HTMLElement)) throw new Error(`nó ${rotulo} não encontrado`);
  return no;
};

const arestasDeMaquina = () =>
  screen.getAllByTestId('malha-aresta').filter((el) => el.getAttribute('data-para') !== null);

beforeEach(() => {
  localStorage.clear();
  api.liveMetrics
    .mockReset()
    .mockResolvedValue({ servers: [lb, vps1, vps2ComOverlay], containers: [], load_balancing: trafego });
  api.servers.mockReset().mockResolvedValue([
    { id: 'lb', name: 'Load Balancer', host_ip: '198.51.100.38', user: 'root', port: 22, created_at: '', aliases: [] },
    { id: 'v1', name: 'VPS-1', host_ip: '198.51.100.25', user: 'root', port: 22, created_at: '', aliases: [] },
    { id: 'v2', name: 'VPS-2', host_ip: '198.51.100.39', user: 'root', port: 22, created_at: '', aliases: [] },
  ]);
  api.updateServerAliases.mockReset().mockResolvedValue({});
  api.history.mockReset().mockResolvedValue([]);
  api.annotations.mockReset().mockResolvedValue([]);
  (dialogo.notify as ReturnType<typeof vi.fn>).mockReset();
});

describe('um nó por máquina, não por endereço', () => {
  it('soma os endereços da mesma VPS num nó só', async () => {
    renderizar(comoAdmin);

    await screen.findAllByTestId('malha-no');
    await vi.waitFor(() => expect(screen.getAllByTestId('malha-no')).toHaveLength(2));

    const vps2 = await noChamado('VPS-2');
    expect(within(vps2).getByText('46 req / 5s')).toBeTruthy();
    expect(vps2.textContent).toMatch(/198\.51\.100\.39:80/);
    expect(vps2.textContent).toMatch(/100\.100\.0\.2:80/);

    const vps1 = await noChamado('VPS-1');
    expect(within(vps1).getByText('68 req / 5s')).toBeTruthy();
  }, 15000);

  it('o cabeçalho conta máquinas, não endereços', async () => {
    renderizar(comoAdmin);

    expect(await screen.findByText(/2 atrás do balanceador/i)).toBeTruthy();
    expect(screen.queryByText(/3 atrás do balanceador/i)).toBeNull();
  }, 15000);

  it('uma aresta por par balanceador e máquina, mesmo com dois endereços', async () => {
    renderizar(comoAdmin);

    await screen.findAllByTestId('malha-no');
    await vi.waitFor(() => expect(arestasDeMaquina()).toHaveLength(2));
  }, 15000);

  it('dois balanceadores para a mesma máquina dão duas arestas e um nó', async () => {
    api.liveMetrics.mockReset().mockResolvedValue({
      servers: [lb, lb2, vps2ComOverlay],
      containers: [],
      load_balancing: doisBalanceadores,
    });
    renderizar(comoAdmin);

    await screen.findAllByTestId('malha-no');
    await vi.waitFor(() => expect(screen.getAllByTestId('malha-lb')).toHaveLength(2));
    expect(screen.getAllByTestId('malha-no')).toHaveLength(1);
    expect(arestasDeMaquina()).toHaveLength(2);

    const vps2 = await noChamado('VPS-2');
    expect(within(vps2).getByText('26 req / 5s')).toBeTruthy();
  }, 15000);

  it('associar um endereço solto funde o nó em vez de duplicar', async () => {
    const usuario = userEvent.setup();
    api.liveMetrics.mockReset().mockResolvedValue({
      servers: [lb, vps1, vps2],
      containers: [],
      load_balancing: trafego,
    });
    renderizar(comoAdmin);

    await vi.waitFor(() => expect(screen.getAllByTestId('malha-no')).toHaveLength(3));

    const solto = await noChamado('100.100.0.2:80');
    await usuario.click(within(solto).getByRole('button', { name: /associar/i }));

    const seletor = screen.getByRole('combobox', { name: 'Associar 100.100.0.2:80 a um servidor' });
    await usuario.click(seletor);
    await usuario.click(await screen.findByRole('option', { name: 'VPS-2' }));
    await vi.waitFor(() => expect(seletor.textContent).toMatch('VPS-2'));

    await usuario.click(screen.getByRole('button', { name: 'Salvar' }));

    await vi.waitFor(() => expect(api.updateServerAliases).toHaveBeenCalledWith('v2', ['100.100.0.2']));

    api.liveMetrics.mockResolvedValue({
      servers: [lb, vps1, vps2ComOverlay],
      containers: [],
      load_balancing: trafego,
    });
    await vi.waitFor(() => expect(screen.getAllByTestId('malha-no')).toHaveLength(2), { timeout: 8000 });
    expect(screen.getAllByText('VPS-2')).toHaveLength(1);
    expect(within(await noChamado('VPS-2')).getByText('46 req / 5s')).toBeTruthy();
  }, 20000);
});

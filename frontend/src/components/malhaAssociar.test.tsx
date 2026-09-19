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
const vps1 = { ...base, id: 'v1', name: 'VPS-1', host_ip: '198.51.100.25', addresses: ['198.51.100.25'], behind_lb: true };

const antes = [lb, vps1];
const depois = [lb, { ...vps1, addresses: ['198.51.100.25', '100.100.0.2'] }];

const trafego = [
  { upstream_addr: '198.51.100.25:80', server_name: 'app.exemplo', status: '200', requests_count: 12, server_id: 'lb' },
  { upstream_addr: '100.100.0.2:80', server_name: 'app.exemplo', status: '200', requests_count: 3, server_id: 'lb' },
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

const comoSuporte: SessionState = { ...comoAdmin, role: 'operator', accesses: [{ site_id: 1, role: 'operator' }] };

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

const noDe = async (endereco: string) => {
  const rotulo = await screen.findByText(endereco);
  const no = rotulo.closest('[data-testid="malha-no"]');
  if (!(no instanceof HTMLElement)) throw new Error(`nó de ${endereco} não encontrado`);
  return no;
};

beforeEach(() => {
  localStorage.clear();
  api.liveMetrics.mockReset().mockResolvedValue({ servers: antes, containers: [], load_balancing: trafego });
  api.servers.mockReset().mockResolvedValue([
    { id: 'lb', name: 'Load Balancer', host_ip: '198.51.100.38', user: 'root', port: 22, created_at: '', aliases: [] },
    { id: 'v1', name: 'VPS-1', host_ip: '198.51.100.25', user: 'root', port: 22, created_at: '', aliases: ['10.0.0.9'] },
  ]);
  api.updateServerAliases.mockReset().mockResolvedValue({});
  api.history.mockReset().mockResolvedValue([]);
  api.annotations.mockReset().mockResolvedValue([]);
  (dialogo.notify as ReturnType<typeof vi.fn>).mockReset();
});

describe('nomear o endereço solto pelo próprio nó da malha', () => {
  it('associa preservando os aliases que o servidor já tinha', async () => {
    const usuario = userEvent.setup();
    renderizar(comoAdmin);

    const no = await noDe('100.100.0.2:80');
    await usuario.click(within(no).getByRole('button', { name: /associar/i }));

    await usuario.click(await screen.findByRole('combobox', { name: 'Associar 100.100.0.2:80 a um servidor' }));
    await usuario.click(await screen.findByRole('option', { name: 'VPS-1' }));
    await usuario.click(screen.getByRole('button', { name: 'Salvar' }));

    await vi.waitFor(() =>
      expect(api.updateServerAliases).toHaveBeenCalledWith('v1', ['10.0.0.9', '100.100.0.2']),
    );
  }, 15000);

  it('depois de associar, o nó mostra o nome e o contador de endereço sem cadastro some', async () => {
    const usuario = userEvent.setup();
    renderizar(comoAdmin);

    expect(await screen.findByText(/1 endereço sem cadastro/i)).toBeTruthy();

    const solto = await noDe('100.100.0.2:80');
    await usuario.click(within(solto).getByRole('button', { name: /associar/i }));
    await usuario.click(await screen.findByRole('combobox', { name: 'Associar 100.100.0.2:80 a um servidor' }));
    await usuario.click(await screen.findByRole('option', { name: 'VPS-1' }));

    await usuario.click(screen.getByRole('button', { name: 'Salvar' }));
    await vi.waitFor(() => expect(api.updateServerAliases).toHaveBeenCalled());

    api.liveMetrics.mockResolvedValue({ servers: depois, containers: [], load_balancing: trafego });
    await vi.waitFor(() => expect(screen.queryByText(/endereço sem cadastro/i)).toBeNull(), { timeout: 8000 });

    const rotulo = await screen.findByText('VPS-1');
    const no = rotulo.closest('[data-testid="malha-no"]');
    if (!(no instanceof HTMLElement)) throw new Error('nó da VPS-1 não encontrado');
    expect(no.textContent).toMatch(/198\.51\.100\.25:80/);
    expect(no.textContent).toMatch(/100\.100\.0\.2:80/);
    expect(within(no).queryByText(/não cadastrado/i)).toBeNull();
  }, 15000);

  it('avisa quando o servidor não foi escolhido', async () => {
    const usuario = userEvent.setup();
    renderizar(comoAdmin);

    const no = await noDe('100.100.0.2:80');
    await usuario.click(within(no).getByRole('button', { name: /associar/i }));
    await usuario.click(screen.getByRole('button', { name: 'Salvar' }));

    expect(api.updateServerAliases).not.toHaveBeenCalled();
    expect(dialogo.notify).toHaveBeenCalled();
  }, 15000);

  it('quem não é admin global lê que precisa de um administrador', async () => {
    renderizar(comoSuporte);

    const no = await noDe('100.100.0.2:80');
    expect(within(no).queryByRole('button', { name: /associar/i })).toBeNull();
    expect(no.textContent).toMatch(/administrador/i);
  });
});

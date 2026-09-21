import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, within } from '@testing-library/react';
import { semEspera } from '../test/usuario';

import Dashboard from './Dashboard';
import NginxView from './NginxView';
import { DialogContext, type DialogApi } from './ui/dialog-context';
import { NavigationContext } from './ui/navigation-context';
import { SessionContext, type SessionState } from './ui/session-context';
import { SiteScopeContext, type SiteScopeState } from './ui/site-scope-context';
import type { SiteAccess } from '../lib/session';

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
  addresses: [] as string[],
  behind_lb: false,
};

const servidores = [
  { ...base, id: 'a', name: 'VPS-1', host_ip: '203.0.113.38', addresses: ['203.0.113.38', '100.100.0.11'], behind_lb: true },
  { ...base, id: 'b', name: 'VPS-2', host_ip: '203.0.113.39' },
];

const trafego = [
  { upstream_addr: '100.100.0.11:80', server_name: 'app.exemplo', status: '200', requests_count: 9 },
  { upstream_addr: '100.100.0.10:80', server_name: 'app.exemplo', status: '200', requests_count: 4 },
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

beforeEach(() => {
  api.liveMetrics.mockReset().mockResolvedValue({
    servers: servidores,
    containers: [],
    load_balancing: trafego,
  });
  api.servers.mockReset().mockResolvedValue(
    servidores.map((s) => ({
      id: s.id,
      name: s.name,
      host_ip: s.host_ip,
      user: 'root',
      port: 22,
      created_at: '',
      aliases: s.addresses.filter((ip) => ip !== s.host_ip),
    })),
  );
  api.updateServerAliases.mockReset().mockResolvedValue({});
  api.history.mockReset().mockResolvedValue([]);
  api.annotations.mockReset().mockResolvedValue([]);
  (dialogo.notify as ReturnType<typeof vi.fn>).mockReset();
});

const renderizar = (tela: React.ReactElement, accesses: SiteAccess[] = [{ site_id: null, role: 'admin' }]) => {
  const sessao: SessionState = {
    username: 'p',
    role: accesses[0].role,
    accesses,
    isToken: false,
    logout: vi.fn(),
  };
  return render(
    <SessionContext.Provider value={sessao}>
      <SiteScopeContext.Provider value={escopo}>
        <NavigationContext.Provider value={{ openSite: vi.fn(), openMachine: vi.fn(), goBack: vi.fn() }}>
          <DialogContext.Provider value={dialogo}>{tela}</DialogContext.Provider>
        </NavigationContext.Provider>
      </SiteScopeContext.Provider>
    </SessionContext.Provider>,
  );
};

describe('malha com endereço de overlay', () => {
  it('nomeia pelo endereço declarado e marca o resto como não cadastrado', async () => {
    renderizar(<Dashboard />);

    expect((await screen.findAllByText('VPS-1')).length).toBeGreaterThan(0);
    expect(screen.getAllByText('100.100.0.10:80').length).toBeGreaterThan(0);
    expect(screen.getAllByText(/não cadastrado/i).length).toBeGreaterThan(0);
  });

  it('não chama o upstream de overlay de VPS-2 por causa do octeto', async () => {
    renderizar(<Dashboard />);

    await screen.findAllByText('VPS-1');
    const caixa = screen.getByText('100.100.0.10:80').closest('div');
    expect(within(caixa as HTMLElement).queryByText('VPS-2')).toBeNull();
  });

  it('diz quantas máquinas estão atrás do balanceador, fora dele e quantos endereços estão soltos', async () => {
    renderizar(<Dashboard />);

    expect(await screen.findByText(/1 atrás do balanceador/i)).toBeTruthy();
    expect(screen.getByText(/1 fora do balanceador/i)).toBeTruthy();
    expect(screen.getByText(/1 endereço sem cadastro/i)).toBeTruthy();
  });
});

describe('associar upstream solto na tela de Nginx', () => {
  it('lista o endereço solto e associa ao servidor escolhido', async () => {
    renderizar(<NginxView />);

    const soltos = await screen.findAllByText('100.100.0.10:80');
    const bloco = soltos.map((el) => el.closest('li')).find(Boolean);
    expect(bloco).toBeTruthy();

    await semEspera().click(within(bloco as HTMLElement).getByLabelText(/associar a/i));
    await semEspera().click(await screen.findByRole('option', { name: /VPS-2/ }));
    await semEspera().click(within(bloco as HTMLElement).getByRole('button', { name: /associar/i }));

    expect(api.updateServerAliases).toHaveBeenCalledWith('b', ['100.100.0.10']);
  });

  it('quem não é admin global não vê a ação de associar', async () => {
    renderizar(<NginxView />, [{ site_id: 4, role: 'admin' }]);

    await screen.findByText(/Virtual hosts/i);
    expect(screen.queryByRole('button', { name: /associar/i })).toBeNull();
  });
});

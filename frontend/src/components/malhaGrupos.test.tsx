import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import Dashboard from './Dashboard';
import ServersView from './ServersView';
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

const servidores = [
  { ...base, id: 'lb', name: 'Load Balancer', host_ip: '198.51.100.38', collect_nginx: true, addresses: ['198.51.100.38'] },
  { ...base, id: 'v1', name: 'VPS-1', host_ip: '198.51.100.25', addresses: ['198.51.100.25'], behind_lb: true, behind_lb_origem: 'trafego' },
  { ...base, id: 'v2', name: 'VPS-2', host_ip: '198.51.100.39', addresses: ['198.51.100.39', '100.100.0.1'], behind_lb: true, behind_lb_origem: 'manual' },
  { ...base, id: 'mail', name: 'VPS E-mail', host_ip: '198.51.100.50', addresses: ['198.51.100.50'], cpu: 7, behind_lb: false, behind_lb_origem: 'nenhum' },
];

const trafego = [
  { upstream_addr: '198.51.100.25:80', server_name: 'app.exemplo', status: '200', requests_count: 12, server_id: 'lb' },
  { upstream_addr: '100.100.0.1:80', server_name: 'app.exemplo', status: '200', requests_count: 7, server_id: 'lb' },
  { upstream_addr: '100.100.0.2:80', server_name: 'app.exemplo', status: '200', requests_count: 3, server_id: 'lb' },
];

const api = vi.hoisted(() => ({
  liveMetrics: vi.fn(),
  servers: vi.fn(),
  createServer: vi.fn(),
  setServerBehindLb: vi.fn(),
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

beforeEach(() => {
  localStorage.clear();
  api.liveMetrics.mockReset().mockResolvedValue({ servers: servidores, containers: [], load_balancing: trafego });
  api.servers.mockReset().mockResolvedValue(
    servidores.map((s) => ({ id: s.id, name: s.name, host_ip: s.host_ip, user: 'root', port: 22, created_at: '' })),
  );
  api.createServer.mockReset().mockResolvedValue({});
  api.setServerBehindLb.mockReset().mockResolvedValue({});
  api.history.mockReset().mockResolvedValue([]);
  api.annotations.mockReset().mockResolvedValue([]);
  (dialogo.notify as ReturnType<typeof vi.fn>).mockReset();
});

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

describe('malha com máquina fora do balanceador', () => {
  it('conta os três grupos no cabeçalho', async () => {
    renderizar(<Dashboard />);

    expect(await screen.findByText(/2 atrás do balanceador/i)).toBeTruthy();
    expect(screen.getByText(/1 fora do balanceador/i)).toBeTruthy();
    expect(screen.getByText(/1 endereço sem cadastro/i)).toBeTruthy();
  });

  it('mostra a VPS de e-mail numa faixa própria, com o estado de coleta e sem linha do balanceador', async () => {
    renderizar(<Dashboard />);

    const faixa = (await screen.findByText('Fora do balanceador')).closest('section');
    expect(faixa).toBeTruthy();

    const cartao = within(faixa as HTMLElement).getByText('VPS E-mail').closest('div.panel');
    expect(within(cartao as HTMLElement).getByText(/CPU 7%/)).toBeTruthy();
    expect(within(cartao as HTMLElement).getByText(/RAM 1\.9\/7\.5 GB/)).toBeTruthy();
    expect(within(faixa as HTMLElement).queryByText(/req/i)).toBeNull();
  });

  it('o filtro esconde cada população e guarda a escolha', async () => {
    renderizar(<Dashboard />);

    await screen.findAllByText('VPS-1');

    await userEvent.click(screen.getByLabelText('Filtrar a malha'));
    await userEvent.click(await screen.findByRole('option', { name: /só atrás do balanceador/i }));
    expect(screen.queryByText('VPS E-mail')).toBeNull();
    expect(localStorage.getItem('dockkeeper.malha')).toBe('atras');

    await userEvent.click(screen.getByLabelText('Filtrar a malha'));
    await userEvent.click(await screen.findByRole('option', { name: /só fora do balanceador/i }));
    expect(await screen.findByText('VPS E-mail')).toBeTruthy();
    expect(screen.queryByText('100.100.0.2:80')).toBeNull();
    expect(localStorage.getItem('dockkeeper.malha')).toBe('fora');
  });

  it('abre já com a escolha guardada', async () => {
    localStorage.setItem('dockkeeper.malha', 'fora');
    renderizar(<Dashboard />);

    expect(await screen.findByText('VPS E-mail')).toBeTruthy();
    expect(screen.queryByText('100.100.0.2:80')).toBeNull();
  });
});

describe('topologia sem tráfego na janela', () => {
  it('desenha internet, balanceador e as duas VPS mesmo com zero requisição', async () => {
    api.liveMetrics.mockResolvedValue({ servers: servidores, containers: [], load_balancing: [] });
    renderizar(<Dashboard />);

    expect((await screen.findAllByText('VPS-1')).length).toBeGreaterThan(0);
    expect(screen.getAllByText('VPS-2').length).toBeGreaterThan(0);
    expect(screen.getAllByText('Load Balancer').length).toBeGreaterThan(0);
    expect(screen.getAllByText(/sem tráfego na janela/i).length).toBe(2);
  });

  it('nó sem tráfego não mostra contagem de requisição', async () => {
    api.liveMetrics.mockResolvedValue({
      servers: servidores,
      containers: [],
      load_balancing: [trafego[0]],
    });
    renderizar(<Dashboard />);

    const comTrafego = (await screen.findAllByText('VPS-1'))[0].closest('div.panel');
    expect(within(comTrafego as HTMLElement).getByText(/12 req/)).toBeTruthy();

    const semTrafego = screen.getAllByText('VPS-2')[0].closest('div.panel');
    expect(within(semTrafego as HTMLElement).getByText(/sem tráfego na janela/i)).toBeTruthy();
    expect(within(semTrafego as HTMLElement).queryByText(/req$/)).toBeNull();
  });

  it('o indicador de tráfego fica no canto, fora do bloco do filtro', async () => {
    renderizar(<Dashboard />);

    const indicador = await screen.findByTestId('malha-trafego');
    const filtro = screen.getByLabelText('Filtrar a malha');
    expect(indicador.parentElement?.contains(filtro)).toBe(false);
  });
});

describe('nó da malha não esmaga o nome', () => {
  const longos = [
    { ...base, id: 'lb', name: 'Load Balancer', host_ip: '198.51.100.38', collect_nginx: true },
    {
      ...base,
      id: 'mail',
      name: 'VPS-Email-Corporativo',
      host_ip: '100.100.0.2',
      addresses: ['100.100.0.2'],
      behind_lb: true,
      behind_lb_origem: 'manual',
    },
  ];

  const noDe = async (nome: string) => {
    const rotulo = (await screen.findAllByText(nome))[0];
    const caixa = rotulo.closest('div.panel');
    if (!(caixa instanceof HTMLElement)) throw new Error(`nó de ${nome} não encontrado`);
    return { rotulo, caixa };
  };

  it('sem tráfego, o nome inteiro fica na própria linha e o estado desce para baixo', async () => {
    api.liveMetrics.mockResolvedValue({ servers: longos, containers: [], load_balancing: [] });
    renderizar(<Dashboard />);

    const { rotulo, caixa } = await noDe('VPS-Email-Corporativo');
    const estado = within(caixa).getByText(/sem tráfego na janela/i);

    expect(rotulo.textContent).toBe('VPS-Email-Corporativo');
    expect(rotulo.parentElement?.contains(estado)).toBe(true);
    expect(rotulo.parentElement?.className).toMatch(/min-w-0/);
    expect(estado.className).toMatch(/text-\[10px\]/);
    expect(estado.className).not.toMatch(/shrink-0/);
    expect(within(caixa).getByText('100.100.0.2:80')).toBeTruthy();
  });

  it('com tráfego, o nó continua mostrando nome, IP e requisições', async () => {
    api.liveMetrics.mockResolvedValue({
      servers: longos,
      containers: [],
      load_balancing: [
        { upstream_addr: '100.100.0.2:80', server_name: 'app', status: '200', requests_count: 12, server_id: 'lb' },
      ],
    });
    renderizar(<Dashboard />);

    const { rotulo, caixa } = await noDe('VPS-Email-Corporativo');
    expect(rotulo.textContent).toBe('VPS-Email-Corporativo');
    expect(within(caixa).getByText('100.100.0.2:80')).toBeTruthy();
    expect(within(caixa).getByText(/12 req/)).toBeTruthy();
  });
});

describe('marcar máquina como atrás do balanceador', () => {
  it('admin global marca e o painel chama o PATCH', async () => {
    renderizar(<ServersView />);

    const linha = (await screen.findByText('VPS E-mail')).closest('tr');
    await userEvent.click(within(linha as HTMLElement).getByRole('button', { name: /atrás do balanceador/i }));

    expect(api.setServerBehindLb).toHaveBeenCalledWith('mail', true);
  });

  it('mostra que a classificação veio do tráfego quando é automática', async () => {
    renderizar(<ServersView />);

    const linha = (await screen.findByText('VPS-1')).closest('tr');
    expect(within(linha as HTMLElement).getByTitle(/automático/i)).toBeTruthy();
  });
});

describe('cadastro de servidor com endereço repetido', () => {
  it('mostra a mensagem do backend no 409', async () => {
    api.createServer.mockRejectedValue(
      new Error(JSON.stringify({ error: 'o endereço 198.51.100.39 já pertence a VPS-2 na unidade Matriz' })),
    );
    renderizar(<ServersView />);

    await userEvent.type(await screen.findByLabelText(/nome de identificação/i), 'VPS nova');
    await userEvent.type(screen.getByLabelText(/endereço ip/i), '198.51.100.39');
    await userEvent.click(screen.getByRole('button', { name: /conectar vps/i }));

    expect(dialogo.notify).toHaveBeenCalledWith(
      'o endereço 198.51.100.39 já pertence a VPS-2 na unidade Matriz',
      'error',
    );
  });
});

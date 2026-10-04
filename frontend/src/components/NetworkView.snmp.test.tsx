import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor, within } from '@testing-library/react';
import { semEspera } from '../test/usuario';

import NetworkView from './NetworkView';
import type { InterfaceDeRede, InterfacesDoHost, NetworkHostView, NetworkInventory } from '../lib/api';
import { SessionContext, type SessionState } from './ui/session-context';
import { DialogContext, type DialogApi } from './ui/dialog-context';
import { SiteScopeContext, type SiteScopeState } from './ui/site-scope-context';

vi.mock('recharts', async (importOriginal) => ({
  ...(await importOriginal<typeof import('recharts')>()),
  ResponsiveContainer: () => <div />,
}));

const { networkHosts, sites, interfacesDoHost, serieDaInterface } = vi.hoisted(() => ({
  networkHosts: vi.fn(),
  sites: vi.fn(),
  interfacesDoHost: vi.fn(),
  serieDaInterface: vi.fn(),
}));

vi.mock('../lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../lib/api')>()),
  api: { networkHosts, sites, interfacesDoHost, serieDaInterface },
}));

const AGORA = new Date().toISOString();

const host = (extra: Partial<NetworkHostView>): NetworkHostView => ({
  id: 1,
  ip: '192.0.2.1',
  hostname: 'sw-core',
  mac: '',
  open_ports: [],
  first_seen: AGORA,
  last_seen: AGORA,
  online: true,
  monitored: false,
  kind: '',
  device_type: '',
  device_type_locked: false,
  site_id: null,
  site_locked: false,
  floor: '',
  sector: '',
  room: '',
  rack: '',
  asset_tag: '',
  owner: '',
  notes: '',
  snmp_visto_em: null,
  snmp_erro: '',
  ...extra,
});

const inventario = (hosts: NetworkHostView[]): NetworkInventory => ({
  hosts,
  total: hosts.length,
  online: hosts.length,
  monitored: 0,
  last_scan: null,
  scan_active: true,
});

const interfaceDeRede = (extra: Partial<InterfaceDeRede>): InterfaceDeRede => ({
  id: 10,
  if_index: 1,
  if_name: 'ge-0/0/1',
  if_descr: '',
  if_alias: 'uplink',
  speed_mbps: 1000,
  oper_status: 'up',
  admin_status: 'up',
  last_seen: AGORA,
  ultima: {
    ts: AGORA,
    in_bps: 1234.5,
    out_bps: 25_000_000,
    in_errors: 0,
    out_errors: null,
    in_discards: 2,
    out_discards: 0,
  },
  ...extra,
});

const interfacesDoSwitch = (): InterfacesDoHost => ({
  host: {
    id: 1,
    ip: '192.0.2.1',
    hostname: 'sw-core',
    snmp_sys_name: 'sw-core',
    snmp_sys_descr: 'Switch gerenciável de teste',
    snmp_uptime_sec: 123456,
    snmp_visto_em: AGORA,
    snmp_erro: '',
    snmp_erro_em: null,
  },
  interfaces: [
    interfaceDeRede({}),
    interfaceDeRede({
      id: 11,
      if_index: 2,
      if_name: 'ge-0/0/2',
      if_alias: '',
      speed_mbps: null,
      oper_status: 'down',
      admin_status: 'down',
      ultima: null,
    }),
  ],
});

const dialogo: DialogApi = { confirm: vi.fn(async () => true), prompt: vi.fn(async () => null), notify: vi.fn() };

const sessao: SessionState = {
  username: 'pessoa',
  role: 'viewer',
  accesses: [{ site_id: null, role: 'viewer' }],
  isToken: false,
  logout: vi.fn(),
};

const escopo: SiteScopeState = {
  siteId: 'all',
  numericSiteId: null,
  setSiteId: vi.fn(),
  sites: [],
  siteName: () => '',
  reloadSites: vi.fn(),
};

const renderizar = () =>
  render(
    <SessionContext.Provider value={sessao}>
      <DialogContext.Provider value={dialogo}>
        <SiteScopeContext.Provider value={escopo}>
          <NetworkView />
        </SiteScopeContext.Provider>
      </DialogContext.Provider>
    </SessionContext.Provider>,
  );

beforeEach(() => {
  networkHosts.mockReset();
  sites.mockReset();
  sites.mockResolvedValue([]);
  interfacesDoHost.mockReset();
  interfacesDoHost.mockResolvedValue(interfacesDoSwitch());
  serieDaInterface.mockReset();
  serieDaInterface.mockResolvedValue([
    { ts: new Date(Date.now() - 120_000).toISOString(), in_bps: 1000, out_bps: 2000 },
    { ts: new Date(Date.now() - 60_000).toISOString(), in_bps: 3000, out_bps: null },
  ]);
});

describe('NetworkView — aba Interfaces', () => {
  it('host sem SNMP não ganha a aba nem a marca', async () => {
    const user = semEspera();
    networkHosts.mockResolvedValue(inventario([host({})]));
    renderizar();

    await user.click(await screen.findByRole('button', { name: 'Ver detalhe de 192.0.2.1' }));

    const painel = await screen.findByTestId('painel-do-host');
    expect(within(painel).getByRole('tab', { name: 'Resumo' })).toBeTruthy();
    expect(within(painel).queryByRole('tab', { name: 'Interfaces' })).toBeNull();
    expect(screen.queryByTestId('marca-snmp')).toBeNull();
    expect(interfacesDoHost).not.toHaveBeenCalled();
  });

  it('host com SNMP mostra cabeçalho e tabela com bps formatado e travessão no nulo', async () => {
    const user = semEspera();
    networkHosts.mockResolvedValue(inventario([host({ snmp_visto_em: AGORA })]));
    renderizar();

    expect(await screen.findByTestId('marca-snmp')).toBeTruthy();
    await user.click(screen.getByRole('button', { name: 'Ver detalhe de 192.0.2.1' }));
    await user.click(await screen.findByRole('tab', { name: 'Interfaces' }));

    expect((await screen.findByTestId('snmp-sys-name')).textContent).toBe('sw-core');
    expect(screen.getByTestId('snmp-sys-descr').textContent).toBe('Switch gerenciável de teste');
    expect(screen.getByTestId('snmp-uptime').textContent).toBe('1d 10h');
    expect(screen.getByTestId('snmp-visto').textContent).toMatch(/^Visto há/);
    expect(interfacesDoHost).toHaveBeenCalledWith(1, expect.anything());

    const [primeira, segunda] = screen.getAllByTestId('interface').map((linha) => within(linha));
    expect(primeira.getByText('uplink')).toBeTruthy();
    expect(primeira.getByTestId('interface-velocidade').textContent).toBe('1 Gbps');
    expect(primeira.getByTestId('interface-estado').textContent).toBe('Up');
    expect(primeira.getByTestId('interface-entrada').textContent).toBe('1,2 kbps');
    expect(primeira.getByTestId('interface-saida').textContent).toBe('25 Mbps');
    expect(primeira.getByTestId('interface-erros').textContent).toBe('0 / —');
    expect(primeira.getByTestId('interface-descartes').textContent).toBe('2 / 0');

    expect(segunda.getByTestId('interface-velocidade').textContent).toBe('—');
    expect(segunda.getByTestId('interface-estado').textContent).toBe('Desativada');
    expect(segunda.getByTestId('interface-entrada').textContent).toBe('—');
    expect(segunda.getByTestId('interface-saida').textContent).toBe('—');
    expect(segunda.getByTestId('interface-erros').textContent).toBe('— / —');
  });

  it('host só com erro de SNMP ganha a aba e mostra a falha', async () => {
    const user = semEspera();
    networkHosts.mockResolvedValue(inventario([host({ snmp_erro: 'timeout' })]));
    const dados = interfacesDoSwitch();
    interfacesDoHost.mockResolvedValue({
      host: { ...dados.host, snmp_visto_em: null, snmp_uptime_sec: null, snmp_sys_name: '', snmp_erro: 'timeout', snmp_erro_em: AGORA },
      interfaces: [],
    });
    renderizar();

    await user.click(await screen.findByRole('button', { name: 'Ver detalhe de 192.0.2.1' }));
    await user.click(await screen.findByRole('tab', { name: 'Interfaces' }));

    expect((await screen.findByTestId('snmp-erro')).textContent).toContain('timeout');
    expect(screen.getByTestId('snmp-sys-name').textContent).toBe('—');
    expect(screen.getByTestId('snmp-uptime').textContent).toBe('—');
    expect(screen.getByTestId('snmp-visto').textContent).toBe('Nunca respondeu ao SNMP');
  });

  it('clicar na interface pede a série de 1h e trocar a janela pede de novo com a escolhida', async () => {
    const user = semEspera();
    networkHosts.mockResolvedValue(inventario([host({ snmp_visto_em: AGORA })]));
    renderizar();

    await user.click(await screen.findByRole('button', { name: 'Ver detalhe de 192.0.2.1' }));
    await user.click(await screen.findByRole('tab', { name: 'Interfaces' }));
    await user.click(await screen.findByRole('button', { name: 'Ver tráfego de ge-0/0/1' }));

    expect(await screen.findByTestId('grafico-da-interface')).toBeTruthy();
    await waitFor(() => expect(serieDaInterface).toHaveBeenCalledWith(10, '1h', expect.anything()));

    await user.click(screen.getByRole('button', { name: '72h' }));
    await waitFor(() => expect(serieDaInterface).toHaveBeenLastCalledWith(10, '72h', expect.anything()));
    expect(screen.getByRole('button', { name: '72h' }).getAttribute('aria-pressed')).toBe('true');
  });
});

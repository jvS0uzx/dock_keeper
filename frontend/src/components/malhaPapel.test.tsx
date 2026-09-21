import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';

import NginxView from './NginxView';
import { DialogContext, type DialogApi } from './ui/dialog-context';
import { SessionContext, type SessionState } from './ui/session-context';

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
  {
    ...base,
    id: 'lb1',
    name: 'LB Principal',
    host_ip: '203.0.113.10',
    nginx_estado: 'candidato' as const,
    nginx_papel: 'principal' as const,
  },
  {
    ...base,
    id: 'lb2',
    name: 'LB Reserva',
    host_ip: '203.0.113.11',
    nginx_estado: 'candidato' as const,
    nginx_papel: 'reserva' as const,
  },
  { ...base, id: 'n1', name: 'NODE 1', host_ip: '198.51.100.21', behind_lb: true },
  { ...base, id: 'n2', name: 'NODE 2', host_ip: '198.51.100.22', behind_lb: true },
];

const topologia = [
  { server_id: 'lb1', bloco: 'app', destino: '198.51.100.21:80', observado_em: '2026-09-20T12:00:00Z' },
  { server_id: 'lb1', bloco: 'app', destino: '198.51.100.22:80', observado_em: '2026-09-20T12:00:00Z' },
  { server_id: 'lb2', bloco: 'app', destino: '198.51.100.21:80', observado_em: '2026-09-20T12:00:00Z' },
];

const api = vi.hoisted(() => ({ liveMetrics: vi.fn(), servers: vi.fn() }));

vi.mock('../lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../lib/api')>()),
  api,
}));

const dialogo: DialogApi = { confirm: vi.fn(), prompt: vi.fn(), notify: vi.fn() };

const renderizar = () => {
  const sessao: SessionState = {
    username: 'p',
    role: 'admin',
    accesses: [{ site_id: null, role: 'admin' }],
    isToken: false,
    logout: vi.fn(),
  };
  return render(
    <SessionContext.Provider value={sessao}>
      <DialogContext.Provider value={dialogo}>
        <NginxView />
      </DialogContext.Provider>
    </SessionContext.Provider>,
  );
};

const responder = (load_balancing: unknown[], nginx_topologia: unknown[] = topologia) =>
  api.liveMetrics.mockResolvedValue({
    servers: servidores,
    containers: [],
    load_balancing,
    lb_window_sec: 300,
    nginx_topologia,
  });

beforeEach(() => {
  api.liveMetrics.mockReset();
  api.servers.mockReset().mockResolvedValue([]);
});

describe('malha desenhada pela topologia', () => {
  it('desenha aresta declarada mesmo sem tráfego nenhum', async () => {
    responder([]);
    renderizar();

    expect(await screen.findByLabelText('Malha de roteamento do Nginx')).toBeTruthy();
    expect(screen.getAllByTestId('aresta-parada').length).toBe(2);
    expect(screen.queryAllByTestId('aresta-com-trafego')).toHaveLength(0);
  });

  it('anima só a aresta que tem requisição na janela', async () => {
    responder([
      { upstream_addr: '198.51.100.21:80', server_name: 'app.exemplo.com.br', status: '200', requests_count: 9, server_id: 'lb1' },
    ]);
    renderizar();

    await screen.findByLabelText('Malha de roteamento do Nginx');
    expect(screen.getAllByTestId('aresta-com-trafego')).toHaveLength(1);
    expect(screen.getAllByTestId('aresta-parada')).toHaveLength(1);
  });

  it('mostra a reserva com aresta potencial, distinta e sem animação', async () => {
    responder([
      { upstream_addr: '198.51.100.21:80', server_name: 'app.exemplo.com.br', status: '200', requests_count: 9, server_id: 'lb1' },
    ]);
    renderizar();

    await screen.findByLabelText('Malha de roteamento do Nginx');
    const potenciais = screen.getAllByTestId('aresta-potencial');
    expect(potenciais).toHaveLength(1);
    expect(potenciais[0].getAttribute('stroke-dasharray')).toBe('5 5');
    expect(await screen.findByText('LB Reserva')).toBeTruthy();
    expect(screen.getByText('reserva')).toBeTruthy();
  });

  it('nomeia o destino pelo servidor cadastrado em vez do endereço cru', async () => {
    responder([]);
    renderizar();

    await screen.findByLabelText('Malha de roteamento do Nginx');
    expect(screen.getByText('NODE 1')).toBeTruthy();
    expect(screen.getByText('NODE 2')).toBeTruthy();
  });

  it('avisa que ninguém foi descoberto quando não há topologia nem tráfego', async () => {
    responder([], []);
    renderizar();

    expect(await screen.findByText(/Nenhum balanceador descoberto ainda/)).toBeTruthy();
  });
});

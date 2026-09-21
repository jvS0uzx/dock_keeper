import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';

import MachineDetailView from './MachineDetailView';
import { NavigationContext } from './ui/navigation-context';
import { CATALOGO } from '../test/catalogo';
import { DialogContext } from './ui/dialog-context';
import { SessionContext, type SessionState } from './ui/session-context';

const leitor: SessionState = {
  username: 'l',
  role: 'viewer',
  accesses: [{ site_id: null, role: 'viewer' }],
  isToken: false,
  logout: vi.fn(),
};

const Contexto = ({ children }: { children: React.ReactNode }) => (
  <SessionContext.Provider value={leitor}>
    <DialogContext.Provider value={{ confirm: vi.fn(), prompt: vi.fn(), notify: vi.fn() }}>
      <NavigationContext.Provider value={{ openSite: vi.fn(), openMachine: vi.fn(), goBack: vi.fn() }}>
        {children}
      </NavigationContext.Provider>
    </DialogContext.Provider>
  </SessionContext.Provider>
);

const maquina = {
  id: 'srv-1',
  host_ip: '192.0.2.10',
  name: 'estacao-01',
  uptime: 3600,
  disk_used: 1,
  disk_total: 2,
  cpu: 10,
  mem_used: 1,
  mem_total: 2,
  load1: 0.5,
  online: true,
  ssh_handshake_ms: null,
  kind: 'agent',
  site_id: null,
  os: 'linux',
  platform: 'debian 12',
  arch: 'x86_64',
  last_user: '',
  agent_version: '1.1.0',
  temperature_c: null,
  collect_nginx: false,
  net_rx_bps: 1536,
  net_tx_bps: null,
  rtt_ms: 12,
};

const api = vi.hoisted(() => ({
  liveMetrics: vi.fn(),
  sites: vi.fn(),
  searchLogs: vi.fn(),
  networkHosts: vi.fn(),
  history: vi.fn(),
  metricsCatalog: vi.fn(),
}));

vi.mock('../lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../lib/api')>()),
  api,
}));

const falha = (mensagem: string) => new Error(JSON.stringify({ error: mensagem }));

beforeEach(() => {
  api.liveMetrics.mockReset().mockResolvedValue({ servers: [maquina], containers: [], load_balancing: [] });
  api.sites.mockReset().mockResolvedValue([]);
  api.searchLogs.mockReset().mockResolvedValue([]);
  api.networkHosts.mockReset().mockResolvedValue({ hosts: [] });
  api.history.mockReset().mockResolvedValue([]);
  api.metricsCatalog.mockReset().mockResolvedValue(CATALOGO);
});

afterEach(() => {
  vi.useRealTimers();
});

const renderizar = () =>
  render(
    <Contexto>
      <MachineDetailView serverId="srv-1" />
    </Contexto>,
  );

const statDe = async (rotulo: string) => {
  const eyebrow = await screen.findByText(rotulo);
  const cartao = eyebrow.closest('.stat-card');
  if (!cartao) throw new Error(`cartão ${rotulo} não encontrado`);
  return cartao.textContent ?? '';
};

describe('MachineDetailView — taxa de rede', () => {
  it('mostra a taxa atual e travessão quando não há medição', async () => {
    render(
      <Contexto>
        <MachineDetailView serverId="srv-1" />
      </Contexto>,
    );

    expect(await statDe('Rede RX')).toContain('1.5 KB/s');
    const tx = await statDe('Rede TX');
    expect(tx).toContain('—');
    expect(tx).not.toContain('0 B/s');
  });
});

describe('MachineDetailView — falha não vira vazio', () => {
  it('falha na leitura ao vivo mostra a mensagem, não "máquina não encontrada"', async () => {
    api.liveMetrics.mockRejectedValue(falha('painel fora do ar'));
    renderizar();

    expect(await screen.findByText(/painel fora do ar/)).toBeTruthy();
    expect(screen.queryByText(/Máquina não encontrada/)).toBeNull();
  });

  it('polling que falha mantém a máquina na tela e avisa de quando são os dados', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    api.liveMetrics
      .mockResolvedValueOnce({ servers: [maquina], containers: [], load_balancing: [] })
      .mockRejectedValue(falha('painel fora do ar'));
    renderizar();
    await screen.findByText('estacao-01');

    await vi.advanceTimersByTimeAsync(10100);

    expect(await screen.findByText(/Dados de \d{2}:\d{2}\. A última atualização falhou: painel fora do ar/)).toBeTruthy();
    expect(screen.getByText('estacao-01')).toBeTruthy();
  });

  it('falhas de logs, inventário e histórico aparecem em cada bloco', async () => {
    api.searchLogs.mockRejectedValue(falha('logs indisponíveis'));
    api.networkHosts.mockRejectedValue(falha('inventário indisponível'));
    api.history.mockRejectedValue(falha('histórico indisponível'));
    renderizar();

    expect(await screen.findByText(/logs indisponíveis/)).toBeTruthy();
    expect(screen.queryByText(/Nenhuma linha de log registrada/)).toBeNull();
    expect(await screen.findByText(/inventário indisponível/)).toBeTruthy();
    expect(await screen.findByText(/histórico indisponível/)).toBeTruthy();
  });
});

describe('MachineDetailView — latência', () => {
  it('mostra a latência atual em ms', async () => {
    renderizar();
    expect(await statDe('Latência')).toContain('12 ms');
  });

  it('latência ausente aparece como travessão', async () => {
    api.liveMetrics.mockResolvedValue({ servers: [{ ...maquina, rtt_ms: null }], containers: [], load_balancing: [] });
    renderizar();
    const texto = await statDe('Latência');
    expect(texto).toContain('—');
    expect(texto).not.toContain('0 ms');
  });
});

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';

import Dashboard from './Dashboard';
import { DialogContext, type DialogApi } from './ui/dialog-context';
import { NavigationContext } from './ui/navigation-context';
import { SessionContext, type SessionState } from './ui/session-context';
import { SiteScopeContext, type SiteScopeState } from './ui/site-scope-context';
import {
  ALTURA_BALANCEADOR,
  ALTURA_CARTAO,
  ALTURA_MAXIMA_MALHA,
  PASSO_CARTAO,
  alturaDoConteudo,
} from '../lib/malhaLayout';
import { avancar, comRelogioFalso } from '../test/usuario';
import { POLL } from '../lib/polling';

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

const cenario = (receptores: number, balanceadores = 1, requisicoes = 3) => {
  const lbs = Array.from({ length: balanceadores }, (_, i) => ({
    ...base,
    id: `lb${i}`,
    name: `Balanceador ${i + 1}`,
    host_ip: `10.1.0.${i + 1}`,
    addresses: [`10.1.0.${i + 1}`],
    collect_nginx: true,
  }));
  const vps = Array.from({ length: receptores }, (_, i) => ({
    ...base,
    id: `v${i}`,
    name: `VPS-${i + 1}`,
    host_ip: `10.2.0.${i + 1}`,
    addresses: [`10.2.0.${i + 1}`],
    behind_lb: true,
    behind_lb_origem: 'trafego' as const,
  }));
  const trafego = vps.map((s, i) => ({
    upstream_addr: `${s.host_ip}:80`,
    server_name: 'app.exemplo',
    status: '200',
    requests_count: requisicoes,
    server_id: lbs[i % lbs.length].id,
  }));
  return { servers: [...lbs, ...vps], load_balancing: trafego, containers: [] };
};

const api = vi.hoisted(() => ({
  liveMetrics: vi.fn(),
  servers: vi.fn(),
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

const topos = (testid: string) =>
  screen
    .getAllByTestId(testid)
    .map((el) => Number.parseFloat((el as HTMLElement).style.top))
    .sort((a, b) => a - b);

beforeEach(() => {
  localStorage.clear();
  api.servers.mockReset().mockResolvedValue([]);
  api.history.mockReset().mockResolvedValue([]);
  api.annotations.mockReset().mockResolvedValue([]);
});

afterEach(() => {
  vi.useRealTimers();
});

describe('malha que se adapta à arquitetura', () => {
  it('com um receptor mantém o piso da caixa', async () => {
    api.liveMetrics.mockReset().mockResolvedValue(cenario(1));
    renderizar();

    await screen.findAllByTestId('malha-no');
    expect(screen.getByTestId('malha-conteudo').style.height).toBe(`${alturaDoConteudo(1, 1)}px`);
  });

  it.each([2, 8])('com %i receptores nenhum cartão se sobrepõe', async (quantos) => {
    api.liveMetrics.mockReset().mockResolvedValue(cenario(quantos));
    renderizar();

    const nos = await screen.findAllByTestId('malha-no');
    expect(nos).toHaveLength(quantos);

    const centros = topos('malha-no');
    for (let i = 1; i < centros.length; i += 1) {
      expect(centros[i] - centros[i - 1]).toBeGreaterThanOrEqual(ALTURA_CARTAO);
    }
    expect(screen.getByTestId('malha-conteudo').style.height).toBe(`${alturaDoConteudo(quantos, 1)}px`);
  });

  it('a altura do painel cresce com a quantidade de nós', async () => {
    api.liveMetrics.mockReset().mockResolvedValue(cenario(2));
    const { unmount } = renderizar();
    await screen.findAllByTestId('malha-no');
    const doisNos = Number.parseFloat(screen.getByTestId('malha-conteudo').style.height);
    unmount();

    api.liveMetrics.mockReset().mockResolvedValue(cenario(8));
    renderizar();
    await screen.findAllByTestId('malha-no');
    const oitoNos = Number.parseFloat(screen.getByTestId('malha-conteudo').style.height);

    expect(oitoNos).toBeGreaterThan(doisNos);
  });

  it('com dois balanceadores empilha a coluna do meio sem sobreposição', async () => {
    api.liveMetrics.mockReset().mockResolvedValue(cenario(4, 2));
    renderizar();

    await screen.findAllByTestId('malha-no');
    await vi.waitFor(() => expect(screen.getAllByTestId('malha-lb')).toHaveLength(2));

    const centros = topos('malha-lb');
    expect(centros[1] - centros[0]).toBeGreaterThanOrEqual(ALTURA_BALANCEADOR);
    expect(screen.getAllByTestId('malha-aresta').length).toBeGreaterThanOrEqual(4);
  });

  it('passando do teto rola dentro da malha em vez de espremer', async () => {
    api.liveMetrics.mockReset().mockResolvedValue(cenario(12));
    renderizar();

    await screen.findAllByTestId('malha-no');
    const area = screen.getByTestId('malha-area');
    const conteudo = screen.getByTestId('malha-conteudo');

    expect(conteudo.style.height).toBe(`${12 * PASSO_CARTAO}px`);
    expect(area.style.height).toBe(`${ALTURA_MAXIMA_MALHA}px`);
    expect(area.className).toMatch(/overflow-y-auto/);

    const centros = topos('malha-no');
    for (let i = 1; i < centros.length; i += 1) {
      expect(centros[i] - centros[i - 1]).toBeGreaterThanOrEqual(ALTURA_CARTAO);
    }
  });
});

const FUNDO_DO_PAINEL = 'var(--color-line)';
const ACENTO_ATIVO = 'color-mix(in srgb, var(--color-accent) 45%, var(--color-ink-900))';

const tracos = () =>
  screen.getAllByTestId('malha-aresta').map((el) => ({
    stroke: el.getAttribute('stroke') ?? '',
    tracejado: el.getAttribute('stroke-dasharray'),
  }));

describe('malha sem tráfego na janela', () => {
  it('não desenha a linha ociosa com o token da borda de fundo nem com o acento', async () => {
    api.liveMetrics.mockReset().mockResolvedValue(cenario(2, 1, 0));
    renderizar();

    await screen.findAllByTestId('malha-no');
    const linhas = tracos();
    expect(linhas.length).toBeGreaterThan(0);
    for (const linha of linhas) {
      expect(linha.stroke).not.toBe(FUNDO_DO_PAINEL);
      expect(linha.stroke).not.toBe(ACENTO_ATIVO);
      expect(linha.tracejado).toBeTruthy();
    }
  });

  it('a linha com tráfego continua no acento âmbar, sem tracejado', async () => {
    api.liveMetrics.mockReset().mockResolvedValue(cenario(2, 1, 9));
    renderizar();

    await screen.findAllByTestId('malha-no');
    const linhas = tracos();
    expect(linhas.some((l) => l.stroke === ACENTO_ATIVO && l.tracejado === null)).toBe(true);
  });

  it('quando a janela esvazia, o traço troca de ativo para ocioso e segue visível', async () => {
    comRelogioFalso();
    api.liveMetrics.mockReset().mockResolvedValue(cenario(2, 1, 9));
    renderizar();

    await screen.findAllByTestId('malha-no');
    expect(tracos().some((l) => l.stroke === ACENTO_ATIVO)).toBe(true);

    api.liveMetrics.mockResolvedValue(cenario(2, 1, 0));
    await avancar(POLL.aoVivo);

    const linhas = tracos();
    expect(linhas.every((l) => l.stroke !== ACENTO_ATIVO)).toBe(true);
    expect(linhas.every((l) => l.stroke !== FUNDO_DO_PAINEL)).toBe(true);
  });
});

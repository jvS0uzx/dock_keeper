import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, within } from '@testing-library/react';
import { semEspera } from '../test/usuario';

import ContainersView from './ContainersView';
import type { ContainerLiveStat } from '../lib/api';
import { SessionContext, type SessionState } from './ui/session-context';
import { DialogContext, type DialogApi } from './ui/dialog-context';

const DADOS = {
  servers: [],
  load_balancing: [],
  containers: [
    { server_id: 's1', docker_id: 'a', name: 'loja-web', project: 'loja', state: 'running', status: 'Up', cpu: 12.5, mem_used: 300 * 1024 * 1024, mem_limit: 2 * 1024 ** 3 },
    { server_id: 's1', docker_id: 'b', name: 'loja-db', project: 'loja', state: 'running', status: 'Up', cpu: 30, mem_used: 1024 * 1024 * 1024, mem_limit: 4 * 1024 ** 3 },
    { server_id: 's1', docker_id: 'c', name: 'loja-worker', project: 'loja', state: 'exited', status: 'Exited (1)', cpu: 0, mem_used: 0, mem_limit: 0 },
  ],
};

const { liveMetrics } = vi.hoisted(() => ({ liveMetrics: vi.fn() }));

vi.mock('../lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../lib/api')>()),
  api: { liveMetrics },
  openStream: vi.fn(),
}));

const falhaDoPainel = () => new Error(JSON.stringify({ error: 'painel fora do ar' }));

beforeEach(() => {
  liveMetrics.mockReset();
  liveMetrics.mockResolvedValue(DADOS);
});

afterEach(() => {
  vi.useRealTimers();
});

const renderizar = () => {
  const sessao: SessionState = {
    username: 'p',
    role: 'viewer',
    accesses: [{ site_id: null, role: 'viewer' }],
    isToken: false,
    logout: vi.fn(),
  };
  const dialogo: DialogApi = { confirm: vi.fn(), prompt: vi.fn(), notify: vi.fn() };
  render(
    <SessionContext.Provider value={sessao}>
      <DialogContext.Provider value={dialogo}>
        <ContainersView />
      </DialogContext.Provider>
    </SessionContext.Provider>,
  );
};

const nomesNaTabela = () =>
  screen.getAllByTestId('container-nome').map((el) => el.textContent);

describe('ContainersView — filtro, ordenação e soma por projeto', () => {
  it('soma CPU e memória no cabeçalho do projeto', async () => {
    renderizar();
    const cabecalho = await screen.findByTestId('projeto-loja');
    expect(within(cabecalho).getByText('CPU 42.5%')).toBeTruthy();
    expect(within(cabecalho).getByText('Mem 1.29 GB')).toBeTruthy();
  });

  it('oculta parados e lembra a escolha', async () => {
    const user = semEspera();
    renderizar();
    await screen.findByText('loja-worker');

    await user.click(screen.getByRole('checkbox', { name: 'Ocultar parados' }));

    expect(screen.queryByText('loja-worker')).toBeNull();
    expect(JSON.parse(localStorage.getItem('dockkeeper.containers') ?? '{}').hideStopped).toBe(true);
  });

  it('ordena por CPU e por memória', async () => {
    const user = semEspera();
    renderizar();
    await screen.findByText('loja-worker');
    expect(nomesNaTabela()).toEqual(['loja-db', 'loja-web', 'loja-worker']);

    await user.click(screen.getByRole('combobox', { name: 'Ordenar por' }));
    await user.click(screen.getByRole('option', { name: 'CPU, maior primeiro' }));
    expect(nomesNaTabela()).toEqual(['loja-db', 'loja-web', 'loja-worker']);

    await user.click(screen.getByRole('combobox', { name: 'Ordenar por' }));
    await user.click(screen.getByRole('option', { name: 'CPU, menor primeiro' }));
    expect(nomesNaTabela()).toEqual(['loja-worker', 'loja-web', 'loja-db']);

    await user.click(screen.getByRole('combobox', { name: 'Ordenar por' }));
    await user.click(screen.getByRole('option', { name: 'Nome, Z a A' }));
    expect(nomesNaTabela()).toEqual(['loja-worker', 'loja-web', 'loja-db']);
  });
});

const BASE: ContainerLiveStat = {
  server_id: 's1',
  docker_id: 'x',
  name: 'app',
  project: 'loja',
  state: 'running',
  status: 'Up 2 minutes',
  cpu: 5,
  mem_used: 100,
  mem_limit: 1000,
};

const comContainer = (extra: Partial<ContainerLiveStat>) => {
  liveMetrics.mockResolvedValue({
    servers: [],
    load_balancing: [],
    containers: [{ ...BASE, ...extra }],
  });
};

const estadoNaTela = () => screen.findByTestId('container-estado');

describe('ContainersView — container que morre não fica verde', () => {
  it('unhealthy sai do verde e diz que está sem saúde', async () => {
    comContainer({ health: 'unhealthy' });
    renderizar();

    const estado = await estadoNaTela();
    expect(estado.textContent).toBe('sem saúde');
    expect(estado.className).toContain('badge-warn');
    expect(estado.className).not.toContain('badge-ok');
  });

  it('morto por falta de memória sai do verde', async () => {
    comContainer({ oom_killed: true });
    renderizar();

    const estado = await estadoNaTela();
    expect(estado.textContent).toBe('sem memória');
    expect(estado.className).toContain('badge-warn');
    expect(estado.className).not.toContain('badge-ok');
  });

  it('reiniciando não é pintado como parado', async () => {
    comContainer({ state: 'restarting' });
    renderizar();

    const estado = await estadoNaTela();
    expect(estado.textContent).toBe('reiniciando');
    expect(estado.className).toContain('badge-warn');
    expect(estado.className).not.toContain('badge-crit');
  });

  it('healthy continua verde', async () => {
    comContainer({ health: 'healthy', oom_killed: false, restart_count: 0 });
    renderizar();

    const estado = await estadoNaTela();
    expect(estado.textContent).toBe('rodando');
    expect(estado.className).toContain('badge-ok');
  });

  it('container parado continua vermelho', async () => {
    comContainer({ state: 'exited', status: 'Exited (1)' });
    renderizar();

    const estado = await estadoNaTela();
    expect(estado.textContent).toBe('parado');
    expect(estado.className).toContain('badge-crit');
  });
});

describe('ContainersView — não observado não vira alarme nem saúde', () => {
  it('inspeção nula mantém o que o estado diz, sem inventar alarme', async () => {
    comContainer({ health: null, oom_killed: null, restart_count: null });
    renderizar();

    const estado = await estadoNaTela();
    expect(estado.textContent).toBe('rodando');
    expect(estado.className).toContain('badge-ok');
    expect(screen.queryByTestId('container-reinicios')).toBeNull();
  });

  it('container sem healthcheck não é acusado de doente', async () => {
    comContainer({ health: '', oom_killed: false, restart_count: 0 });
    renderizar();

    const estado = await estadoNaTela();
    expect(estado.textContent).toBe('rodando');
    expect(estado.className).toContain('badge-ok');
  });

  it('campos ausentes no payload antigo não quebram a linha', async () => {
    comContainer({});
    renderizar();

    const estado = await estadoNaTela();
    expect(estado.textContent).toBe('rodando');
    expect(estado.className).toContain('badge-ok');
    expect(screen.queryByTestId('container-reinicios')).toBeNull();
  });
});

describe('ContainersView — contagem de reinícios', () => {
  it('mostra a contagem quando maior que zero', async () => {
    comContainer({ restart_count: 7 });
    renderizar();

    expect((await screen.findByTestId('container-reinicios')).textContent).toContain('7 reinícios');
  });

  it('usa o singular para um reinício', async () => {
    comContainer({ restart_count: 1 });
    renderizar();

    expect((await screen.findByTestId('container-reinicios')).textContent).toContain('1 reinício');
  });

  it('omite a contagem quando é zero', async () => {
    comContainer({ restart_count: 0 });
    renderizar();

    await estadoNaTela();
    expect(screen.queryByTestId('container-reinicios')).toBeNull();
  });
});

describe('ContainersView — falha não vira vazio', () => {
  it('falha na primeira leitura mostra a mensagem, não "Nenhum container encontrado"', async () => {
    liveMetrics.mockRejectedValue(falhaDoPainel());
    renderizar();

    expect(await screen.findByText(/painel fora do ar/)).toBeTruthy();
    expect(screen.queryByText('Nenhum container encontrado.')).toBeNull();
  });

  it('polling que falha mantém a tabela e avisa de quando são os dados', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    liveMetrics.mockResolvedValueOnce(DADOS).mockRejectedValue(falhaDoPainel());
    renderizar();
    await screen.findByText('loja-web');

    await vi.advanceTimersByTimeAsync(3100);

    expect(await screen.findByText(/Dados de \d{2}:\d{2}\. A última atualização falhou: painel fora do ar/)).toBeTruthy();
    expect(screen.getByText('loja-web')).toBeTruthy();
  });
});

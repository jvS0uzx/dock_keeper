import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import ContainersView from './ContainersView';
import { DialogContext, type DialogApi } from './ui/dialog-context';
import { SessionContext, type SessionState } from './ui/session-context';

class StreamFalso {
  onmessage: ((e: { data: string }) => void) | null = null;
  onerror: (() => void) | null = null;
  fechado = false;

  close() {
    this.fechado = true;
  }
}

const abertos: StreamFalso[] = [];

const api = vi.hoisted(() => ({
  liveMetrics: vi.fn(),
  containerAction: vi.fn(),
}));

const openStream = vi.hoisted(() => vi.fn());

vi.mock('../lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../lib/api')>()),
  api,
  openStream,
}));

const container = {
  server_id: 's1',
  docker_id: 'abc',
  name: 'api',
  project: 'loja',
  state: 'running',
  status: 'Up 2 hours',
  cpu: 1,
  mem_used: 10,
  mem_limit: 100,
};

const sessao: SessionState = {
  username: 'p',
  role: 'admin',
  accesses: [{ site_id: null, role: 'admin' }],
  isToken: false,
  logout: vi.fn(),
};

const dialogo: DialogApi = { confirm: vi.fn(), prompt: vi.fn(), notify: vi.fn() };

const renderizar = () =>
  render(
    <SessionContext.Provider value={sessao}>
      <DialogContext.Provider value={dialogo}>
        <ContainersView />
      </DialogContext.Provider>
    </SessionContext.Provider>,
  );

beforeEach(() => {
  abertos.length = 0;
  api.liveMetrics.mockReset().mockResolvedValue({ servers: [], containers: [container], load_balancing: [] });
  api.containerAction.mockReset().mockResolvedValue({ status: 'ok' });
  openStream.mockReset().mockImplementation(() => {
    const s = new StreamFalso();
    abertos.push(s);
    return Promise.resolve(s);
  });
});

afterEach(() => {
  vi.useRealTimers();
});

const abrirLogs = async () => {
  const usuario = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
  const botao = await screen.findByTitle('Ver logs ao vivo');
  await usuario.click(botao);
  await vi.waitFor(() => expect(abertos.length).toBe(1));
};

describe('stream de logs reconecta sozinho', () => {
  it('pede ticket novo depois da queda e avisa que está reconectando', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    renderizar();
    await abrirLogs();

    act(() => abertos[0].onerror?.());

    const avisos = await screen.findAllByRole('status');
    expect(avisos.some((n) => /reconectando/i.test(n.textContent ?? ''))).toBe(true);
    expect(abertos[0].fechado).toBe(true);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1500);
    });

    expect(openStream).toHaveBeenCalledTimes(2);
  });

  it('para de reconectar quando a tela sai', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const tela = renderizar();
    await abrirLogs();

    act(() => abertos[0].onerror?.());
    tela.unmount();

    await act(async () => {
      await vi.advanceTimersByTimeAsync(60000);
    });

    expect(openStream).toHaveBeenCalledTimes(1);
  });

  it('não abre em laço apertado: cada tentativa que falha espera mais que a anterior', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    renderizar();
    await abrirLogs();

    openStream.mockImplementation(() => Promise.reject(new Error('sem ticket')));
    act(() => abertos[0].onerror?.());

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1300);
    });
    expect(openStream).toHaveBeenCalledTimes(2);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1000);
    });
    expect(openStream).toHaveBeenCalledTimes(2);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1600);
    });
    expect(openStream).toHaveBeenCalledTimes(3);
  });

  it('volta ao início do backoff depois de uma conexão saudável', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    renderizar();
    await abrirLogs();

    act(() => abertos[0].onerror?.());
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1300);
    });
    expect(openStream).toHaveBeenCalledTimes(2);

    act(() => abertos[1].onerror?.());
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1300);
    });
    expect(openStream).toHaveBeenCalledTimes(3);
  });
});

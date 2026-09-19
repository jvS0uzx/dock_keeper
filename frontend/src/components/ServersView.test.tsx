import { describe, expect, it, vi } from 'vitest';
import { render, screen, within } from '@testing-library/react';

import ServersView from './ServersView';
import { DialogContext, type DialogApi } from './ui/dialog-context';
import { SessionContext, type SessionState } from './ui/session-context';

const api = vi.hoisted(() => ({
  servers: vi.fn(async () => [
    { id: 'a', name: 'vps-lenta', host_ip: '203.0.113.10', user: 'root', port: 22, created_at: '' },
    { id: 'b', name: 'vps-rapida', host_ip: '203.0.113.11', user: 'root', port: 22, created_at: '' },
    { id: 'c', name: 'vps-sem-medida', host_ip: '203.0.113.12', user: 'root', port: 22, created_at: '' },
  ]),
  liveMetrics: vi.fn(async () => ({
    containers: [],
    load_balancing: [],
    servers: [
      { id: 'a', online: true, cpu: 1, mem_used: 1, mem_total: 2, load1: 0.1, rtt_ms: 1200 },
      { id: 'b', online: true, cpu: 1, mem_used: 1, mem_total: 2, load1: 0.1, rtt_ms: 12 },
      { id: 'c', online: true, cpu: 1, mem_used: 1, mem_total: 2, load1: 0.1, rtt_ms: null },
    ],
  })),
}));

vi.mock('../lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../lib/api')>()),
  api,
}));

const linhaDe = async (nome: string) => {
  const celula = await screen.findByText(nome);
  const linha = celula.closest('tr');
  if (!linha) throw new Error(`linha de ${nome} não encontrada`);
  return within(linha);
};

describe('ServersView — latência', () => {
  it('mostra a latência de cada servidor, em segundos acima de 1000 ms e travessão sem medida', async () => {
    const dialogo: DialogApi = { confirm: vi.fn(), prompt: vi.fn(), notify: vi.fn() };
    const sessao: SessionState = {
      username: 'p',
      role: 'admin',
      accesses: [{ site_id: null, role: 'admin' }],
      isToken: false,
      logout: vi.fn(),
    };
    render(
      <SessionContext.Provider value={sessao}>
        <DialogContext.Provider value={dialogo}>
          <ServersView />
        </DialogContext.Provider>
      </SessionContext.Provider>,
    );

    expect(await screen.findByRole('columnheader', { name: 'Latência' })).toBeTruthy();
    expect((await linhaDe('vps-lenta')).getByText('1,2 s')).toBeTruthy();
    expect((await linhaDe('vps-rapida')).getByText('12 ms')).toBeTruthy();
    const semMedida = await linhaDe('vps-sem-medida');
    expect(semMedida.getByTestId('latencia').textContent).toBe('—');
  });
});

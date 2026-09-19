import { afterEach, describe, expect, it, vi } from 'vitest';

import { api } from './api';

const resposta = (corpo: unknown) =>
  new Response(JSON.stringify(corpo), { status: 200, headers: { 'Content-Type': 'application/json' } });

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('prontidão do painel', () => {
  it('usa a rota com CORS servida sob /api', async () => {
    const chamada = vi.fn().mockResolvedValue(resposta({ status: 'ok' }));
    vi.stubGlobal('fetch', chamada);

    await api.readiness();

    expect(chamada.mock.calls[0][0]).toMatch(/\/api\/readyz$/);
  });

  it('devolve o corpo da prontidão como veio do backend', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        resposta({ status: 'degradado', alertas_sem_canal: 2, degradado: ['alerta preso sem canal'] }),
      ),
    );

    const estado = await api.readiness();

    expect(estado.status).toBe('degradado');
    expect(estado.alertas_sem_canal).toBe(2);
  });
});

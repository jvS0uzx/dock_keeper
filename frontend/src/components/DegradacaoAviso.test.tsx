import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { semEspera } from '../test/usuario';

import DegradacaoAviso from './DegradacaoAviso';

const api = vi.hoisted(() => ({ readiness: vi.fn() }));

vi.mock('../lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../lib/api')>()),
  api,
}));

beforeEach(() => {
  api.readiness.mockReset();
});

describe('faixa de degradação', () => {
  it('fica escondida quando o painel está saudável', async () => {
    api.readiness.mockResolvedValue({
      status: 'ok',
      db: 'ok',
      alertas: 'ok',
      alertas_falhos: 0,
      logs_descartados: 0,
    });
    render(<DegradacaoAviso irParaAlertas={vi.fn()} />);

    await vi.waitFor(() => expect(api.readiness).toHaveBeenCalled());
    expect(screen.queryByRole('status')).toBeNull();
  });

  it('fica escondida enquanto o backend não manda os campos novos', async () => {
    api.readiness.mockResolvedValue({ status: 'ok', db: 'ok' });
    render(<DegradacaoAviso irParaAlertas={vi.fn()} />);

    await vi.waitFor(() => expect(api.readiness).toHaveBeenCalled());
    expect(screen.queryByRole('status')).toBeNull();
  });

  it('mostra os motivos do servidor quando o canal está degradado', async () => {
    api.readiness.mockResolvedValue({
      status: 'degradado',
      db: 'ok',
      alertas: 'degradado',
      alertas_detalhe: 'telegram fora do ar desde 12:04',
      alertas_falhos: 0,
      logs_descartados: 0,
      degradado: ['canal de alerta degradado'],
    });
    render(<DegradacaoAviso irParaAlertas={vi.fn()} />);

    const faixa = await screen.findByRole('status');
    expect(faixa.textContent).toMatch(/canal de alerta degradado/);
    expect(faixa.textContent).toMatch(/telegram fora do ar desde 12:04/);
    expect(screen.queryByRole('button', { name: /ver alertas/i })).toBeNull();
  });

  it('oferece a tela de alertas quando há entrega falha', async () => {
    const irParaAlertas = vi.fn();
    api.readiness.mockResolvedValue({
      status: 'degradado',
      db: 'ok',
      alertas: 'ok',
      alertas_falhos: 3,
      logs_descartados: 0,
      degradado: ['alerta com entrega falhou'],
    });
    render(<DegradacaoAviso irParaAlertas={irParaAlertas} />);

    const faixa = await screen.findByRole('status');
    expect(faixa.textContent).toMatch(/alerta com entrega falhou/);
    expect(faixa.textContent).toMatch(/3/);

    const usuario = semEspera();
    await usuario.click(screen.getByRole('button', { name: /ver alertas/i }));
    expect(irParaAlertas).toHaveBeenCalled();
  });

  it('avisa sobre log descartado sem oferecer os alertas', async () => {
    api.readiness.mockResolvedValue({
      status: 'degradado',
      db: 'ok',
      alertas: 'ok',
      alertas_falhos: 0,
      logs_descartados: 1200,
      degradado: ['linha de log descartada pela fila'],
    });
    render(<DegradacaoAviso irParaAlertas={vi.fn()} />);

    const faixa = await screen.findByRole('status');
    expect(faixa.textContent).toMatch(/linha de log descartada pela fila/);
    expect(faixa.textContent).toMatch(/1200/);
    expect(screen.queryByRole('button', { name: /ver alertas/i })).toBeNull();
  });

  it('avisa sobre alerta preso sem canal e leva para os alertas', async () => {
    const irParaAlertas = vi.fn();
    api.readiness.mockResolvedValue({
      status: 'degradado',
      db: 'ok',
      alertas: 'ok',
      alertas_falhos: 0,
      alertas_sem_canal: 2,
      logs_descartados: 0,
      degradado: ['alerta preso sem canal de entrega configurado'],
    });
    render(<DegradacaoAviso irParaAlertas={irParaAlertas} />);

    const faixa = await screen.findByRole('status');
    expect(faixa.textContent).toMatch(/alerta preso sem canal de entrega configurado/);
    expect(faixa.textContent).toMatch(/2 alerta\(s\) sem canal de entrega/);

    const usuario = semEspera();
    await usuario.click(screen.getByRole('button', { name: /ver alertas/i }));
    expect(irParaAlertas).toHaveBeenCalled();
  });

  it('avisa que não consegue ler a prontidão em vez de sumir', async () => {
    api.readiness.mockRejectedValue(new Error(JSON.stringify({ error: 'painel fora do ar' })));
    render(<DegradacaoAviso irParaAlertas={vi.fn()} />);

    const faixa = await screen.findByRole('status');
    expect(faixa.textContent).toMatch(/prontidão/i);
    expect(faixa.textContent).toMatch(/painel fora do ar/);
    expect(faixa.textContent).not.toMatch(/Painel degradado/);
  });

  it('não confunde falha de leitura com painel saudável', async () => {
    api.readiness.mockResolvedValue({ status: 'ok', db: 'ok', alertas: 'ok', degradado: [] });
    render(<DegradacaoAviso irParaAlertas={vi.fn()} />);

    await vi.waitFor(() => expect(api.readiness).toHaveBeenCalled());
    expect(screen.queryByRole('status')).toBeNull();
  });

  it('volta ao estado normal quando a leitura seguinte dá certo', async () => {
    api.readiness
      .mockRejectedValueOnce(new Error('rede fora'))
      .mockResolvedValue({ status: 'ok', db: 'ok', alertas: 'ok', degradado: [] });
    vi.useFakeTimers();
    try {
      render(<DegradacaoAviso irParaAlertas={vi.fn()} />);
      await vi.waitFor(() => expect(screen.queryByRole('status')).not.toBeNull());
      await vi.advanceTimersByTimeAsync(30000);
      await vi.waitFor(() => expect(screen.queryByRole('status')).toBeNull());
    } finally {
      vi.useRealTimers();
    }
  });
});

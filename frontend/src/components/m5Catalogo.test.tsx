import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';

import { CATALOGO } from '../test/catalogo';
import { carregarCatalogo, esquecerCatalogo } from '../lib/catalogo';
import { useCatalogo } from './ui/useCatalogo';

const api = vi.hoisted(() => ({ metricsCatalog: vi.fn() }));

vi.mock('../lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../lib/api')>()),
  api,
}));

const Tela = () => {
  const catalogo = useCatalogo();
  return (
    <div>
      {catalogo.erro && <p role="alert">{catalogo.erro}</p>}
      <span>{catalogo.rotulo('net_rx')}</span>
      <span>{catalogo.metricas.length} métricas</span>
    </div>
  );
};

beforeEach(() => {
  api.metricsCatalog.mockReset();
});

describe('catálogo de métricas', () => {
  it('é pedido uma vez por sessão, por mais telas que o usem', async () => {
    api.metricsCatalog.mockResolvedValue(CATALOGO);
    render(
      <>
        <Tela />
        <Tela />
      </>,
    );
    expect(await screen.findAllByText('Rede recebida')).toHaveLength(2);
    await carregarCatalogo();
    expect(api.metricsCatalog).toHaveBeenCalledTimes(1);
  });

  it('sair da sessão esquece o catálogo', async () => {
    api.metricsCatalog.mockResolvedValue(CATALOGO);
    await carregarCatalogo();
    esquecerCatalogo();
    await carregarCatalogo();
    expect(api.metricsCatalog).toHaveBeenCalledTimes(2);
  });

  it('falha aparece na tela, não vira lista vazia calada, e a próxima tentativa pede de novo', async () => {
    api.metricsCatalog.mockRejectedValueOnce(new Error(JSON.stringify({ error: 'catálogo fora do ar' })));
    render(<Tela />);
    expect((await screen.findByRole('alert')).textContent).toBe('catálogo fora do ar');
    expect(screen.getByText('net_rx')).toBeTruthy();

    api.metricsCatalog.mockResolvedValue(CATALOGO);
    await expect(carregarCatalogo()).resolves.toHaveLength(CATALOGO.length);
  });
});

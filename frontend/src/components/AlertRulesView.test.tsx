import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { semEspera } from '../test/usuario';

import AlertRulesView from './AlertRulesView';
import { SessionContext, type SessionState } from './ui/session-context';
import { SiteScopeContext, type SiteScopeState } from './ui/site-scope-context';
import { DialogContext, type DialogApi } from './ui/dialog-context';

vi.mock('../lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../lib/api')>()),
  api: {
    alertRules: vi.fn(async () => [
      {
        id: 1,
        name: 'Download alto',
        target: '*',
        metric: 'net_rx',
        operator: '>',
        threshold: 10 * 1024 * 1024,
        enabled: true,
        last_fired: null,
        for_duration_sec: 0,
      },
    ]),
    liveMetrics: vi.fn(async () => ({ servers: [], containers: [], load_balancing: [] })),
    metricsCatalog: vi.fn(async () => (await import('../test/catalogo')).CATALOGO),
  },
}));

const renderizar = () => {
  const sessao: SessionState = {
    username: 'pessoa-de-teste',
    role: 'operator',
    accesses: [{ site_id: null, role: 'operator' }],
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
  const dialogo: DialogApi = { confirm: vi.fn(), prompt: vi.fn(), notify: vi.fn() };

  render(
    <SessionContext.Provider value={sessao}>
      <SiteScopeContext.Provider value={escopo}>
        <DialogContext.Provider value={dialogo}>
          <AlertRulesView />
        </DialogContext.Provider>
      </SiteScopeContext.Provider>
    </SessionContext.Provider>,
  );
};

describe('AlertRulesView — métricas novas', () => {
  it('oferece temperatura e taxa de rede além das métricas de uso', async () => {
    const user = semEspera();
    renderizar();

    await screen.findByText(/Rede recebida/);
    await user.click(screen.getByRole('combobox', { name: 'Métrica' }));
    const opcoes = screen.getAllByRole('option').map((o) => o.textContent);

    expect(opcoes).toEqual(expect.arrayContaining([
      'CPU (%)',
      'Temperatura (°C)',
      'Rede recebida (bytes/s)',
      'Rede enviada (bytes/s)',
      'RTT (ms)',
    ]));
    expect(opcoes).not.toContain('Handshake SSH (ms)');
  });

  it('mostra o limiar de taxa em unidade legível', async () => {
    renderizar();

    expect(await screen.findByText(/Rede recebida \(bytes\/s\) > 10 MB\/s/)).toBeTruthy();
  });
});

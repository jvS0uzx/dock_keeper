import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';

import FloorPlanView from './FloorPlanView';
import { SessionContext, type SessionState } from './ui/session-context';
import { SiteScopeContext, type SiteScopeState } from './ui/site-scope-context';
import { DialogContext, type DialogApi } from './ui/dialog-context';
import { NavigationContext } from './ui/navigation-context';

const api = vi.hoisted(() => ({
  floorPlans: vi.fn(async () => [{ id: 3, name: 'Térreo', site_id: 1, created_at: '', pins: [] }]),
  floorPlan: vi.fn(() => Promise.reject(new Error(JSON.stringify({ error: 'marcadores indisponíveis' })))),
  floorPlanImageUrl: vi.fn(async () => 'blob:planta'),
  networkHosts: vi.fn(async () => ({ hosts: [], scan_active: true, scan_cidrs: [], last_scan: null })),
}));

vi.mock('../lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../lib/api')>()),
  api,
}));

describe('FloorPlanView — marcadores', () => {
  it('falha ao ler os marcadores aparece, não "0 marcador(es)"', async () => {
    const sessao: SessionState = { username: 'p', role: 'admin', accesses: [{ site_id: null, role: 'admin' }], isToken: false, logout: vi.fn() };
    const escopo: SiteScopeState = { siteId: '1', numericSiteId: 1, setSiteId: vi.fn(), sites: [], siteName: () => 'Matriz', reloadSites: vi.fn() };
    const dialogo: DialogApi = { confirm: vi.fn(), prompt: vi.fn(), notify: vi.fn() };
    render(
      <SessionContext.Provider value={sessao}>
        <SiteScopeContext.Provider value={escopo}>
          <DialogContext.Provider value={dialogo}>
            <NavigationContext.Provider value={{ openSite: vi.fn(), openMachine: vi.fn(), goBack: vi.fn() }}>
              <FloorPlanView />
            </NavigationContext.Provider>
          </DialogContext.Provider>
        </SiteScopeContext.Provider>
      </SessionContext.Provider>,
    );

    expect(await screen.findByText(/marcadores indisponíveis/)).toBeTruthy();
    expect(screen.queryByText(/marcador\(es\) nesta planta/)).toBeNull();
  });
});

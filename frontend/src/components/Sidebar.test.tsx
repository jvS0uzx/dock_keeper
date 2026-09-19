import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';

import Sidebar from './Sidebar';
import { SessionContext, type SessionState } from './ui/session-context';
import { SiteScopeContext, type SiteScopeState } from './ui/site-scope-context';
import type { Role, SiteAccess } from '../lib/session';
import type { PanelId } from '../lib/panels';

const escopoVazio: SiteScopeState = {
  siteId: 'all',
  numericSiteId: null,
  setSiteId: vi.fn(),
  sites: [],
  siteName: () => '',
  reloadSites: vi.fn(),
};

const renderComSessao = (role: Role, accesses: SiteAccess[], panel: PanelId = 'dev', sitesError: string | null = null) => {
  const sessao: SessionState = {
    username: 'pessoa-de-teste',
    role,
    accesses,
    isToken: false,
    logout: vi.fn(),
  };

  return render(
    <SessionContext.Provider value={sessao}>
      <SiteScopeContext.Provider value={{ ...escopoVazio, sitesError }}>
        <Sidebar activeTab="dashboard" setActiveTab={vi.fn()} panel={panel} setPanel={vi.fn()} />
      </SiteScopeContext.Provider>
    </SessionContext.Provider>,
  );
};

const ABAS_DE_ADMIN = ['Servidores', 'Usuários', 'Log de Auditoria'];

describe('Sidebar — gate de papel', () => {
  it('admin de uma unidade não vê as abas de administração', () => {
    renderComSessao('admin', [{ site_id: 7, role: 'admin' }]);

    for (const aba of ABAS_DE_ADMIN) {
      expect(screen.queryByText(aba)).toBeNull();
    }
  });

  it('admin global vê as três abas de administração', () => {
    renderComSessao('admin', [{ site_id: null, role: 'admin' }]);

    for (const aba of ABAS_DE_ADMIN) {
      expect(screen.getByText(aba)).toBeTruthy();
    }
  });

  it('visualizador não vê as abas de administração', () => {
    renderComSessao('viewer', [{ site_id: null, role: 'viewer' }]);

    for (const aba of ABAS_DE_ADMIN) {
      expect(screen.queryByText(aba)).toBeNull();
    }
  });

  it('Suporte TI opera mas não administra', () => {
    renderComSessao('operator', [{ site_id: null, role: 'operator' }]);

    expect(screen.getByText('Containers')).toBeTruthy();
    expect(screen.getByText('SSL & Domínios')).toBeTruthy();

    for (const aba of ABAS_DE_ADMIN) {
      expect(screen.queryByText(aba)).toBeNull();
    }
  });

  it('mostra o rótulo do papel em português ao lado do usuário', () => {
    renderComSessao('operator', [{ site_id: null, role: 'operator' }]);

    const rodape = screen.getByText('pessoa-de-teste').parentElement;
    expect(rodape?.textContent).toContain('Suporte TI');
    expect(rodape?.textContent).not.toContain('operator');
  });

  it('operador global não abre as abas de administração', () => {
    renderComSessao('viewer', [
      { site_id: null, role: 'operator' },
      { site_id: 3, role: 'admin' },
    ]);

    for (const aba of ABAS_DE_ADMIN) {
      expect(screen.queryByText(aba)).toBeNull();
    }
  });

  it('visualizador não vê a aba Dispositivos', () => {
    renderComSessao('viewer', [{ site_id: null, role: 'viewer' }], 'suporte');

    expect(screen.getByText('Estações')).toBeTruthy();
    expect(screen.queryByText('Dispositivos')).toBeNull();
  });

  it('admin de uma unidade não vê a aba Dispositivos', () => {
    renderComSessao('admin', [{ site_id: 7, role: 'admin' }], 'suporte');

    expect(screen.queryByText('Dispositivos')).toBeNull();
  });

  it('admin global vê a aba Dispositivos no painel de suporte', () => {
    renderComSessao('admin', [{ site_id: null, role: 'admin' }], 'suporte');

    expect(screen.getByText('Dispositivos')).toBeTruthy();
  });

  it('a aba Painéis aparece no painel Infra / Dev para qualquer papel', () => {
    renderComSessao('viewer', [{ site_id: null, role: 'viewer' }], 'dev');

    expect(screen.getByText('Painéis')).toBeTruthy();
  });

  it('falha ao listar unidades aparece sob o seletor', () => {
    renderComSessao('viewer', [{ site_id: null, role: 'viewer' }], 'suporte', 'unidades indisponíveis');

    expect(screen.getByRole('alert').textContent).toBe('unidades indisponíveis');
  });
});

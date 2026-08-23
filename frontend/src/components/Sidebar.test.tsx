import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';

import Sidebar from './Sidebar';
import { SessionContext, type SessionState } from './ui/session-context';
import { SiteScopeContext, type SiteScopeState } from './ui/site-scope-context';
import type { Role, SiteAccess } from '../lib/session';

const escopoVazio: SiteScopeState = {
  siteId: 'all',
  numericSiteId: null,
  setSiteId: vi.fn(),
  sites: [],
  siteName: () => '',
  reloadSites: vi.fn(),
};

const renderComSessao = (role: Role, accesses: SiteAccess[]) => {
  const sessao: SessionState = {
    username: 'pessoa-de-teste',
    role,
    accesses,
    isToken: false,
    logout: vi.fn(),
  };

  return render(
    <SessionContext.Provider value={sessao}>
      <SiteScopeContext.Provider value={escopoVazio}>
        <Sidebar activeTab="dashboard" setActiveTab={vi.fn()} panel="dev" setPanel={vi.fn()} />
      </SiteScopeContext.Provider>
    </SessionContext.Provider>,
  );
};

// As três abas que o backend gateia com requireGlobalRole(admin).
const ABAS_DE_ADMIN = ['Servidores', 'Usuários', 'Log de Auditoria'];

describe('Sidebar — gate de papel', () => {
  // Administrar uma filial não vira administrar o parque. Esta é a regressão do
  // item C3: o menu gateava pelo papel da CONTA, então admin de uma unidade via
  // as três abas e só descobria o 403 ao clicar.
  it('admin de uma unidade não vê as abas de administração', () => {
    renderComSessao('admin', [{ site_id: 7, role: 'admin' }]);

    for (const aba of ABAS_DE_ADMIN) {
      expect(screen.queryByText(aba)).toBeNull();
    }
  });

  // O contraponto que impede o teste acima de ser satisfeito por um menu que
  // esconde tudo.
  it('admin global vê as três abas de administração', () => {
    renderComSessao('admin', [{ site_id: null, role: 'admin' }]);

    for (const aba of ABAS_DE_ADMIN) {
      expect(screen.getByText(aba)).toBeTruthy();
    }
  });

  // Visualizador é estritamente somente-leitura: nem as abas de administração,
  // nem nada que dependa de concessão global.
  it('visualizador não vê as abas de administração', () => {
    renderComSessao('viewer', [{ site_id: null, role: 'viewer' }]);

    for (const aba of ABAS_DE_ADMIN) {
      expect(screen.queryByText(aba)).toBeNull();
    }
  });

  // "Suporte TI" é como o papel operator se chama na interface. Ele opera, mas
  // não administra: cadastro de servidor SSH e de usuário continuam fora.
  it('Suporte TI opera mas não administra', () => {
    renderComSessao('operator', [{ site_id: null, role: 'operator' }]);

    // As telas de operação continuam lá.
    expect(screen.getByText('Containers')).toBeTruthy();
    expect(screen.getByText('SSL & Domínios')).toBeTruthy();

    for (const aba of ABAS_DE_ADMIN) {
      expect(screen.queryByText(aba)).toBeNull();
    }
  });

  // O papel aparece na interface pelo rótulo em português, não pelo
  // identificador da API. "operator" na tela seria vocabulário de código
  // vazando para o técnico de campo.
  //
  // A busca é feita a partir do nome do usuário porque "Suporte TI" também é o
  // nome de um dos painéis no seletor acima: procurar o texto solto acha os
  // dois e o teste falha por ambiguidade, não por defeito.
  it('mostra o rótulo do papel em português ao lado do usuário', () => {
    renderComSessao('operator', [{ site_id: null, role: 'operator' }]);

    const rodape = screen.getByText('pessoa-de-teste').parentElement;
    expect(rodape?.textContent).toContain('Suporte TI');
    expect(rodape?.textContent).not.toContain('operator');
  });

  // Concessão global de papel MENOR que admin não abre as abas. A comparação é
  // por posto, não por "tem alguma concessão global" — um operador global
  // passaria por um teste de presença.
  it('operador global não abre as abas de administração', () => {
    renderComSessao('viewer', [
      { site_id: null, role: 'operator' },
      { site_id: 3, role: 'admin' },
    ]);

    for (const aba of ABAS_DE_ADMIN) {
      expect(screen.queryByText(aba)).toBeNull();
    }
  });
});

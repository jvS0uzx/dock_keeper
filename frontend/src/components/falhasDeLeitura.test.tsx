import type { ReactElement } from 'react';
import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';

import StationsView from './StationsView';
import ServersView from './ServersView';
import UsersView from './UsersView';
import AlertRulesView from './AlertRulesView';
import SitesView from './SitesView';
import SslView from './SslView';
import NetworkView from './NetworkView';
import NginxView from './NginxView';
import LogsView from './LogsView';
import SecurityView from './SecurityView';
import SiteDetailView from './SiteDetailView';
import FloorPlanView from './FloorPlanView';
import Dashboard from './Dashboard';
import { SessionContext, type SessionState } from './ui/session-context';
import { SiteScopeContext, type SiteScopeState } from './ui/site-scope-context';
import { DialogContext, type DialogApi } from './ui/dialog-context';
import { NavigationContext } from './ui/navigation-context';

const { falha } = vi.hoisted(() => ({
  falha: () => Promise.reject(new Error(JSON.stringify({ error: 'painel fora do ar' }))),
}));

vi.mock('../lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../lib/api')>()),
  api: new Proxy({}, { get: () => () => falha() }),
  openStream: () => falha(),
}));

const renderizar = (tela: ReactElement, numericSiteId: number | null = null) => {
  const sessao: SessionState = {
    username: 'p',
    role: 'admin',
    accesses: [{ site_id: null, role: 'admin' }],
    isToken: false,
    logout: vi.fn(),
  };
  const escopo: SiteScopeState = {
    siteId: numericSiteId === null ? 'all' : String(numericSiteId),
    numericSiteId,
    setSiteId: vi.fn(),
    sites: [],
    siteName: () => 'Matriz',
    reloadSites: vi.fn(),
  };
  const dialogo: DialogApi = { confirm: vi.fn(), prompt: vi.fn(), notify: vi.fn() };
  render(
    <SessionContext.Provider value={sessao}>
      <SiteScopeContext.Provider value={escopo}>
        <DialogContext.Provider value={dialogo}>
          <NavigationContext.Provider value={{ openSite: vi.fn(), openMachine: vi.fn(), goBack: vi.fn() }}>
            {tela}
          </NavigationContext.Provider>
        </DialogContext.Provider>
      </SiteScopeContext.Provider>
    </SessionContext.Provider>,
  );
};

const CASOS: [string, () => ReactElement, RegExp | null, number | null][] = [
  ['Estações', () => <StationsView />, /Nenhuma estação com agente/, null],
  ['Servidores', () => <ServersView />, /Nenhum servidor cadastrado/, null],
  ['Usuários', () => <UsersView />, /Nenhuma conta cadastrada/, null],
  ['Regras de alerta', () => <AlertRulesView />, /Nenhuma regra cadastrada/, null],
  ['Unidades', () => <SitesView />, /Nenhuma unidade cadastrada/, null],
  ['SSL', () => <SslView />, /Nenhum domínio monitorado/, null],
  ['Inventário de rede', () => <NetworkView />, null, null],
  ['Nginx', () => <NginxView />, /Aguardando tráfego/, null],
  ['Logs', () => <LogsView />, null, null],
  ['Segurança', () => <SecurityView />, null, null],
  ['Detalhe da unidade', () => <SiteDetailView siteId={1} />, /Nenhuma máquina com agente|Nenhuma regra de alerta cobre/, null],
  ['Planta baixa', () => <FloorPlanView />, null, 1],
  ['Dashboard', () => <Dashboard />, /Nenhum host online reportando disco/, null],
];

describe('falha de leitura aparece como erro, nunca como lista vazia', () => {
  it.each(CASOS)('%s', async (_, tela, vazio, unidade) => {
    renderizar(tela(), unidade);

    expect((await screen.findAllByText(/painel fora do ar/)).length).toBeGreaterThan(0);
    if (vazio) expect(screen.queryByText(vazio)).toBeNull();
  });
});

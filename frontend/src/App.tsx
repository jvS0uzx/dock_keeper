import { Suspense, lazy, useCallback, useEffect, useMemo, useState, type ComponentType } from 'react';
import Sidebar from './components/Sidebar';
import Dashboard from './components/Dashboard';
import LoginView from './components/LoginView';
import { DialogProvider } from './components/ui/DialogProvider';
import { SessionContext, type SessionState } from './components/ui/session-context';
import { NavigationContext, type NavigationState } from './components/ui/navigation-context';
import { ALL_SITES, SiteScopeContext, type SiteScopeState } from './components/ui/site-scope-context';
import { api, type Site } from './lib/api';
import { ADMIN_TABS, PANELS, hasGlobalAdmin, loadPanel, savePanel, type PanelId } from './lib/panels';
import {
  SESSION_EXPIRED_EVENT,
  clearSession,
  loadSession,
  type SessionInfo,
} from './lib/session';
import { API_TOKEN } from './config';

// Só o Dashboard entra no bundle inicial; as demais telas (e o recharts que
// algumas delas carregam) chegam sob demanda ao trocar de aba.
const MetricsHistoryView = lazy(() => import('./components/MetricsHistoryView'));
const ContainersView = lazy(() => import('./components/ContainersView'));
const NginxView = lazy(() => import('./components/NginxView'));
const SslView = lazy(() => import('./components/SslView'));
const AlertRulesView = lazy(() => import('./components/AlertRulesView'));
const LogsView = lazy(() => import('./components/LogsView'));
const SecurityView = lazy(() => import('./components/SecurityView'));
const ServersView = lazy(() => import('./components/ServersView'));
const NetworkView = lazy(() => import('./components/NetworkView'));
const FloorPlanView = lazy(() => import('./components/FloorPlanView'));
const StationsView = lazy(() => import('./components/StationsView'));
const SitesView = lazy(() => import('./components/SitesView'));
const UsersView = lazy(() => import('./components/UsersView'));
const SiteDetailView = lazy(() => import('./components/SiteDetailView'));
const MachineDetailView = lazy(() => import('./components/MachineDetailView'));
const AuditView = lazy(() => import('./components/AuditView'));

const VIEWS: Record<string, ComponentType> = {
  dashboard: Dashboard,
  history: MetricsHistoryView,
  containers: ContainersView,
  nginx: NginxView,
  ssl: SslView,
  alerts: AlertRulesView,
  logs: LogsView,
  security: SecurityView,
  servers: ServersView,
  network: NetworkView,
  floorplan: FloorPlanView,
  stations: StationsView,
  sites: SitesView,
  users: UsersView,
  audit: AuditView,
};

/** Tela de detalhe aberta sobre a aba atual. */
type Detail = { kind: 'site'; id: number } | { kind: 'machine'; id: string } | null;

function App() {
  const [panel, setPanel] = useState<PanelId>(loadPanel);
  const [activeTab, setActiveTab] = useState(() => PANELS[loadPanel()].tabs[0]);
  const [session, setSession] = useState<SessionInfo | null>(loadSession);
  const [loginNotice, setLoginNotice] = useState('');

  const [detail, setDetail] = useState<Detail>(null);
  const [siteId, setSiteId] = useState<string>(ALL_SITES);
  const [sites, setSites] = useState<Site[]>([]);

  // Sessão vencida ou revogada: o cliente da API limpa o storage e avisa por
  // evento — aqui só resta voltar para a tela de entrada.
  useEffect(() => {
    const onExpired = () => {
      setSession(null);
      setLoginNotice('Sessão expirada. Entre novamente.');
    };
    window.addEventListener(SESSION_EXPIRED_EVENT, onExpired);
    return () => window.removeEventListener(SESSION_EXPIRED_EVENT, onExpired);
  }, []);

  // Trocar de painel precisa levar a uma tela que ele contenha; senão o
  // conteúdo continuaria numa aba que sumiu do menu.
  useEffect(() => {
    savePanel(panel);
    setActiveTab(current => (PANELS[panel].tabs.includes(current) ? current : PANELS[panel].tabs[0]));
    setDetail(null);
  }, [panel]);

  const reloadSites = useCallback(() => {
    api.sites()
      .then(list => {
        setSites(list);
        // Unidade removida enquanto estava selecionada: volta para "todas" em
        // vez de manter um filtro que não existe mais.
        setSiteId(current =>
          current !== ALL_SITES && !list.some(s => String(s.id) === current) ? ALL_SITES : current,
        );
      })
      .catch(() => {});
  }, []);

  const handleLogin = useCallback((next: SessionInfo) => {
    setSession(next);
    setLoginNotice('');
    // O papel pode ter mudado entre logins; volta para uma aba que todos veem.
    setActiveTab(PANELS[loadPanel()].tabs[0]);
    setDetail(null);
    setSiteId(ALL_SITES);
  }, []);

  const handleLogout = useCallback(() => {
    // Invalida no servidor antes de esquecer o token; falha de rede não pode
    // prender a pessoa logada.
    api.logout().catch(() => {});
    clearSession();
    setSession(null);
    setLoginNotice('');
    setActiveTab(PANELS[loadPanel()].tabs[0]);
    setDetail(null);
    setSiteId(ALL_SITES);
    setSites([]);
  }, []);

  // Modo token: sem sessão, mas com VITE_API_TOKEN no ambiente. Existe para o
  // desenvolvimento não parar na tela de login a cada recarga; em produção o
  // config zera o token, este ramo nunca corre e a entrada é sempre o login.
  const sessionState = useMemo<SessionState | null>(() => {
    if (session) {
      return {
        username: session.username,
        role: session.role,
        accesses: session.accesses,
        isToken: false,
        logout: handleLogout,
      };
    }
    if (API_TOKEN) {
      return {
        // O sufixo aparece na barra lateral: quem vir esta sessão precisa saber
        // que ela só existe no ambiente de desenvolvimento.
        username: 'api-token (dev)',
        role: 'admin',
        accesses: [{ site_id: null, role: 'admin' }],
        isToken: true,
        logout: handleLogout,
      };
    }
    return null;
  }, [session, handleLogout]);

  const navigation = useMemo<NavigationState>(() => ({
    openSite: (id) => setDetail({ kind: 'site', id }),
    openMachine: (id) => setDetail({ kind: 'machine', id }),
    goBack: () => setDetail(null),
  }), []);

  const siteScope = useMemo<SiteScopeState>(() => ({
    siteId,
    numericSiteId: siteId === ALL_SITES ? null : Number(siteId),
    setSiteId: (value) => {
      setSiteId(value);
      // Trocar de unidade invalida o detalhe aberto, que é de outra filial.
      setDetail(null);
    },
    sites,
    siteName: (id) => (id === null ? 'Sem unidade' : sites.find(s => s.id === id)?.name ?? '—'),
    reloadSites,
  }), [siteId, sites, reloadSites]);

  // A lista de unidades só faz sentido depois de autenticado.
  useEffect(() => {
    if (sessionState) reloadSites();
  }, [sessionState, reloadSites]);

  if (!sessionState) {
    return (
      <DialogProvider>
        <LoginView onLogin={handleLogin} notice={loginNotice} />
      </DialogProvider>
    );
  }

  // Aba de admin acessada por quem não tem concessão global (o alcance mudou no
  // meio do uso): volta para a primeira aba do painel em vez de renderizar uma
  // tela que o backend vai recusar.
  const effectiveTab =
    ADMIN_TABS.has(activeTab) && !hasGlobalAdmin(sessionState.accesses)
      ? PANELS[panel].tabs[0]
      : activeTab;
  const ActiveView = VIEWS[effectiveTab] ?? Dashboard;

  // A tela de detalhe ocupa o lugar da aba; o menu continua visível para o
  // operador saber de onde veio.
  const content = (() => {
    if (detail?.kind === 'site') return <SiteDetailView siteId={detail.id} />;
    if (detail?.kind === 'machine') return <MachineDetailView serverId={detail.id} />;
    return <ActiveView />;
  })();

  const openTab = (tab: string) => {
    setActiveTab(tab);
    setDetail(null);
  };

  return (
    <DialogProvider>
      <SessionContext.Provider value={sessionState}>
        <SiteScopeContext.Provider value={siteScope}>
          <NavigationContext.Provider value={navigation}>
            <div className="flex h-screen w-screen overflow-hidden bg-ink-950 font-sans text-text">
              <Sidebar activeTab={detail ? '' : effectiveTab} setActiveTab={openTab} panel={panel} setPanel={setPanel} />
              <main className="relative flex-1 overflow-y-auto bg-ink-950">
                <Suspense fallback={<div className="p-8 text-sm text-text-mut">Carregando tela...</div>}>
                  {/* A key reinicia a animação de entrada a cada troca de tela. */}
                  <div key={detail ? `${detail.kind}-${detail.id}` : effectiveTab} className="anim-rise">
                    {content}
                  </div>
                </Suspense>
              </main>
            </div>
          </NavigationContext.Provider>
        </SiteScopeContext.Provider>
      </SessionContext.Provider>
    </DialogProvider>
  );
}

export default App;

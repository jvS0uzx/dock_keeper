import { Suspense, lazy, useCallback, useEffect, useMemo, useRef, useState, type ComponentType } from 'react';
import Sidebar from './components/Sidebar';
import DegradacaoAviso from './components/DegradacaoAviso';
import Dashboard from './components/Dashboard';
import LoginView from './components/LoginView';
import { DialogProvider } from './components/ui/DialogProvider';
import { SessionContext, type SessionState } from './components/ui/session-context';
import { NavigationContext, type NavigationState } from './components/ui/navigation-context';
import { ALL_SITES, SiteScopeContext, type SiteScopeState } from './components/ui/site-scope-context';
import { api, apiErrorMessage, type Site } from './lib/api';
import { ADMIN_TABS, PANELS, hasGlobalAdmin, loadPanel, savePanel, type PanelId } from './lib/panels';
import {
  SESSION_EXPIRED_EVENT,
  clearSession,
  loadSession,
  type SessionInfo,
} from './lib/session';
import { API_TOKEN } from './config';
import {
  CAMINHO_LOGIN,
  abaDoCaminho,
  caminhoDaAba,
  caminhoDoDetalhe,
  detalheDoCaminho,
  painelDaAba,
  useRota,
} from './lib/rotas';

const MetricsHistoryView = lazy(() => import('./components/MetricsHistoryView'));
const ContainersView = lazy(() => import('./components/ContainersView'));
const NginxView = lazy(() => import('./components/NginxView'));
const SslView = lazy(() => import('./components/SslView'));
const AlertRulesView = lazy(() => import('./components/AlertRulesView'));
const AlertsView = lazy(() => import('./components/AlertsView'));
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
const DevicesView = lazy(() => import('./components/DevicesView'));
const DashboardsView = lazy(() => import('./components/DashboardsView'));

const VIEWS: Record<string, ComponentType> = {
  dashboard: Dashboard,
  history: MetricsHistoryView,
  containers: ContainersView,
  nginx: NginxView,
  ssl: SslView,
  alertas: AlertsView,
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
  devices: DevicesView,
  dashboards: DashboardsView,
};

function App() {
  const { caminho, navegar } = useRota();
  const [panel, setPanel] = useState<PanelId>(loadPanel);
  const [session, setSession] = useState<SessionInfo | null>(loadSession);
  const [modoToken, setModoToken] = useState(false);
  const [loginNotice, setLoginNotice] = useState('');
  const destinoRef = useRef<string | null>(null);
  const navegacoesRef = useRef(0);

  const [siteId, setSiteId] = useState<string>(ALL_SITES);
  const [sites, setSites] = useState<Site[]>([]);
  const [sitesError, setSitesError] = useState<string | null>(null);

  useEffect(() => {
    const onExpired = () => {
      setSession(null);
      setLoginNotice('Sessão expirada. Entre novamente.');
    };
    window.addEventListener(SESSION_EXPIRED_EVENT, onExpired);
    return () => window.removeEventListener(SESSION_EXPIRED_EVENT, onExpired);
  }, []);

  useEffect(() => {
    savePanel(panel);
  }, [panel]);

  const reloadSites = useCallback(() => {
    api.sites()
      .then(list => {
        setSites(list);
        setSiteId(current =>
          current !== ALL_SITES && !list.some(s => String(s.id) === current) ? ALL_SITES : current,
        );
        setSitesError(null);
      })
      .catch(err => setSitesError(apiErrorMessage(err, 'Falha ao listar as unidades.')));
  }, []);

  const inicialDoPainel = useCallback(
    (id: PanelId = loadPanel()) => caminhoDaAba(PANELS[id].tabs[0]),
    [],
  );

  const handleLogin = useCallback(
    (next: SessionInfo) => {
      setSession(next);
      setLoginNotice('');
      setSiteId(ALL_SITES);
      const destino = destinoRef.current ?? inicialDoPainel();
      destinoRef.current = null;
      navegar(destino, { substituir: true });
    },
    [inicialDoPainel, navegar],
  );

  const entrarComToken = useCallback(() => {
    setModoToken(true);
    setLoginNotice('');
    const destino = destinoRef.current ?? inicialDoPainel();
    destinoRef.current = null;
    navegar(destino, { substituir: true });
  }, [inicialDoPainel, navegar]);

  const handleLogout = useCallback(() => {
    api.logout().catch(() => {});
    clearSession();
    setSession(null);
    setModoToken(false);
    setLoginNotice('');
    setSiteId(ALL_SITES);
    setSites([]);
    destinoRef.current = null;
    navegar(CAMINHO_LOGIN, { substituir: true });
  }, [navegar]);

  const sessionState = useMemo<SessionState | null>(() => {
    if (session) {
      return {
        username: session.username,
        nome: session.nome ?? null,
        role: session.role,
        accesses: session.accesses,
        isToken: false,
        logout: handleLogout,
      };
    }
    if (modoToken && API_TOKEN) {
      return {
        username: 'api-token (dev)',
        role: 'admin',
        accesses: [{ site_id: null, role: 'admin' }],
        isToken: true,
        logout: handleLogout,
      };
    }
    return null;
  }, [session, modoToken, handleLogout]);

  const irPara = useCallback(
    (destino: string) => {
      navegacoesRef.current += 1;
      navegar(destino);
    },
    [navegar],
  );

  const navigation = useMemo<NavigationState>(() => ({
    openSite: (id) => irPara(caminhoDoDetalhe({ kind: 'site', id })),
    openMachine: (id) => irPara(caminhoDoDetalhe({ kind: 'machine', id })),
    goBack: () => {
      if (navegacoesRef.current > 0) {
        navegacoesRef.current -= 1;
        window.history.back();
        return;
      }
      navegar(inicialDoPainel(), { substituir: true });
    },
  }), [irPara, navegar, inicialDoPainel]);

  const siteScope = useMemo<SiteScopeState>(() => ({
    siteId,
    numericSiteId: siteId === ALL_SITES ? null : Number(siteId),
    setSiteId: (value) => setSiteId(value),
    sites,
    siteName: (id) => (id === null ? 'Sem unidade' : sites.find(s => s.id === id)?.name ?? '—'),
    reloadSites,
    sitesError,
  }), [siteId, sites, reloadSites, sitesError]);

  useEffect(() => {
    if (sessionState) reloadSites();
  }, [sessionState, reloadSites]);

  const detalhe = detalheDoCaminho(caminho);
  const abaDaURL = abaDoCaminho(caminho);
  const semPermissao =
    abaDaURL !== null &&
    ADMIN_TABS.has(abaDaURL) &&
    sessionState !== null &&
    !hasGlobalAdmin(sessionState.accesses);
  const abaValida = abaDaURL !== null && !semPermissao ? abaDaURL : null;

  useEffect(() => {
    if (abaValida === null) return;
    setPanel((atual) => painelDaAba(abaValida, atual));
  }, [abaValida]);

  useEffect(() => {
    if (!sessionState) {
      if (caminho !== CAMINHO_LOGIN) {
        destinoRef.current = caminho;
        navegar(CAMINHO_LOGIN, { substituir: true });
      }
      return;
    }
    if (caminho === CAMINHO_LOGIN || (abaValida === null && detalhe === null)) {
      navegar(inicialDoPainel(panel), { substituir: true });
    }
  }, [sessionState, caminho, abaValida, detalhe, panel, navegar, inicialDoPainel]);

  if (!sessionState) {
    return (
      <DialogProvider>
        <LoginView
          onLogin={handleLogin}
          notice={loginNotice}
          onTokenLogin={API_TOKEN ? entrarComToken : undefined}
        />
      </DialogProvider>
    );
  }

  const effectiveTab = abaValida ?? PANELS[panel].tabs[0];
  const ActiveView = VIEWS[effectiveTab] ?? Dashboard;

  const content = (() => {
    if (detalhe?.kind === 'site') return <SiteDetailView siteId={detalhe.id} />;
    if (detalhe?.kind === 'machine') return <MachineDetailView serverId={detalhe.id} />;
    return <ActiveView />;
  })();

  const openTab = (tab: string) => irPara(caminhoDaAba(tab));

  const trocarPainel = (id: PanelId) => {
    setPanel(id);
    irPara(caminhoDaAba(PANELS[id].tabs[0]));
  };

  return (
    <DialogProvider>
      <SessionContext.Provider value={sessionState}>
        <SiteScopeContext.Provider value={siteScope}>
          <NavigationContext.Provider value={navigation}>
            <div className="flex h-screen w-screen overflow-hidden bg-ink-950 font-sans text-text">
              <Sidebar
                activeTab={detalhe ? '' : effectiveTab}
                setActiveTab={openTab}
                panel={panel}
                setPanel={trocarPainel}
              />
              <main className="relative flex-1 overflow-y-auto bg-ink-950">
                <DegradacaoAviso irParaAlertas={() => openTab('alertas')} />
                <Suspense fallback={<div className="p-8 text-sm text-text-mut">Carregando tela...</div>}>
                  <div key={detalhe ? `${detalhe.kind}-${detalhe.id}` : effectiveTab} className="anim-rise">
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

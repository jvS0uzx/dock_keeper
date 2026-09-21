import {
  LayoutDashboard, LayoutGrid, Box, Globe, Lock, ShieldAlert, Server, LineChart, BellRing,
  ScrollText, Network, Map, MonitorSmartphone, Building2, Users, LogOut, KeyRound,
  FileClock, FingerprintPattern,
  type LucideIcon,
} from 'lucide-react';
import { useEffect, useState } from 'react';
import { api } from '../lib/api';
import { ADMIN_TABS, PANELS, PANEL_IDS, hasGlobalAdmin, type PanelId } from '../lib/panels';
import { ROLE_LABELS } from '../lib/session';
import { useSession } from './ui/session-context';
import { ALL_SITES, useSiteScope } from './ui/site-scope-context';
import Select from './ui/Select';
import logo from '../assets/dockkeeper.png';
import { POLL } from '../lib/polling';

interface SidebarProps {
  activeTab: string;
  setActiveTab: (tab: string) => void;
  panel: PanelId;
  setPanel: (panel: PanelId) => void;
}

const TABS: Record<string, { label: string; icon: LucideIcon }> = {
  dashboard: { label: 'Dashboard Geral', icon: LayoutDashboard },
  history: { label: 'Histórico de Métricas', icon: LineChart },
  dashboards: { label: 'Painéis', icon: LayoutGrid },
  containers: { label: 'Containers', icon: Box },
  nginx: { label: 'Nginx & Tráfego', icon: Globe },
  ssl: { label: 'SSL & Domínios', icon: Lock },
  security: { label: 'Segurança & Auditoria', icon: ShieldAlert },
  servers: { label: 'Servidores', icon: Server },
  stations: { label: 'Estações', icon: MonitorSmartphone },
  network: { label: 'Inventário de Rede', icon: Network },
  floorplan: { label: 'Planta Baixa', icon: Map },
  sites: { label: 'Unidades', icon: Building2 },
  devices: { label: 'Dispositivos', icon: FingerprintPattern },
  alertas: { label: 'Alertas', icon: BellRing },
  alerts: { label: 'Regras de Alerta', icon: BellRing },
  logs: { label: 'Logs & Busca', icon: ScrollText },
  users: { label: 'Usuários', icon: Users },
  audit: { label: 'Log de Auditoria', icon: FileClock },
};


const Sidebar = ({ activeTab, setActiveTab, panel, setPanel }: SidebarProps) => {
  const session = useSession();
  const { siteId, numericSiteId, setSiteId, sites, sitesError } = useSiteScope();
  const [alertasAbertos, setAlertasAbertos] = useState(0);

  useEffect(() => {
    let vivo = true;
    const carregar = async () => {
      try {
        const resumo = await api.alertsSummary(numericSiteId);
        if (vivo) setAlertasAbertos(resumo.open);
      } catch {
        return;
      }
    };
    carregar();
    const timer = setInterval(carregar, POLL.resumoDeAlertas);
    return () => {
      vivo = false;
      clearInterval(timer);
    };
  }, [numericSiteId]);

  const visibleTabs = PANELS[panel].tabs.filter(
    (id) => !ADMIN_TABS.has(id) || hasGlobalAdmin(session.accesses),
  );

  return (
    <aside className="flex h-full w-64 shrink-0 flex-col border-r border-line bg-ink-900">
      <div className="border-b border-line p-5">
        <div className="flex items-center gap-2.5">
          <img src={logo} alt="DockKeeper" width={32} height={32} className="h-8 w-8 object-contain" />
          <h1 className="text-lg font-bold tracking-tight text-text-hi">
            Dock<span className="text-accent">Keeper</span>
          </h1>
        </div>
        <p className="mt-1 text-xs text-text-mut">{PANELS[panel].description}</p>
      </div>

      <div className="border-b border-line p-3">
        <div className="grid grid-cols-2 gap-1 rounded-ctrl border border-line bg-ink-850 p-1">
          {PANEL_IDS.map((id) => (
            <button
              key={id}
              onClick={() => setPanel(id)}
              className={`rounded-[0.4rem] py-1.5 text-xs font-semibold transition-colors ${
                panel === id
                  ? 'bg-ink-750 text-text-hi'
                  : 'text-text-mut hover:text-text-hi'
              }`}
            >
              {PANELS[id].label}
            </button>
          ))}
        </div>
      </div>

      {panel === 'suporte' && (
        <div className="border-b border-line p-3">
          <label htmlFor="sidebar-site" className="eyebrow mb-1.5 block">
            Unidade
          </label>
          <Select
            id="sidebar-site"
            value={siteId}
            onChange={setSiteId}
            options={[
              { value: ALL_SITES, label: 'Todas as unidades' },
              ...sites.map((s) => ({ value: String(s.id), label: s.name })),
            ]}
          />
          {sitesError && (
            <p role="alert" className="mt-1.5 text-[11px] text-crit">{sitesError}</p>
          )}
        </div>
      )}

      <nav className="flex-1 overflow-y-auto py-3 custom-scrollbar">
        <ul className="flex flex-col gap-0.5 px-3">
          {visibleTabs.map((id) => {
            const tab = TABS[id];
            if (!tab) return null;

            const isActive = activeTab === id;
            const Icon = tab.icon;
            return (
              <li key={id}>
                <button
                  onClick={() => setActiveTab(id)}
                  className={`relative flex w-full items-center gap-2.5 rounded-ctrl px-3 py-2 text-sm transition-colors ${
                    isActive
                      ? 'bg-ink-850 font-medium text-text-hi'
                      : 'text-text-mut hover:bg-ink-850 hover:text-text-hi'
                  }`}
                >
                  {isActive && (
                    <span
                      aria-hidden="true"
                      className="absolute left-0 top-1/2 h-5 w-0.5 -translate-y-1/2 rounded-full bg-accent"
                    />
                  )}
                  <Icon
                    size={16}
                    strokeWidth={1.75}
                    className={isActive ? 'text-accent' : 'text-text-faint'}
                  />
                  <span className="truncate">{tab.label}</span>
                  {id === 'alertas' && alertasAbertos > 0 && (
                    <span
                      title={`${alertasAbertos} alerta(s) em aberto`}
                      className="ml-auto rounded-full bg-crit/15 px-1.5 py-0.5 text-xs font-medium text-crit"
                    >
                      {alertasAbertos}
                    </span>
                  )}
                </button>
              </li>
            );
          })}
        </ul>
      </nav>

      <div className="border-t border-line p-4">
        <div className="flex items-center gap-3">
          <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full border border-line bg-ink-800">
            <KeyRound size={14} strokeWidth={1.75} className="text-text-mut" />
          </div>
          <div className="min-w-0 flex-1">
            <p className="truncate text-xs font-medium text-text-hi" title={session.username}>
              {session.nome || session.username}
            </p>
            <p className="eyebrow mt-0.5">
              {session.isToken ? 'Token de API' : ROLE_LABELS[session.role]}
            </p>
          </div>
          {!session.isToken && (
            <button
              onClick={session.logout}
              title="Sair"
              className="rounded-ctrl border border-transparent p-2 text-text-faint transition-colors hover:border-line hover:text-crit"
            >
              <LogOut size={16} strokeWidth={1.75} />
              <span className="sr-only">Sair</span>
            </button>
          )}
        </div>
        <div className="eyebrow mt-3 text-center">v{__APP_VERSION__}</div>
      </div>
    </aside>
  );
};

export default Sidebar;

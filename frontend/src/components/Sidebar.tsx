import {
  LayoutDashboard, Box, Globe, Lock, ShieldAlert, Server, LineChart, BellRing,
  ScrollText, Network, Map, MonitorSmartphone, Building2, Users, LogOut, KeyRound,
  FileClock,
  type LucideIcon,
} from 'lucide-react';
import { ADMIN_TABS, PANELS, PANEL_IDS, hasGlobalAdmin, type PanelId } from '../lib/panels';
import { ROLE_LABELS } from '../lib/session';
import { useSession } from './ui/session-context';
import { ALL_SITES, useSiteScope } from './ui/site-scope-context';
import Select from './ui/Select';

interface SidebarProps {
  activeTab: string;
  setActiveTab: (tab: string) => void;
  panel: PanelId;
  setPanel: (panel: PanelId) => void;
}

// Catálogo único de telas. Cada painel escolhe quais mostrar e em que ordem,
// então uma tela usada pelos dois (logs, alertas) é definida uma vez só.
const TABS: Record<string, { label: string; icon: LucideIcon }> = {
  dashboard: { label: 'Dashboard Geral', icon: LayoutDashboard },
  history: { label: 'Histórico de Métricas', icon: LineChart },
  containers: { label: 'Containers', icon: Box },
  nginx: { label: 'Nginx & Tráfego', icon: Globe },
  ssl: { label: 'SSL & Domínios', icon: Lock },
  security: { label: 'Segurança & Auditoria', icon: ShieldAlert },
  servers: { label: 'Servidores', icon: Server },
  stations: { label: 'Estações', icon: MonitorSmartphone },
  network: { label: 'Inventário de Rede', icon: Network },
  floorplan: { label: 'Planta Baixa', icon: Map },
  sites: { label: 'Unidades', icon: Building2 },
  alerts: { label: 'Regras de Alerta', icon: BellRing },
  logs: { label: 'Logs & Busca', icon: ScrollText },
  users: { label: 'Usuários', icon: Users },
  audit: { label: 'Log de Auditoria', icon: FileClock },
};

const Sidebar = ({ activeTab, setActiveTab, panel, setPanel }: SidebarProps) => {
  const session = useSession();
  const { siteId, setSiteId, sites } = useSiteScope();

  // Abas de administração somem para quem não tem concessão GLOBAL de admin.
  // O backend gateia as três com requireGlobalRole(admin), então o papel da
  // conta não basta: admin de uma filial só receberia 403. Mostrar uma porta
  // trancada só gera chamado de suporte.
  const visibleTabs = PANELS[panel].tabs.filter(
    (id) => !ADMIN_TABS.has(id) || hasGlobalAdmin(session.accesses),
  );

  return (
    <aside className="flex h-full w-64 shrink-0 flex-col border-r border-line bg-ink-900">
      <div className="border-b border-line p-5">
        <h1 className="text-lg font-bold tracking-tight text-text-hi">
          Dock<span className="text-accent">Keeper</span>
        </h1>
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

      {/* O escopo por unidade só faz sentido no painel de campo: as VPS do
          painel Dev são infraestrutura e não pertencem a filial nenhuma. */}
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
            <p className="truncate text-xs font-medium text-text-hi">{session.username}</p>
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
        <div className="eyebrow mt-3 text-center">v2.1.0</div>
      </div>
    </aside>
  );
};

export default Sidebar;

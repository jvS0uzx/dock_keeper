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
  nginx: { label: 'Nginx & Tráfego (Read Only)', icon: Globe },
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
    <aside className="w-64 glass-panel border-r border-white/5 flex flex-col h-full bg-[#0c0c0e]/95 backdrop-blur-xl">
      <div className="p-6 border-b border-white/5">
        <h1 className="text-lg font-bold tracking-widest uppercase text-white drop-shadow-md">
          VD <span className="text-[#10b981]">Stats</span>
        </h1>
        <p className="text-[10px] text-[#737373] mt-1 tracking-widest uppercase">
          {PANELS[panel].description}
        </p>
      </div>

      <div className="p-3 border-b border-white/5">
        <div className="grid grid-cols-2 gap-1 bg-black/40 rounded-lg p-1 border border-white/5">
          {PANEL_IDS.map((id) => (
            <button
              key={id}
              onClick={() => setPanel(id)}
              className={`py-2 rounded-md text-[10px] font-bold tracking-widest uppercase transition-all ${
                panel === id ? 'bg-[#10b981]/20 text-[#10b981]' : 'text-[#737373] hover:text-white'
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
        <div className="p-3 border-b border-white/5">
          <label
            htmlFor="sidebar-site"
            className="text-[10px] text-[#737373] uppercase tracking-widest block mb-1.5"
          >
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

    <nav className="flex-1 overflow-y-auto py-4">
        <ul className="flex flex-col gap-1 px-3">
          {visibleTabs.map((id) => {
            const tab = TABS[id];
            if (!tab) return null;

            const isActive = activeTab === id;
            const Icon = tab.icon;
            return (
              <li key={id}>
                <button
                  onClick={() => setActiveTab(id)}
                  className={`w-full flex items-center gap-3 px-4 py-3 rounded-xl text-sm font-medium transition-all duration-300 ${
                    isActive
                      ? 'bg-[#10b981]/10 text-[#10b981] border border-[#10b981]/20 shadow-[0_0_15px_rgba(16,185,129,0.1)]'
                      : 'text-white/60 hover:bg-white/5 hover:text-white border border-transparent'
                  }`}
                >
                  <Icon size={18} className={isActive ? 'text-[#10b981]' : 'text-[#737373]'} />
                  <span className="truncate">{tab.label}</span>
                </button>
              </li>
            );
          })}
        </ul>
      </nav>

      <div className="p-4 border-t border-white/5">
        <div className="flex items-center gap-3">
          <div className="w-8 h-8 rounded-full bg-[#10b981]/10 border border-[#10b981]/20 flex items-center justify-center shrink-0">
            <KeyRound size={14} className="text-[#10b981]" />
          </div>
          <div className="flex-1 min-w-0">
            <p className="text-xs text-white/90 font-medium truncate">{session.username}</p>
            <p className="text-[10px] text-[#737373] tracking-wider uppercase">
              {session.isToken ? 'Token de API' : ROLE_LABELS[session.role]}
            </p>
          </div>
          {!session.isToken && (
            <button
              onClick={session.logout}
              title="Sair"
              className="p-2 rounded-lg text-[#737373] hover:text-rose-400 hover:bg-rose-400/10 transition-colors"
            >
              <LogOut size={16} />
              <span className="sr-only">Sair</span>
            </button>
          )}
        </div>
        <div className="text-[10px] text-center text-[#737373] tracking-wider uppercase mt-3">
          v2.1.0
        </div>
      </div>
    </aside>
  );
};

export default Sidebar;

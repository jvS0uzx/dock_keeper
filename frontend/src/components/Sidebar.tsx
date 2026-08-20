import { LayoutDashboard, Box, Globe, Lock, ShieldAlert, Server } from 'lucide-react';

interface SidebarProps {
  activeTab: string;
  setActiveTab: (tab: string) => void;
}

const Sidebar = ({ activeTab, setActiveTab }: SidebarProps) => {
  const menuItems = [
    { id: 'dashboard', label: 'Dashboard Geral', icon: LayoutDashboard },
    { id: 'containers', label: 'Containers', icon: Box },
    { id: 'nginx', label: 'Nginx & Tráfego (Read Only)', icon: Globe },
    { id: 'ssl', label: 'SSL & Domínios', icon: Lock },
    { id: 'security', label: 'Segurança & Auditoria', icon: ShieldAlert },
    { id: 'servers', label: 'Servidores', icon: Server },
  ];

  return (
    <aside className="w-64 glass-panel border-r border-white/5 flex flex-col h-full bg-[#0c0c0e]/95 backdrop-blur-xl">
      <div className="p-6 border-b border-white/5">
        <h1 className="text-lg font-bold tracking-widest uppercase text-white drop-shadow-md">
          VD <span className="text-[#10b981]">Stats</span>
        </h1>
        <p className="text-[10px] text-[#737373] mt-1 tracking-widest uppercase">Admin Dashboard</p>
      </div>

      <nav className="flex-1 overflow-y-auto py-4">
        <ul className="flex flex-col gap-1 px-3">
          {menuItems.map((item) => {
            const isActive = activeTab === item.id;
            const Icon = item.icon;
            return (
              <li key={item.id}>
                <button
                  onClick={() => setActiveTab(item.id)}
                  className={`w-full flex items-center gap-3 px-4 py-3 rounded-xl text-sm font-medium transition-all duration-300 ${
                    isActive
                      ? 'bg-[#10b981]/10 text-[#10b981] border border-[#10b981]/20 shadow-[0_0_15px_rgba(16,185,129,0.1)]'
                      : 'text-white/60 hover:bg-white/5 hover:text-white border border-transparent'
                  }`}
                >
                  <Icon size={18} className={isActive ? 'text-[#10b981]' : 'text-[#737373]'} />
                  <span className="truncate">{item.label}</span>
                </button>
              </li>
            );
          })}
        </ul>
      </nav>

      <div className="p-4 border-t border-white/5">
        <div className="text-[10px] text-center text-[#737373] tracking-wider uppercase">
          v2.0.0
        </div>
      </div>
    </aside>
  );
};

export default Sidebar;

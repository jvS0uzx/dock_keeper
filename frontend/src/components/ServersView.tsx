import { useState, useEffect } from 'react';

interface Server {
  id: string;
  name: string;
  host_ip: string;
  user: string;
  created_at: string;
}

interface ServerLiveStat {
  id: string;
  uptime: number;
  latency_ms: number;
}

const ServersView = () => {
  const [servers, setServers] = useState<Server[]>([]);
  const [liveStats, setLiveStats] = useState<Record<string, ServerLiveStat>>({});
  const [loading, setLoading] = useState(true);
  const [form, setForm] = useState({ name: '', host_ip: '', user: 'root' });

  const fetchServers = async () => {
    try {
      const res = await fetch('http://localhost:8080/api/servers');
      const data = await res.json();
      setServers(data || []);
    } catch (err) {
      console.error(err);
    } finally {
      setLoading(false);
    }
  };

  const fetchLiveStatus = async () => {
    try {
      const res = await fetch('http://localhost:8080/api/metrics/live');
      const data = await res.json();
      const statsMap: Record<string, ServerLiveStat> = {};
      if (data.servers) {
        data.servers.forEach((s: ServerLiveStat) => {
          statsMap[s.id] = s;
        });
      }
      setLiveStats(statsMap);
    } catch (err) {
      // ignora erro silenciosamente
    }
  };

  useEffect(() => {
    fetchServers();
    fetchLiveStatus();
    const interval = setInterval(fetchLiveStatus, 5000);
    return () => clearInterval(interval);
  }, []);

  const handleAdd = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!form.host_ip || !form.name) return;
    
    try {
      await fetch('http://localhost:8080/api/servers', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(form)
      });
      setForm({ name: '', host_ip: '', user: 'root' });
      fetchServers();
    } catch (err) {
      console.error(err);
    }
  };

  const handleDelete = async (id: string) => {
    if (!confirm('Deseja realmente remover este servidor? Os gráficos pararão imediatamente.')) return;
    try {
      await fetch(`http://localhost:8080/api/servers?id=${id}`, { method: 'DELETE' });
      fetchServers();
    } catch (err) {
      console.error(err);
    }
  };

  return (
    <div className="p-8">
      <h1 className="text-2xl font-light text-white mb-2">Gestão de <span className="font-bold">Servidores</span></h1>
      <p className="text-[#737373] text-sm mb-8">Adicione ou remova servidores VPS para monitoramento. As conexões SSH são iniciadas instantaneamente de forma Hot-Plug.</p>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        <div className="glass-panel p-6 rounded-xl border border-white/5 bg-white/[0.02] col-span-1 h-fit">
          <h2 className="text-sm font-bold tracking-widest text-[#737373] uppercase mb-6">Adicionar Servidor</h2>
          <form onSubmit={handleAdd} className="flex flex-col gap-4">
            <div>
              <label className="text-xs text-[#737373] block mb-1">Nome de Identificação</label>
              <input 
                type="text" 
                value={form.name} 
                onChange={(e) => setForm({...form, name: e.target.value})} 
                className="w-full bg-black/40 border border-white/10 rounded-lg px-4 py-2 text-sm text-white focus:outline-none focus:border-[#10b981] transition-colors"
                placeholder="Ex: VPS Produção"
                required
              />
            </div>
            <div>
              <label className="text-xs text-[#737373] block mb-1">Endereço IP</label>
              <input 
                type="text" 
                value={form.host_ip} 
                onChange={(e) => setForm({...form, host_ip: e.target.value})} 
                className="w-full bg-black/40 border border-white/10 rounded-lg px-4 py-2 text-sm text-white focus:outline-none focus:border-[#10b981] transition-colors"
                placeholder="Ex: 203.0.113.10"
                required
              />
            </div>
            <div>
              <label className="text-xs text-[#737373] block mb-1">Usuário SSH</label>
              <input 
                type="text" 
                value={form.user} 
                onChange={(e) => setForm({...form, user: e.target.value})} 
                className="w-full bg-black/40 border border-white/10 rounded-lg px-4 py-2 text-sm text-white focus:outline-none focus:border-[#10b981] transition-colors"
                placeholder="Padrão: root"
              />
            </div>
            <button 
              type="submit" 
              className="mt-2 bg-[#10b981]/20 hover:bg-[#10b981]/30 border border-[#10b981]/50 text-[#10b981] font-bold text-xs uppercase tracking-widest py-3 rounded-lg transition-all"
            >
              + Conectar VPS
            </button>
          </form>
        </div>

        <div className="glass-panel p-6 rounded-xl border border-white/5 bg-white/[0.02] col-span-2">
          <h2 className="text-sm font-bold tracking-widest text-[#737373] uppercase mb-6">Servidores Ativos</h2>
          {loading ? (
            <p className="text-sm text-[#737373]">Carregando...</p>
          ) : servers.length === 0 ? (
            <p className="text-sm text-[#737373]">Nenhum servidor cadastrado.</p>
          ) : (
            <div className="overflow-x-auto custom-scrollbar">
              <table className="w-full text-sm text-left border-collapse">
                <thead className="text-[10px] text-[#737373] uppercase tracking-widest bg-[#0c0c0e]">
                  <tr>
                    <th className="py-3 px-4 rounded-l">Status</th>
                    <th className="py-3 px-4">Nome</th>
                    <th className="py-3 px-4">IP</th>
                    <th className="py-3 px-4">Usuário</th>
                    <th className="py-3 px-4 text-right rounded-r">Ação</th>
                  </tr>
                </thead>
                <tbody>
                  {servers.map((s) => {
                    const isOnline = liveStats[s.id] && liveStats[s.id].uptime > 0;
                    return (
                    <tr key={s.id} className="border-b border-white/[0.03] hover:bg-white/[0.05] transition-all">
                      <td className="py-4 px-4">
                        <div className="flex items-center gap-2">
                          <span className={`w-2 h-2 rounded-full ${isOnline ? 'bg-[#10b981] animate-pulse' : 'bg-red-500'}`}></span>
                          <span className={`text-[10px] ${isOnline ? 'text-[#10b981]' : 'text-red-500'} font-bold tracking-widest uppercase`}>
                            {isOnline ? 'Online' : 'Offline'}
                          </span>
                        </div>
                      </td>
                      <td className="py-4 px-4 font-medium text-white/90">{s.name}</td>
                      <td className="py-4 px-4 text-[#737373] font-mono">{s.host_ip}</td>
                      <td className="py-4 px-4 text-[#737373]">{s.user}</td>
                      <td className="py-4 px-4 text-right">
                        <button 
                          onClick={() => handleDelete(s.id)}
                          className="text-xs text-red-400/80 hover:text-red-400 hover:underline tracking-wider"
                        >
                          Remover
                        </button>
                      </td>
                    </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          )}
        </div>
      </div>
    </div>
  );
};

export default ServersView;

import { useEffect, useState } from 'react';
import { Box, Search, Play, Square, RefreshCw } from 'lucide-react';

interface ContainerStat {
  server_id: string;
  docker_id: string;
  name: string;
  cpu: number;
  mem_used: number;
  mem_limit: number;
}

const ContainersView = () => {
  const [containers, setContainers] = useState<ContainerStat[]>([]);
  const [search, setSearch] = useState('');

  useEffect(() => {
    const fetchMetrics = () => {
      fetch('http://localhost:8080/api/metrics/live')
        .then(res => res.json())
        .then(data => {
          if (data && data.containers) setContainers(data.containers);
        })
        .catch(() => {});
    };
    fetchMetrics();
    const interval = setInterval(fetchMetrics, 3000);
    return () => clearInterval(interval);
  }, []);

  const filtered = containers.filter(c => c.name.toLowerCase().includes(search.toLowerCase()));

  return (
    <div className="p-4 md:p-8 min-h-full">
      <div className="flex flex-col md:flex-row justify-between items-start md:items-end mb-8 gap-4">
        <div>
          <h1 className="text-2xl font-light text-white mb-2 flex items-center gap-3">
            <Box className="text-[#10b981]" /> Gestão de <span className="font-bold">Containers</span>
          </h1>
          <p className="text-[#737373] text-sm">Visualize o consumo em tempo real de todos os containers distribuídos na sua infraestrutura.</p>
        </div>
        <div className="relative w-full md:w-64">
          <Search className="w-4 h-4 text-[#737373] absolute left-3 top-1/2 -translate-y-1/2" />
          <input 
            type="text" 
            placeholder="Buscar container..." 
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="w-full bg-[#0c0c0e] border border-white/10 rounded-lg pl-10 pr-4 py-2 text-sm text-white focus:outline-none focus:border-[#10b981] transition-colors"
          />
        </div>
      </div>

      <div className="glass-panel p-6 rounded-xl border border-white/5 bg-white/[0.02]">
        <div className="flex items-center justify-between mb-6">
          <h2 className="text-sm font-bold tracking-widest text-[#737373] uppercase">Containers Ativos ({filtered.length})</h2>
          <div className="flex gap-2 items-center bg-[#10b981]/10 px-2 py-1 rounded border border-[#10b981]/20">
            <span className="w-1.5 h-1.5 rounded-full bg-[#10b981] animate-pulse"></span>
            <span className="text-[10px] text-[#10b981] font-bold tracking-widest uppercase">Live Sync</span>
          </div>
        </div>

        <div className="overflow-x-auto custom-scrollbar">
          <table className="w-full text-sm text-left border-collapse min-w-[600px]">
            <thead className="text-[10px] text-[#737373] uppercase tracking-widest bg-[#0c0c0e]">
              <tr>
                <th className="py-3 px-4 rounded-l">Status</th>
                <th className="py-3 px-4">Nome do Container</th>
                <th className="py-3 px-4 text-right">CPU Usage</th>
                <th className="py-3 px-4 text-right">Memória (RAM)</th>
                <th className="py-3 px-4 text-center rounded-r">Ações</th>
              </tr>
            </thead>
            <tbody>
              {filtered.length === 0 ? (
                <tr>
                  <td colSpan={5} className="py-8 text-center text-[#737373]">Nenhum container encontrado.</td>
                </tr>
              ) : (
                filtered.map((c, idx) => {
                  const formatBytes = (bytes: number) => {
                    if (bytes === 0) return '0 B';
                    const k = 1024;
                    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
                    const i = Math.floor(Math.log(bytes) / Math.log(k));
                    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
                  };
                  
                  const memUsedStr = formatBytes(c.mem_used);
                  const memLimitStr = formatBytes(c.mem_limit);
                  const memPercent = c.mem_limit > 0 ? ((c.mem_used / c.mem_limit) * 100).toFixed(1) : '0.0';
                  
                  return (
                    <tr key={`${c.docker_id}-${idx}`} className="border-b border-white/[0.03] hover:bg-white/[0.05] transition-all">
                      <td className="py-4 px-4">
                        <div className="flex items-center gap-2">
                          <span className="w-2 h-2 rounded-full bg-[#10b981] shadow-[0_0_5px_rgba(16,185,129,0.5)]"></span>
                          <span className="text-[10px] text-[#10b981] font-bold tracking-widest uppercase">Running</span>
                        </div>
                      </td>
                      <td className="py-4 px-4 font-mono text-[#f59e0b] text-[13px]">{c.name}</td>
                      <td className="py-4 px-4 text-right font-medium text-white/90">
                        <div className="flex flex-col items-end">
                          <span>{c.cpu.toFixed(2)}%</span>
                          <div className="w-16 h-1 bg-white/10 rounded-full mt-1 overflow-hidden">
                            <div className="h-full bg-[#10b981]" style={{ width: `${Math.min(c.cpu, 100)}%` }}></div>
                          </div>
                        </div>
                      </td>
                      <td className="py-4 px-4 text-right font-medium text-white/90">
                        <div className="flex flex-col items-end">
                          <span>{memUsedStr} <span className="text-[#737373] text-xs">/ {memLimitStr}</span></span>
                          <span className="text-[10px] text-[#10b981] mt-0.5">{memPercent}%</span>
                        </div>
                      </td>
                      <td className="py-4 px-4 text-center">
                        <div className="flex items-center justify-center gap-2">
                          <button className="p-1.5 hover:bg-white/10 rounded text-[#737373] hover:text-[#10b981] transition-colors" title="Restart (Em Breve)">
                            <RefreshCw size={14} />
                          </button>
                          <button className="p-1.5 hover:bg-white/10 rounded text-[#737373] hover:text-red-400 transition-colors" title="Stop (Em Breve)">
                            <Square size={14} />
                          </button>
                        </div>
                      </td>
                    </tr>
                  )
                })
              )}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  );
};

export default ContainersView;

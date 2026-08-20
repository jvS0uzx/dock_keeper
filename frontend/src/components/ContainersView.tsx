import { useEffect, useState, useRef } from 'react';
import { Box, Search, Play, Square, RefreshCw, Terminal, X, Cpu, MemoryStick } from 'lucide-react';

interface ContainerStat {
  server_id: string;
  docker_id: string;
  name: string;
  cpu: number;
  mem_used: number;
  mem_limit: number;
}

const formatBytes = (bytes: number) => {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
};

const ContainersView = () => {
  const [containers, setContainers] = useState<ContainerStat[]>([]);
  const [search, setSearch] = useState('');
  
  const [selectedContainer, setSelectedContainer] = useState<ContainerStat | null>(null);
  const [logs, setLogs] = useState<string[]>([]);
  const logsEndRef = useRef<HTMLDivElement>(null);
  const eventSourceRef = useRef<EventSource | null>(null);

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

  useEffect(() => {
    if (selectedContainer) {
      setLogs([]);
      const url = `http://localhost:8080/api/containers/logs/stream?server_id=${selectedContainer.server_id}&container_name=${selectedContainer.name}`;
      const es = new EventSource(url);
      
      es.onmessage = (event) => {
        setLogs(prev => [...prev, event.data].slice(-100)); // mantem ultimas 100 linhas
      };
      
      es.onerror = () => {
        setLogs(prev => [...prev, "[Conexão de Logs Encerrada]"]);
        es.close();
      };
      
      eventSourceRef.current = es;
    } else {
      if (eventSourceRef.current) {
        eventSourceRef.current.close();
        eventSourceRef.current = null;
      }
    }
    
    return () => {
      if (eventSourceRef.current) {
        eventSourceRef.current.close();
      }
    };
  }, [selectedContainer]);

  useEffect(() => {
    if (logsEndRef.current) {
      logsEndRef.current.scrollIntoView({ behavior: 'smooth' });
    }
  }, [logs]);

  const filtered = containers.filter(c => c.name.toLowerCase().includes(search.toLowerCase()));

  // Procura o container atualizado caso ele mude no live feed enquanto o modal esta aberto
  const liveSelected = selectedContainer ? containers.find(c => c.docker_id === selectedContainer.docker_id) || selectedContainer : null;

  return (
    <div className="p-4 md:p-8 min-h-full relative">
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
                          <button onClick={() => setSelectedContainer(c)} className="p-1.5 hover:bg-[#10b981]/20 rounded text-[#10b981] transition-colors border border-[#10b981]/30 bg-[#10b981]/10" title="Ver Logs ao Vivo">
                            <Terminal size={14} />
                          </button>
                          <button className="p-1.5 hover:bg-white/10 rounded text-[#737373] hover:text-white transition-colors" title="Restart (Em Breve)">
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

      {/* Modal de Logs */}
      {selectedContainer && liveSelected && (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/60 backdrop-blur-sm">
          <div className="w-full max-w-4xl h-[80vh] flex flex-col bg-[#0c0c0e] border border-white/10 rounded-xl shadow-2xl overflow-hidden">
            {/* Header Modal */}
            <div className="flex items-center justify-between p-4 border-b border-white/5 bg-[#1a1c23]">
              <div className="flex items-center gap-3">
                <Terminal className="text-[#10b981] w-5 h-5" />
                <h3 className="text-white font-bold font-mono tracking-wider">{liveSelected.name}</h3>
                <span className="px-2 py-0.5 rounded bg-[#10b981]/10 text-[#10b981] text-[10px] uppercase font-bold border border-[#10b981]/20">Live Stream</span>
              </div>
              <button onClick={() => setSelectedContainer(null)} className="text-[#737373] hover:text-white transition-colors">
                <X size={20} />
              </button>
            </div>
            
            {/* Consumo Isolado do Container */}
            <div className="grid grid-cols-2 gap-4 p-4 border-b border-white/5 bg-[#0c0c0e]">
              <div className="flex items-center gap-4 p-3 rounded-lg border border-white/5 bg-[#1a1c23]">
                <Cpu className="text-[#f59e0b] w-8 h-8" />
                <div>
                  <div className="text-[10px] text-[#737373] uppercase tracking-widest font-bold">Uso de CPU</div>
                  <div className="text-xl font-bold text-white">{liveSelected.cpu.toFixed(2)}%</div>
                </div>
              </div>
              <div className="flex items-center gap-4 p-3 rounded-lg border border-white/5 bg-[#1a1c23]">
                <MemoryStick className="text-blue-400 w-8 h-8" />
                <div>
                  <div className="text-[10px] text-[#737373] uppercase tracking-widest font-bold">Memória RAM</div>
                  <div className="text-xl font-bold text-white">{formatBytes(liveSelected.mem_used)} <span className="text-sm text-[#737373]">/ {formatBytes(liveSelected.mem_limit)}</span></div>
                </div>
              </div>
            </div>

            {/* Terminal de Logs */}
            <div className="flex-1 bg-black p-4 overflow-y-auto font-mono text-[12px] text-gray-300 leading-relaxed custom-scrollbar">
              {logs.length === 0 ? (
                <div className="text-[#737373] italic">Conectando ao container via SSH e puxando os logs...</div>
              ) : (
                logs.map((log, i) => <div key={i} className="whitespace-pre-wrap break-all">{log}</div>)
              )}
              <div ref={logsEndRef} />
            </div>
          </div>
        </div>
      )}
    </div>
  );
};

export default ContainersView;

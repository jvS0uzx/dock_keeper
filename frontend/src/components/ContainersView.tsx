import React, { useEffect, useState, useRef, useMemo } from 'react';
import { API_URL } from '../config';
import { Box, Search, Play, Square, RefreshCw, Terminal, X, Cpu, MemoryStick, ChevronDown, ChevronRight, Folder } from 'lucide-react';

interface ContainerStat {
  server_id: string;
  docker_id: string;
  name: string;
  project: string;
  state: string;
  status: string;
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
  const [expandedProjects, setExpandedProjects] = useState<Record<string, boolean>>({});
  
  const logsEndRef = useRef<HTMLDivElement>(null);
  const eventSourceRef = useRef<EventSource | null>(null);

  useEffect(() => {
    const fetchMetrics = () => {
      fetch(API_URL + '/api/metrics/live')
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
      const url = `${API_URL}/api/containers/logs/stream?server_id=${selectedContainer.server_id}&container_name=${selectedContainer.name}`;
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

  const toggleProject = (project: string) => {
    setExpandedProjects(prev => ({ ...prev, [project]: !prev[project] }));
  };

  const [actionBusy, setActionBusy] = useState<Record<string, boolean>>({});

  const containerAction = async (c: ContainerStat, action: 'start' | 'stop' | 'restart') => {
    if (action === 'stop' && !confirm(`Parar o container ${c.name}?`)) return;
    setActionBusy(prev => ({ ...prev, [c.docker_id]: true }));
    try {
      const res = await fetch(API_URL + '/api/containers/action', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ server_id: c.server_id, container_name: c.name, action }),
      });
      if (!res.ok) {
        const msg = await res.text();
        alert(`Falha ao ${action} ${c.name}: ${msg}`);
      }
    } catch (err) {
      alert(`Erro de rede ao ${action} ${c.name}`);
    } finally {
      setActionBusy(prev => ({ ...prev, [c.docker_id]: false }));
    }
  };

  const groupedContainers = useMemo(() => {
    const filtered = containers.filter(c => c.name.toLowerCase().includes(search.toLowerCase()));
    const groups: Record<string, ContainerStat[]> = {};
    
    filtered.forEach(c => {
      const proj = c.project || 'Sem Projeto (Avulsos)';
      if (!groups[proj]) groups[proj] = [];
      groups[proj].push(c);
    });
    
    return groups;
  }, [containers, search]);

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
          <h2 className="text-sm font-bold tracking-widest text-[#737373] uppercase">Projetos & Containers ({containers.length})</h2>
          <div className="flex gap-2 items-center bg-[#10b981]/10 px-2 py-1 rounded border border-[#10b981]/20">
            <span className="w-1.5 h-1.5 rounded-full bg-[#10b981] animate-pulse"></span>
            <span className="text-[10px] text-[#10b981] font-bold tracking-widest uppercase">Live Sync</span>
          </div>
        </div>

        <div className="overflow-x-auto custom-scrollbar">
          <table className="w-full text-sm text-left border-collapse min-w-[800px]">
            <thead className="text-[10px] text-[#737373] uppercase tracking-widest bg-[#0c0c0e]">
              <tr>
                <th className="py-3 px-4 rounded-l w-8"></th>
                <th className="py-3 px-4">Nome</th>
                <th className="py-3 px-4">Status / Uptime</th>
                <th className="py-3 px-4 text-right">CPU Usage</th>
                <th className="py-3 px-4 text-right">Memória (RAM)</th>
                <th className="py-3 px-4 text-center rounded-r">Ações</th>
              </tr>
            </thead>
            <tbody>
              {Object.keys(groupedContainers).length === 0 ? (
                <tr>
                  <td colSpan={6} className="py-8 text-center text-[#737373]">Nenhum container encontrado.</td>
                </tr>
              ) : (
                Object.entries(groupedContainers).map(([project, projContainers]) => {
                  const isExpanded = expandedProjects[project] !== false; // default true
                  
                  return (
                    <React.Fragment key={project}>
                      {/* Project Header Row */}
                      <tr 
                        className="border-b border-white/[0.05] bg-white/[0.01] hover:bg-white/[0.03] transition-colors cursor-pointer"
                        onClick={() => toggleProject(project)}
                      >
                        <td className="py-3 px-4 text-[#737373]">
                          {isExpanded ? <ChevronDown size={16} /> : <ChevronRight size={16} />}
                        </td>
                        <td colSpan={5} className="py-3 px-4">
                          <div className="flex items-center gap-2">
                            <Folder size={14} className="text-[#10b981]" />
                            <span className="font-bold text-white/90">{project}</span>
                            <span className="text-xs text-[#737373] bg-black/20 px-2 py-0.5 rounded border border-white/5">
                              {projContainers.length} containers
                            </span>
                          </div>
                        </td>
                      </tr>
                      
                      {/* Container Rows */}
                      {isExpanded && projContainers.map((c, idx) => {
                        const memUsedStr = formatBytes(c.mem_used);
                        const memLimitStr = formatBytes(c.mem_limit);
                        const memPercent = c.mem_limit > 0 ? ((c.mem_used / c.mem_limit) * 100).toFixed(1) : '0.0';
                        const isRunning = c.state === 'running';
                        
                        return (
                          <tr key={`${c.docker_id}-${idx}`} className="border-b border-white/[0.02] hover:bg-white/[0.04] transition-all">
                            <td className="py-4 px-4"></td>
                            <td className="py-4 px-4 font-mono text-[#f59e0b] text-[13px]">
                              <div className="flex items-center gap-3">
                                <div className="w-6 h-px bg-white/10" />
                                {c.name}
                              </div>
                            </td>
                            <td className="py-4 px-4">
                              <div className="flex flex-col gap-1">
                                <div className="flex items-center gap-2">
                                  <span className={`w-2 h-2 rounded-full shadow-[0_0_5px_currentColor] ${isRunning ? 'bg-[#10b981] text-[#10b981]' : 'bg-red-500 text-red-500'}`}></span>
                                  <span className={`text-[10px] font-bold tracking-widest uppercase ${isRunning ? 'text-[#10b981]' : 'text-red-500'}`}>
                                    {c.state || 'Unknown'}
                                  </span>
                                </div>
                                <span className="text-xs text-[#737373] whitespace-nowrap" title={c.status}>{c.status}</span>
                              </div>
                            </td>
                            <td className="py-4 px-4 text-right font-medium text-white/90">
                              {isRunning ? (
                                <div className="flex flex-col items-end">
                                  <span>{c.cpu.toFixed(2)}%</span>
                                  <div className="w-16 h-1 bg-white/10 rounded-full mt-1 overflow-hidden">
                                    <div className="h-full bg-[#10b981]" style={{ width: `${Math.min(c.cpu, 100)}%` }}></div>
                                  </div>
                                </div>
                              ) : (
                                <span className="text-[#737373] text-xs">-</span>
                              )}
                            </td>
                            <td className="py-4 px-4 text-right font-medium text-white/90">
                              {isRunning ? (
                                <div className="flex flex-col items-end">
                                  <span>{memUsedStr} <span className="text-[#737373] text-xs">/ {memLimitStr}</span></span>
                                  <span className="text-[10px] text-[#10b981] mt-0.5">{memPercent}%</span>
                                </div>
                              ) : (
                                <span className="text-[#737373] text-xs">-</span>
                              )}
                            </td>
                            <td className="py-4 px-4 text-center">
                              <div className="flex items-center justify-center gap-2">
                                <button onClick={() => setSelectedContainer(c)} className="p-1.5 hover:bg-[#10b981]/20 rounded text-[#10b981] transition-colors border border-[#10b981]/30 bg-[#10b981]/10" title="Ver Logs ao Vivo">
                                  <Terminal size={14} />
                                </button>
                                {isRunning ? (
                                  <>
                                    <button disabled={actionBusy[c.docker_id]} onClick={() => containerAction(c, 'restart')} className="p-1.5 hover:bg-white/10 rounded text-[#737373] hover:text-white transition-colors disabled:opacity-40" title="Restart">
                                      <RefreshCw size={14} className={actionBusy[c.docker_id] ? 'animate-spin' : ''} />
                                    </button>
                                    <button disabled={actionBusy[c.docker_id]} onClick={() => containerAction(c, 'stop')} className="p-1.5 hover:bg-white/10 rounded text-[#737373] hover:text-red-400 transition-colors disabled:opacity-40" title="Stop">
                                      <Square size={14} />
                                    </button>
                                  </>
                                ) : (
                                  <button disabled={actionBusy[c.docker_id]} onClick={() => containerAction(c, 'start')} className="p-1.5 hover:bg-white/10 rounded text-[#737373] hover:text-emerald-400 transition-colors disabled:opacity-40" title="Start">
                                    <Play size={14} />
                                  </button>
                                )}
                              </div>
                            </td>
                          </tr>
                        )
                      })}
                    </React.Fragment>
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
            {liveSelected.state === 'running' && (
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
            )}

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

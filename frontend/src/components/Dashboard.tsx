import { useEffect, useState } from 'react';
import { Clock, Filter, Radio, Globe, Server, Database, Activity, ArrowRight } from 'lucide-react';
import { AreaChart, Area, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer, PieChart, Pie, Cell } from 'recharts';

interface ContainerStat {
  server_id: string;
  docker_id: string;
  name: string;
  cpu: number;
  mem_used: number;
  mem_limit: number;
}

interface ServerStat {
  id: string;
  host_ip: string;
  name: string;
  uptime: number;
  disk_used: number;
  disk_total: number;
  latency_ms: number;
}

const Gauge = ({ value, title, color }: { value: number, title: string, color: string }) => {
  const data = [
    { name: 'value', value: value },
    { name: 'empty', value: Math.max(100 - value, 0) }
  ];
  return (
    <div className="glass-panel rounded-xl flex flex-col items-center justify-center relative p-6 h-full transition-all duration-300 hover:bg-white/[0.02] group">
       <span className="text-[#737373] text-[11px] font-bold absolute top-5 uppercase tracking-widest">{title}</span>
       <div className="w-full h-32 mt-6 relative transition-all duration-500" style={{ filter: `drop-shadow(0px 4px 8px ${color}1a)` }}>
         <ResponsiveContainer width="100%" height="100%">
           <PieChart>
             <Pie
               data={data}
               cx="50%"
               cy="100%"
               startAngle={180}
               endAngle={0}
               innerRadius={65}
               outerRadius={85}
               paddingAngle={0}
               dataKey="value"
               stroke="none"
               animationDuration={500}
               cornerRadius={4}
             >
               <Cell fill={color} />
               <Cell fill="rgba(255,255,255,0.03)" />
             </Pie>
           </PieChart>
         </ResponsiveContainer>
         <div className="absolute bottom-0 w-full flex justify-center">
           <span className="text-3xl font-bold text-white tracking-tight" style={{ textShadow: `0 0 10px ${color}33` }}>
             {value.toFixed(1)}{title.includes('%') ? '%' : ''}
           </span>
         </div>
       </div>
    </div>
  );
}

const TopWidget = ({ title, value, unit, type = 'success' }: { title: string, value: string | number, unit: string, type?: 'success' | 'danger' | 'warning' }) => {
  const colors = {
    success: { text: 'text-[#10b981]', glow: 'text-glow-green', bg: 'bg-[#10b981]', shadow: 'shadow-[0_0_8px_rgba(16,185,129,0.3)]' },
    danger: { text: 'text-[#ef4444]', glow: 'text-glow-red', bg: 'bg-[#ef4444]', shadow: 'shadow-[0_0_8px_rgba(239,68,68,0.3)]' },
    warning: { text: 'text-[#f59e0b]', glow: 'text-glow-yellow', bg: 'bg-[#f59e0b]', shadow: 'shadow-[0_0_8px_rgba(245,158,11,0.3)]' }
  };
  const theme = colors[type];

  return (
    <div className="glass-panel rounded-xl p-6 flex flex-col items-center justify-center relative overflow-hidden transition-all duration-300">
      <div className={`absolute top-0 left-0 w-full h-[2px] ${theme.bg} opacity-40 ${theme.shadow}`} />
      <span className="text-[#737373] mb-2 text-center tracking-widest font-bold uppercase text-[11px]">{title}</span>
      <div className="flex items-baseline gap-1">
        <span className={`text-5xl font-bold tracking-tight ${theme.text} ${theme.glow}`}>{value}</span>
        <span className={`text-xl font-bold ${theme.text} opacity-80`}>{unit}</span>
      </div>
    </div>
  );
};

const CustomSelect = ({ options, value, onChange }: { options: {value: string, label: string}[], value: string, onChange: (v: string) => void }) => {
  const [isOpen, setIsOpen] = useState(false);
  const selectedOption = options.find(o => o.value === value) || options[0];

  return (
    <div className="relative">
      <button 
        onClick={() => setIsOpen(!isOpen)}
        className="flex items-center gap-2 bg-[#0c0c0e] border border-white/10 hover:border-[#10b981]/50 text-sm text-white rounded-lg px-3 py-1.5 outline-none transition-all shadow-inner min-w-[200px] justify-between"
      >
        <span className="truncate">{selectedOption?.label}</span>
        <svg className={`w-4 h-4 transition-transform text-[#737373] ${isOpen ? 'rotate-180' : ''}`} fill="none" stroke="currentColor" viewBox="0 0 24 24"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" /></svg>
      </button>

      {isOpen && (
        <>
          <div className="fixed inset-0 z-40" onClick={() => setIsOpen(false)}></div>
          <div className="absolute top-full left-0 mt-2 w-full glass-panel !bg-[#0c0c0e]/95 border border-white/10 rounded-lg shadow-2xl z-50 overflow-hidden">
            {options.map(opt => (
              <div 
                key={opt.value}
                onClick={() => { onChange(opt.value); setIsOpen(false); }}
                className={`px-3 py-2.5 text-sm cursor-pointer transition-colors ${value === opt.value ? 'bg-[#10b981]/15 text-[#10b981] font-medium' : 'text-white/80 hover:bg-white/5'}`}
              >
                {opt.label}
              </div>
            ))}
          </div>
        </>
      )}
    </div>
  );
}

interface LbStat {
  upstream_addr: string;
  requests_count: number;
  server_name?: string;
  status?: string;
}

const LoadBalancerFlow = ({ stats }: { stats: LbStat[] }) => {
  const targetIps: string[] = (import.meta.env.VITE_TARGET_VPS_IPS || '').split(',').map((ip: string) => ip.trim()).filter(Boolean) as string[];
  const getReqs = (ip: string) => stats.filter(s => s.upstream_addr.includes(ip)).reduce((a, b) => a + b.requests_count, 0);
  const total = stats.reduce((acc, curr) => acc + curr.requests_count, 0);

  const getY = (index: number, totalNodes: number) => {
    if (totalNodes === 1) return 50;
    return 15 + (index * (70 / (totalNodes - 1)));
  };

  return (
    <div className="glass-panel rounded-xl p-6 mb-6 overflow-hidden relative">
      <div className="absolute inset-0 bg-[radial-gradient(ellipse_at_center,_var(--tw-gradient-stops))] from-[#10b981]/[0.03] to-transparent pointer-events-none" />
       
      <div className="flex items-center justify-between mb-8 pb-3 border-b border-white/[0.03] relative z-10">
        <span className="uppercase tracking-widest text-[11px] font-bold text-[#737373] flex items-center gap-2">
          <Activity className="w-4 h-4 text-[#10b981]" />
          Malha de Roteamento (NGINX Cluster)
        </span>
        <div className="flex gap-2 items-center bg-[#10b981]/5 px-3 py-1.5 rounded border border-[#10b981]/20 backdrop-blur-sm">
          <span className="w-1.5 h-1.5 rounded-full bg-[#10b981] animate-pulse shadow-[0_0_8px_#10b981]"></span>
          <span className="text-[10px] text-[#10b981] font-bold tracking-widest uppercase">
            {total} Req / 5s
          </span>
        </div>
      </div>

      <div className="relative w-full max-w-4xl mx-auto flex items-center justify-between min-h-[192px] md:px-4">
        <div className="z-10 flex flex-col items-center justify-center min-w-[80px]">
          <div className="w-14 h-14 rounded-full bg-gradient-to-br from-[#1a1c23] to-[#0c0c0e] border border-white/10 flex items-center justify-center shadow-lg relative group transition-all duration-500 hover:border-[#10b981]/50 hover:shadow-[0_0_20px_rgba(16,185,129,0.1)]">
            <Globe className="w-6 h-6 text-white/50 group-hover:text-white/80 transition-colors" strokeWidth={1.5} />
          </div>
          <span className="text-[10px] text-[#737373] mt-3 font-bold tracking-widest uppercase">Cloudflare</span>
        </div>

        <div className="flex-1 relative flex items-center -mx-4 z-0">
          <svg className="w-full h-12 overflow-visible" viewBox="0 0 100 100" preserveAspectRatio="none">
            <path id="path-in" d="M 0,50 L 100,50" fill="none" stroke="rgba(255,255,255,0.05)" strokeWidth="1" />
            {total > 0 && (
              <circle r="2" fill="#10b981" filter="drop-shadow(0 0 3px #10b981)">
                <animateMotion dur="1s" repeatCount="indefinite">
                  <mpath href="#path-in" />
                </animateMotion>
              </circle>
            )}
          </svg>
          <div className="absolute left-1/2 -translate-x-1/2 -top-4 bg-[#0c0c0e] px-2 py-0.5 rounded-full border border-white/5 text-[10px] text-white/60 font-mono flex items-center gap-1 shadow-md">
            {total} <ArrowRight className="w-3 h-3 text-[#10b981]" />
          </div>
        </div>

        <div className="z-10 flex flex-col items-center justify-center min-w-[80px]">
          <div className="w-16 h-16 rounded-2xl bg-gradient-to-br from-[#10b981]/10 to-[#0c0c0e] border border-[#10b981]/40 flex items-center justify-center shadow-[0_0_30px_rgba(16,185,129,0.15)] relative backdrop-blur-sm group transition-all duration-500 hover:scale-105 hover:border-[#10b981]">
            <div className="absolute inset-0 rounded-2xl bg-[#10b981]/5 blur-xl group-hover:bg-[#10b981]/10 transition-colors" />
            <Server className="w-7 h-7 text-[#10b981] relative z-10" strokeWidth={1.5} />
          </div>
          <span className="text-[10px] text-[#10b981] mt-3 font-bold tracking-widest uppercase">Load Balancer</span>
        </div>

        <div className="flex-1 relative -mx-4 h-full z-0 min-h-[192px]">
          <svg className="absolute inset-0 w-full h-full overflow-visible" viewBox="0 0 100 100" preserveAspectRatio="none">
            {targetIps.map((ip, idx) => {
              const y = getY(idx, targetIps.length);
              const reqs = getReqs(ip);
              return (
                <g key={`path-${ip}`}>
                  <path id={`path-v-${idx}`} d={`M 0,50 C 40,50 40,${y} 100,${y}`} fill="none" stroke="rgba(255,255,255,0.05)" strokeWidth="1" />
                  {reqs > 0 && Array.from({length: Math.min(reqs, 5)}).map((_, i) => (
                    <circle key={`v-${idx}-${i}`} r="2" fill="#10b981" filter="drop-shadow(0 0 3px #10b981)">
                      <animateMotion dur="1.5s" begin={`${i * 0.3}s`} repeatCount="indefinite">
                        <mpath href={`#path-v-${idx}`} />
                      </animateMotion>
                    </circle>
                  ))}
                </g>
              );
            })}
          </svg>

          {targetIps.map((ip, idx) => {
             const reqs = getReqs(ip);
             const y = getY(idx, targetIps.length);
             return (
               <div key={`badge-${ip}`} className="absolute right-[15%] bg-[#0c0c0e] px-2 py-0.5 rounded-full border border-white/5 text-[10px] text-[#10b981] font-mono shadow-md translate-y-[-50%]" style={{ top: `${y}%` }}>
                 {reqs} req
               </div>
             );
          })}
        </div>

        <div className="z-10 flex flex-col justify-around h-full py-[12px] min-w-[160px] md:min-w-[200px] gap-3">
          {targetIps.map((ip, idx) => (
            <div key={ip} className="w-full bg-gradient-to-r from-[#0c0c0e] to-[#1a1c23] border border-white/5 hover:border-white/20 transition-colors rounded-xl p-3 flex items-center gap-4 shadow-lg group relative h-[64px]">
              <div className="w-10 h-10 rounded-lg bg-white/5 flex items-center justify-center flex-shrink-0">
                <Database className="w-5 h-5 text-white/40 group-hover:text-[#10b981] transition-colors" />
              </div>
              <div className="flex flex-col">
                <span className="text-[10px] text-[#737373] font-bold tracking-widest uppercase">Node {idx + 1}</span>
                <span className="text-[11px] text-white/80 font-mono mt-0.5">{ip}</span>
              </div>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
};

const LoadBalancerDashboard = ({ stats }: { stats: LbStat[] }) => {
  const targetIps: string[] = (import.meta.env.VITE_TARGET_VPS_IPS || '').split(',').map((ip: string) => ip.trim()).filter(Boolean) as string[];
  
  const projectsMap = new Map<string, { total: number, errors: number, vpsCounts: Record<string, number>, local: number, algo: string }>();
  
  stats.forEach(s => {
    const proj = s.server_name || 'Desconhecido';
    if (!projectsMap.has(proj)) {
      const isWs = proj.includes('ws.') || proj.includes('soketi') || s.upstream_addr.includes('6001');
      projectsMap.set(proj, { total: 0, errors: 0, vpsCounts: {}, local: 0, algo: isWs ? 'IP_HASH (Sticky)' : 'LEAST_CONN' });
    }
    const data = projectsMap.get(proj)!;
    data.total += s.requests_count;
    if (['500', '502', '503', '504', '400', '404'].includes(s.status || '')) {
      data.errors += s.requests_count;
    }
    
    let matchedVps = false;
    for (const ip of targetIps) {
      if (s.upstream_addr.includes(ip)) {
        data.vpsCounts[ip] = (data.vpsCounts[ip] || 0) + s.requests_count;
        matchedVps = true;
        break;
      }
    }
    
    if (!matchedVps) {
      data.local += s.requests_count;
    }
  });

  const projects = Array.from(projectsMap.entries()).map(([name, data]) => ({ name, ...data })).sort((a, b) => b.total - a.total);
  const totalErrors = projects.reduce((acc, p) => acc + p.errors, 0);

  return (
    <div className="flex flex-col gap-6">
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4 md:gap-6">
        <div className="glass-panel rounded-xl p-6 relative overflow-hidden">
          <div className="absolute top-0 left-0 w-full h-[2px] bg-red-500 opacity-40 shadow-[0_0_8px_rgba(239,68,68,0.3)]" />
          <span className="text-[#737373] text-[11px] font-bold tracking-widest uppercase mb-2 block">Alertas Críticos (5s)</span>
          <div className="flex items-baseline gap-2">
            <span className={`text-5xl font-bold tracking-tight ${totalErrors > 0 ? 'text-red-500 text-glow-red' : 'text-[#10b981]'}`}>{totalErrors}</span>
            <span className="text-sm font-bold text-white/50">ERROS HTTP 5xx/4xx</span>
          </div>
          {totalErrors > 0 && <span className="text-xs text-red-400 mt-2 block animate-pulse">Verifique os logs no painel abaixo!</span>}
        </div>
        
        <div className="glass-panel rounded-xl p-6 relative overflow-hidden flex flex-col justify-center">
          <div className="absolute top-0 left-0 w-full h-[2px] bg-[#10b981] opacity-40 shadow-[0_0_8px_#10b981]" />
          <span className="text-[#737373] text-[11px] font-bold tracking-widest uppercase mb-2 block">Algoritmos Ativos (NGINX)</span>
          <div className="flex flex-col gap-2">
            <div className="flex justify-between items-center bg-[#0c0c0e] px-3 py-1.5 rounded border border-white/5">
               <span className="text-xs font-bold text-white/80">Tráfego HTTP/API</span>
               <span className="text-xs font-mono text-[#10b981]">LEAST_CONN</span>
            </div>
            <div className="flex justify-between items-center bg-[#0c0c0e] px-3 py-1.5 rounded border border-white/5">
               <span className="text-xs font-bold text-white/80">WebSockets (Kanban)</span>
               <span className="text-xs font-mono text-[#10b981]">IP_HASH</span>
            </div>
          </div>
        </div>
      </div>

      <div className="glass-panel rounded-xl p-6 overflow-hidden">
        <div className="flex items-center justify-between mb-4 border-b border-white/5 pb-3">
           <span className="uppercase tracking-widest text-[11px] font-bold text-[#737373] flex items-center gap-2">
             <Server className="w-4 h-4 text-[#10b981]" />
             Comportamento por Sistema (Projetos)
           </span>
        </div>
        
        <div className="overflow-x-auto">
          <table className="w-full text-sm text-left border-collapse">
            <thead className="text-[10px] text-[#737373] uppercase tracking-widest bg-[#0c0c0e]">
              <tr>
                <th className="py-3 px-3 rounded-l">Domínio / Sistema</th>
                <th className="py-3 px-3">Algoritmo</th>
                {targetIps.map((ip, idx) => (
                  <th key={ip} className="py-3 px-3 text-right">Node {idx + 1}</th>
                ))}
                <th className="py-3 px-3 text-right text-[#10b981]/80">Local/Cache</th>
                <th className="py-3 px-3 text-right">Total Req/5s</th>
                <th className="py-3 px-3 text-right rounded-r">Erros</th>
              </tr>
            </thead>
            <tbody>
              {projects.length === 0 && (
                <tr><td colSpan={5 + targetIps.length} className="text-center py-8 text-[#737373] text-xs">Sem tráfego nos últimos 5 segundos...</td></tr>
              )}
              {projects.map(p => (
                <tr key={p.name} className="border-b border-white/[0.03] hover:bg-white/[0.05] transition-all duration-300">
                  <td className="py-3 px-3 text-white/90 font-medium">{p.name}</td>
                  <td className="py-3 px-3"><span className="bg-[#10b981]/10 text-[#10b981] px-2 py-0.5 rounded text-[10px] font-mono border border-[#10b981]/20">{p.algo}</span></td>
                  {targetIps.map(ip => (
                    <td key={ip} className="py-3 px-3 text-right font-mono text-[#737373]">{p.vpsCounts[ip] || 0}</td>
                  ))}
                  <td className="py-3 px-3 text-right font-mono text-[#10b981]/80">{p.local > 0 ? p.local : '-'}</td>
                  <td className="py-3 px-3 text-right font-bold text-[#10b981]">{p.total}</td>
                  <td className={`py-3 px-3 text-right font-bold ${p.errors > 0 ? 'text-red-500' : 'text-[#737373]'}`}>{p.errors}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  );
};

export default function Dashboard() {
  const [servers, setServers] = useState<ServerStat[]>([]);
  const [containers, setContainers] = useState<ContainerStat[]>([]);
  const [loadBalancing, setLoadBalancing] = useState<LbStat[]>([]);
  
  const [selectedServerId, setSelectedServerId] = useState<string>('all');
  const [selectedPeriod, setSelectedPeriod] = useState('live');

  const [timeStr, setTimeStr] = useState("");
  const [dateStr, setDateStr] = useState("");

  useEffect(() => {
    const timer = setInterval(() => {
      const now = new Date();
      setTimeStr(now.toLocaleTimeString('pt-BR'));
      setDateStr(now.toLocaleDateString('pt-BR').replace(/\//g, '-'));
    }, 1000);
    return () => clearInterval(timer);
  }, []);

  useEffect(() => {
    const fetchMetrics = () => {
      fetch('http://localhost:8080/api/metrics/live')
        .then(res => res.json())
        .then(data => {
          if (data && data.servers) setServers(data.servers);
          if (data && data.containers) setContainers(data.containers);
          if (data && data.load_balancing) setLoadBalancing(data.load_balancing);
        })
        .catch(err => console.error("Erro API:", err));
    };
    fetchMetrics();
    const interval = setInterval(fetchMetrics, 2000);
    return () => clearInterval(interval);
  }, []);

  const filteredContainers = selectedServerId === 'all' 
    ? containers 
    : containers.filter(c => c.server_id === selectedServerId);

  const activeServer = selectedServerId !== 'all' 
    ? servers.find(s => s.id === selectedServerId) 
    : null;

  const totalCPU = filteredContainers.reduce((acc, curr) => acc + curr.cpu, 0);
  const totalMemUsed = filteredContainers.reduce((acc, curr) => acc + curr.mem_used, 0);
  const totalMemLimit = filteredContainers.length > 0 ? filteredContainers.reduce((acc, curr) => acc + curr.mem_limit, 0) : 1;
  const memPercent = (totalMemUsed / totalMemLimit) * 100;

  const isUp = servers.length > 0;

  const diskUsed = activeServer ? activeServer.disk_used : servers.reduce((a, b) => a + b.disk_used, 0);
  const diskTotal = activeServer ? activeServer.disk_total : servers.reduce((a, b) => a + b.disk_total, 0);
  const diskPercent = diskTotal > 0 ? (diskUsed / diskTotal) * 100 : 0;

  const history = Array.from({length: 20}).map((_, i) => ({ time: i, val1: diskPercent }));

  const uniqueServersMap = new Map();
  servers.forEach(s => {
    if (!uniqueServersMap.has(s.host_ip)) {
      uniqueServersMap.set(s.host_ip, s);
    }
  });
  const uniqueServers = Array.from(uniqueServersMap.values());

  const vpsOptions = [
    { value: 'all', label: 'Global (Cluster)' },
    ...uniqueServers.map(s => ({ value: s.id, label: s.host_ip === import.meta.env.VITE_LB_IP ? `${s.host_ip} (LB)` : s.host_ip }))
  ];

  const timeOptions = [
    { value: 'live', label: 'Ao Vivo (Tempo Real)' },
    { value: '1h', label: 'Última Hora' },
    { value: '24h', label: 'Últimas 24 Horas' }
  ];

  const isLoadBalancerSelected = activeServer?.host_ip === import.meta.env.VITE_LB_IP;

  return (
    <div className="min-h-full bg-[radial-gradient(ellipse_at_top,_var(--tw-gradient-stops))] from-[#1a1c23] via-[#050505] to-[#000000] text-white font-sans px-4 pb-4 pt-2 md:px-6 md:pb-6 md:pt-3 lg:px-8 lg:pb-8 lg:pt-4">
            <div className="glass-panel p-4 rounded-xl flex flex-wrap items-center gap-6 mb-6 relative z-50">
        <div className="flex items-center gap-3">
          <Filter className="text-[#737373] w-4 h-4" />
          <span className="text-[11px] font-bold tracking-widest text-[#737373] uppercase">Filtro IP:</span>
          <CustomSelect options={vpsOptions} value={selectedServerId} onChange={setSelectedServerId} />
        </div>

        <div className="h-6 w-px bg-white/10"></div>

        <div className="flex items-center gap-3">
          <Clock className="text-[#737373] w-4 h-4" />
          <CustomSelect options={timeOptions} value={selectedPeriod} onChange={setSelectedPeriod} />
        </div>

        {selectedPeriod !== 'live' && (
          <button 
            onClick={() => setSelectedPeriod('live')}
            className="ml-auto flex items-center gap-2 text-xs font-bold tracking-wider uppercase bg-red-900/30 hover:bg-red-900/50 text-red-400 border border-red-900/50 px-4 py-2 rounded-lg transition-all"
          >
            <Radio className="w-4 h-4 animate-pulse" />
            Voltar Ao Vivo
          </button>
        )}
      </div>

      {isLoadBalancerSelected ? (
        <LoadBalancerDashboard stats={loadBalancing} />
      ) : (
        <>
          <div className="grid grid-cols-1 md:grid-cols-3 gap-4 md:gap-6 mb-6">
            <div className="glass-panel rounded-xl p-6 flex flex-col items-center justify-center h-32 md:h-auto relative overflow-hidden">
              <div className="absolute inset-0 bg-gradient-to-b from-white/[0.02] to-transparent pointer-events-none" />
              <span className="text-[#737373] text-[11px] mb-2 font-bold tracking-widest uppercase">{dateStr || "Carregando..."}</span>
              <span className="text-5xl md:text-6xl font-light tracking-tight text-white/90 drop-shadow-md">{timeStr || "00:00:00"}</span>
            </div>
            
            <TopWidget title="Disponibilidade Global" value={isUp ? "UP" : "DOWN"} unit="" type={isUp ? 'success' : 'danger'} />
            
            <TopWidget title="Containers em Execução" value={filteredContainers.length} unit="Ativos" type="warning" />
          </div>

          {selectedServerId === 'all' && <LoadBalancerFlow stats={loadBalancing} />}

          <div className="grid grid-cols-1 md:grid-cols-3 gap-4 md:gap-6 mb-6 h-56">
            <Gauge value={totalCPU} title="CPU Total %" color="#10b981" />
            <Gauge value={totalCPU > 100 ? 100 : (totalCPU / 10)} title="Carga do sistema" color="#10b981" />
            <Gauge value={memPercent || 0} title="Memória %" color="#10b981" />
          </div>

          <div className="grid grid-cols-1 md:grid-cols-2 gap-4 md:gap-6">
        <div className="glass-panel rounded-xl p-6 flex flex-col relative h-80 overflow-hidden">
          <div className="flex items-center justify-between mb-4 border-b border-white/5 pb-3">
             <span className="uppercase tracking-widest text-[11px] font-bold text-[#737373]">
               Consumo (Ao Vivo)
             </span>
             <div className="flex gap-2 items-center bg-[#10b981]/10 px-2 py-1 rounded border border-[#10b981]/20">
               <span className="w-1.5 h-1.5 rounded-full bg-[#10b981] animate-pulse"></span>
               <span className="text-[10px] text-[#10b981] font-bold tracking-widest uppercase">Live</span>
             </div>
          </div>
          <div className="flex-1 overflow-y-auto pr-2 custom-scrollbar">
            <table className="w-full text-sm text-left border-collapse">
              <thead className="text-[10px] text-[#737373] uppercase tracking-widest sticky top-0 bg-[#0c0c0e] shadow-[0_4px_10px_rgba(0,0,0,0.5)] z-10">
                <tr>
                  <th className="py-3 px-2 rounded-l">Container</th>
                  <th className="py-3 px-2 text-right">CPU</th>
                  <th className="py-3 px-2 text-right rounded-r">Memória (MB)</th>
                </tr>
              </thead>
              <tbody>
                {filteredContainers.length === 0 && <tr><td colSpan={3} className="text-center py-8 text-[#737373] text-xs">Buscando dados no motor Go...</td></tr>}
                {filteredContainers.map(s => (
                  <tr key={s.docker_id} className="border-b border-white/[0.03] hover:bg-white/[0.05] transition-all duration-300">
                    <td className="py-3 px-2 text-[#f59e0b] font-mono text-[12px]">{s.name}</td>
                    <td className="py-3 px-2 text-right font-medium text-white/90">{s.cpu.toFixed(2)}%</td>
                    <td className="py-3 px-2 text-right font-medium text-white/90">{(s.mem_used / 1024 / 1024).toFixed(1)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>

        <div className="glass-panel rounded-xl p-6 flex flex-col relative h-80 overflow-hidden">
          <div className="flex items-center justify-between mb-4 border-b border-white/5 pb-3">
             <span className="uppercase tracking-widest text-[11px] font-bold text-[#737373]">
               Espaço de Disco Ocupado
             </span>
          </div>
          <div className="flex-1 mt-2">
            <ResponsiveContainer width="100%" height="100%">
              <AreaChart data={history} margin={{ top: 10, right: 0, left: -20, bottom: 0 }}>
                <defs>
                  <linearGradient id="colorDisk" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="5%" stopColor="#10b981" stopOpacity={0.5}/>
                    <stop offset="95%" stopColor="#10b981" stopOpacity={0.0}/>
                  </linearGradient>
                </defs>
                <CartesianGrid strokeDasharray="3 3" stroke="rgba(255,255,255,0.04)" vertical={false} />
                <XAxis dataKey="time" stroke="#404040" fontSize={10} tickLine={false} axisLine={false} />
                <YAxis stroke="#404040" fontSize={10} tickLine={false} axisLine={false} tickFormatter={(v) => `${v}%`} />
                <Tooltip 
                  contentStyle={{ backgroundColor: 'rgba(12,12,14,0.9)', borderColor: 'rgba(255,255,255,0.1)', borderRadius: '8px', backdropFilter: 'blur(10px)' }} 
                  itemStyle={{ color: '#10b981', fontWeight: 'bold' }}
                />
                <Area type="monotone" dataKey="val1" stroke="#10b981" strokeWidth={2} fill="url(#colorDisk)" isAnimationActive={false} />
              </AreaChart>
            </ResponsiveContainer>
          </div>
          <div className="flex flex-col gap-1 mt-3 text-[11px] text-[#737373] tracking-wider uppercase">
            <div className="flex items-center gap-2">
              <div className="w-2 h-2 rounded-full bg-[#10b981] shadow-[0_0_5px_rgba(16,185,129,0.4)]"></div> 
              Partição / (root): Atual {diskPercent.toFixed(1)}% 
              <span className="ml-auto font-mono text-[#10b981]">{(diskUsed/1024/1024/1024).toFixed(1)} GB / {(diskTotal/1024/1024/1024).toFixed(1)} GB</span>
            </div>
          </div>
        </div>
      </div>
      </>
      )}
    </div>
  );
}



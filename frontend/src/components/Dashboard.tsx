import { useEffect, useMemo, useState } from 'react';
import { Clock, Filter, Globe, Server, Database, Activity, ArrowRight, HardDrive } from 'lucide-react';
import { AreaChart, Area, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer, PieChart, Pie, Cell } from 'recharts';
import { api, type ContainerLiveStat, type HistoryRange, type LbStat, type ServerLiveStat } from '../lib/api';
import { formatBytes, formatGB } from '../lib/format';
import { deriveUpstreams, splitUpstreams, totalRequests, type UpstreamNode } from '../lib/upstream';
import Select, { type SelectOption } from './ui/Select';

const LIVE_POLL_MS = 2000;
const HISTORY_POLL_MS = 30000;

const ERROR_STATUSES = ['500', '502', '503', '504', '400', '404'];

// IPs das VPS de destino do Load Balancer. Vem do .env porque muda por ambiente.
const TARGET_IPS: string[] = (import.meta.env.VITE_TARGET_VPS_IPS || '')
  .split(',')
  .map((ip: string) => ip.trim())
  .filter(Boolean);

const LB_IP: string = import.meta.env.VITE_LB_IP || '';

// Distribui os nós de upstream verticalmente no diagrama de fluxo (em %).
const nodeY = (index: number, total: number) => (total === 1 ? 50 : 15 + index * (70 / (total - 1)));

const Gauge = ({ value, title, color }: { value: number; title: string; color: string }) => {
  const clamped = Math.max(0, Math.min(value, 100));
  const data = [
    { name: 'value', value: clamped },
    { name: 'empty', value: 100 - clamped },
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
            {value.toFixed(1)}%
          </span>
        </div>
      </div>
    </div>
  );
};

const TopWidget = ({ title, value, unit, type = 'success' }: { title: string; value: string | number; unit: string; type?: 'success' | 'danger' | 'warning' }) => {
  const colors = {
    success: { text: 'text-[#10b981]', glow: 'text-glow-green', bg: 'bg-[#10b981]', shadow: 'shadow-[0_0_8px_rgba(16,185,129,0.3)]' },
    danger: { text: 'text-[#ef4444]', glow: 'text-glow-red', bg: 'bg-[#ef4444]', shadow: 'shadow-[0_0_8px_rgba(239,68,68,0.3)]' },
    warning: { text: 'text-[#f59e0b]', glow: 'text-glow-yellow', bg: 'bg-[#f59e0b]', shadow: 'shadow-[0_0_8px_rgba(245,158,11,0.3)]' },
  };
  const theme = colors[type];

  return (
    <div className="glass-panel rounded-xl p-6 flex flex-col items-center justify-center relative overflow-hidden transition-all duration-300">
      <div className={`absolute top-0 left-0 w-full h-[2px] ${theme.bg} opacity-40 ${theme.shadow}`} />
      <span className="text-[#737373] mb-2 text-center tracking-widest font-bold uppercase text-[11px]">{title}</span>
      <div className="flex items-baseline gap-1">
        <span className={`text-5xl font-bold tracking-tight ${theme.text} ${theme.glow}`}>{value}</span>
        {unit && <span className={`text-xl font-bold ${theme.text} opacity-80`}>{unit}</span>}
      </div>
    </div>
  );
};

const LoadBalancerFlow = ({ stats }: { stats: LbStat[] }) => {
  // Os nós vêm do que o Nginx reporta, não da lista do .env: se os endereços
  // divergirem (troca de rede, VPN), o diagrama continua mostrando a verdade.
  const nodes = useMemo(() => deriveUpstreams(stats, TARGET_IPS), [stats]);
  const total = totalRequests(stats);

  return (
    <div className="glass-panel rounded-xl p-6 mb-6 overflow-hidden relative">
      <div className="absolute inset-0 bg-[radial-gradient(ellipse_at_center,_var(--tw-gradient-stops))] from-[#10b981]/[0.03] to-transparent pointer-events-none" />

      <div className="flex items-center justify-between mb-8 pb-3 border-b border-white/[0.03] relative z-10">
        <span className="uppercase tracking-widest text-[11px] font-bold text-[#737373] flex items-center gap-2">
          <Activity className="w-4 h-4 text-[#10b981]" />
          Malha de Roteamento (NGINX Cluster)
        </span>
        <div className="flex gap-2 items-center bg-[#10b981]/5 px-3 py-1.5 rounded border border-[#10b981]/20 backdrop-blur-sm">
          <span className="w-1.5 h-1.5 rounded-full bg-[#10b981] animate-pulse shadow-[0_0_8px_#10b981]" />
          <span className="text-[10px] text-[#10b981] font-bold tracking-widest uppercase">{total} Req / 5s</span>
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
          <svg className="w-full h-12 overflow-visible" viewBox="0 0 100 100" preserveAspectRatio="none" aria-hidden="true">
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
          <svg className="absolute inset-0 w-full h-full overflow-visible" viewBox="0 0 100 100" preserveAspectRatio="none" aria-hidden="true">
            {nodes.map((node, idx) => {
              const y = nodeY(idx, nodes.length);
              return (
                <g key={`path-${node.addr}`}>
                  <path
                    id={`path-v-${idx}`}
                    d={`M 0,50 C 40,50 40,${y} 100,${y}`}
                    fill="none"
                    stroke={node.reqs > 0 ? 'rgba(16,185,129,0.25)' : 'rgba(255,255,255,0.05)'}
                    strokeWidth="1"
                  />
                  {node.reqs > 0 &&
                    Array.from({ length: Math.min(node.reqs, 5) }).map((_, i) => (
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

          {nodes.map((node, idx) => (
            <div
              key={`badge-${node.addr}`}
              className={`absolute right-[15%] bg-[#0c0c0e] px-2 py-0.5 rounded-full border text-[10px] font-mono shadow-md translate-y-[-50%] ${
                node.reqs > 0 ? 'border-[#10b981]/20 text-[#10b981]' : 'border-white/5 text-[#737373]'
              }`}
              style={{ top: `${nodeY(idx, nodes.length)}%` }}
            >
              {node.reqs} req
            </div>
          ))}
        </div>

        <div className="z-10 flex flex-col justify-around h-full py-[12px] min-w-[160px] md:min-w-[200px] gap-3">
          {nodes.map((node, idx) => (
            <div
              key={node.addr}
              className={`w-full bg-gradient-to-r from-[#0c0c0e] to-[#1a1c23] border transition-colors rounded-xl p-3 flex items-center gap-4 shadow-lg group relative h-[64px] ${
                node.reqs > 0 ? 'border-[#10b981]/30' : 'border-white/5 hover:border-white/20'
              }`}
              title={node.reqs > 0 ? `${node.reqs} req / 5s` : 'Sem tráfego na janela'}
            >
              <div className="w-10 h-10 rounded-lg bg-white/5 flex items-center justify-center flex-shrink-0">
                <Database
                  className={`w-5 h-5 transition-colors ${
                    node.reqs > 0 ? 'text-[#10b981]' : 'text-white/40 group-hover:text-[#10b981]'
                  }`}
                />
              </div>
              <div className="flex flex-col">
                <span className="text-[10px] text-[#737373] font-bold tracking-widest uppercase">Node {idx + 1}</span>
                <span className="text-[11px] text-white/80 font-mono mt-0.5 selectable">{node.addr}</span>
              </div>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
};

interface ProjectTraffic {
  name: string;
  total: number;
  errors: number;
  vpsCounts: Record<string, number>;
  local: number;
  algo: string;
}

const groupTrafficByProject = (stats: LbStat[], nodes: UpstreamNode[]): ProjectTraffic[] => {
  const projects = new Map<string, ProjectTraffic>();
  const known = new Set(nodes.map((n) => n.addr));

  stats.forEach((s) => {
    const name = s.server_name || 'Desconhecido';
    if (!projects.has(name)) {
      const isWebSocket = name.includes('ws.') || name.includes('soketi') || s.upstream_addr.includes('6001');
      projects.set(name, {
        name,
        total: 0,
        errors: 0,
        vpsCounts: {},
        local: 0,
        algo: isWebSocket ? 'IP_HASH (Sticky)' : 'LEAST_CONN',
      });
    }
    const data = projects.get(name)!;
    data.total += s.requests_count;
    if (ERROR_STATUSES.includes(s.status)) data.errors += s.requests_count;

    // Uma linha pode citar mais de um upstream quando o Nginx refaz a
    // requisição; conta em todos os que aparecem, e no cache local só quando
    // não houve upstream nenhum.
    const addrs = splitUpstreams(s.upstream_addr).filter((a) => known.has(a));
    if (addrs.length === 0) {
      data.local += s.requests_count;
      return;
    }
    for (const addr of addrs) {
      data.vpsCounts[addr] = (data.vpsCounts[addr] || 0) + s.requests_count;
    }
  });

  return Array.from(projects.values()).sort((a, b) => b.total - a.total);
};

const LoadBalancerDashboard = ({ stats }: { stats: LbStat[] }) => {
  const nodes = useMemo(() => deriveUpstreams(stats, TARGET_IPS), [stats]);
  const projects = useMemo(() => groupTrafficByProject(stats, nodes), [stats, nodes]);
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
          {totalErrors > 0 && <span className="text-xs text-red-400 mt-2 block">Verifique a tela de Logs para o detalhe das falhas.</span>}
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
                {nodes.map((node, idx) => (
                  <th key={node.addr} className="py-3 px-3 text-right" title={node.addr}>Node {idx + 1}</th>
                ))}
                <th className="py-3 px-3 text-right text-[#10b981]/80">Local/Cache</th>
                <th className="py-3 px-3 text-right">Total Req/5s</th>
                <th className="py-3 px-3 text-right rounded-r">Erros</th>
              </tr>
            </thead>
            <tbody>
              {projects.length === 0 && (
                <tr>
                  <td colSpan={5 + nodes.length} className="text-center py-8 text-[#737373] text-xs">Sem tráfego nos últimos 5 segundos...</td>
                </tr>
              )}
              {projects.map((p) => (
                <tr key={p.name} className="border-b border-white/[0.03] hover:bg-white/[0.05] transition-all duration-300">
                  <td className="py-3 px-3 text-white/90 font-medium">{p.name}</td>
                  <td className="py-3 px-3">
                    <span className="bg-[#10b981]/10 text-[#10b981] px-2 py-0.5 rounded text-[10px] font-mono border border-[#10b981]/20">{p.algo}</span>
                  </td>
                  {nodes.map((node) => (
                    <td key={node.addr} className="py-3 px-3 text-right font-mono text-[#737373]">{p.vpsCounts[node.addr] || 0}</td>
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

const RANGE_OPTIONS: { value: HistoryRange; label: string }[] = [
  { value: '1h', label: 'Última hora' },
  { value: '6h', label: 'Últimas 6 horas' },
  { value: '24h', label: 'Últimas 24 horas' },
  { value: '7d', label: 'Últimos 7 dias' },
];

interface DiskPoint {
  time: string;
  value: number;
}

export default function Dashboard() {
  const [servers, setServers] = useState<ServerLiveStat[]>([]);
  const [containers, setContainers] = useState<ContainerLiveStat[]>([]);
  const [loadBalancing, setLoadBalancing] = useState<LbStat[]>([]);

  const [selectedServerId, setSelectedServerId] = useState('all');
  const [historyRange, setHistoryRange] = useState<HistoryRange>('1h');
  const [diskHistory, setDiskHistory] = useState<DiskPoint[]>([]);

  const [now, setNow] = useState<Date | null>(null);

  useEffect(() => {
    setNow(new Date());
    const timer = setInterval(() => setNow(new Date()), 1000);
    return () => clearInterval(timer);
  }, []);

  useEffect(() => {
    const controller = new AbortController();
    const fetchMetrics = () => {
      api.liveMetrics(controller.signal)
        .then((data) => {
          setServers(data.servers);
          setContainers(data.containers);
          setLoadBalancing(data.load_balancing);
        })
        .catch((err) => {
          if (!controller.signal.aborted) console.error('Erro API:', err);
        });
    };
    fetchMetrics();
    const interval = setInterval(fetchMetrics, LIVE_POLL_MS);
    return () => {
      clearInterval(interval);
      controller.abort();
    };
  }, []);

  // Série real de disco. Só existe para um host específico: o endpoint de
  // histórico agrega por server_id, não há série consolidada do cluster.
  useEffect(() => {
    if (selectedServerId === 'all') {
      setDiskHistory([]);
      return;
    }
    const controller = new AbortController();
    const fetchHistory = () => {
      api.history(selectedServerId, 'disk', historyRange, controller.signal)
        .then((points) =>
          setDiskHistory(points.map((p) => ({
            time: new Date(p.ts).toLocaleTimeString('pt-BR', { hour: '2-digit', minute: '2-digit' }),
            value: Number(p.value.toFixed(1)),
          }))),
        )
        .catch(() => {});
    };
    fetchHistory();
    const interval = setInterval(fetchHistory, HISTORY_POLL_MS);
    return () => {
      clearInterval(interval);
      controller.abort();
    };
  }, [selectedServerId, historyRange]);

  const activeServer = selectedServerId === 'all' ? null : servers.find((s) => s.id === selectedServerId) ?? null;
  const scopedServers = activeServer ? [activeServer] : servers;
  const onlineServers = scopedServers.filter((s) => s.online);

  const filteredContainers = selectedServerId === 'all' ? containers : containers.filter((c) => c.server_id === selectedServerId);

  // Métricas de host: média de CPU e razão real de RAM/disco dos hosts em escopo.
  const cpuPercent = onlineServers.length > 0 ? onlineServers.reduce((acc, s) => acc + s.cpu, 0) / onlineServers.length : 0;

  const memUsed = onlineServers.reduce((acc, s) => acc + s.mem_used, 0);
  const memTotal = onlineServers.reduce((acc, s) => acc + s.mem_total, 0);
  const memPercent = memTotal > 0 ? (memUsed / memTotal) * 100 : 0;

  const diskUsed = onlineServers.reduce((acc, s) => acc + s.disk_used, 0);
  const diskTotal = onlineServers.reduce((acc, s) => acc + s.disk_total, 0);
  const diskPercent = diskTotal > 0 ? (diskUsed / diskTotal) * 100 : 0;

  const isUp = onlineServers.length > 0;

  // Um host pode aparecer por mais de um registro; o filtro é por IP.
  const uniqueServers = useMemo(() => {
    const byIp = new Map<string, ServerLiveStat>();
    servers.forEach((s) => {
      if (!byIp.has(s.host_ip)) byIp.set(s.host_ip, s);
    });
    return Array.from(byIp.values());
  }, [servers]);

  const vpsOptions: SelectOption[] = [
    { value: 'all', label: 'Global (Cluster)' },
    ...uniqueServers.map((s) => ({ value: s.id, label: s.host_ip === LB_IP ? `${s.host_ip} (LB)` : s.host_ip })),
  ];

  const isLoadBalancerSelected = activeServer?.host_ip === LB_IP;

  return (
    <div className="min-h-full bg-[radial-gradient(ellipse_at_top,_var(--tw-gradient-stops))] from-[#1a1c23] via-[#050505] to-[#000000] text-white font-sans px-4 pb-4 pt-2 md:px-6 md:pb-6 md:pt-3 lg:px-8 lg:pb-8 lg:pt-4">
      <div className="glass-panel p-4 rounded-xl flex flex-wrap items-center gap-6 mb-6 relative z-50">
        <div className="flex items-center gap-3">
          <Filter className="text-[#737373] w-4 h-4" />
          <span className="text-[11px] font-bold tracking-widest text-[#737373] uppercase">Filtro IP:</span>
          <Select ariaLabel="Filtrar por IP" options={vpsOptions} value={selectedServerId} onChange={setSelectedServerId} />
        </div>

        <div className="h-6 w-px bg-white/10" />

        <div className="flex items-center gap-3">
          <Clock className="text-[#737373] w-4 h-4" />
          <span className="text-[11px] font-bold tracking-widest text-[#737373] uppercase">Histórico:</span>
          <Select
            ariaLabel="Janela do histórico de disco"
            options={RANGE_OPTIONS}
            value={historyRange}
            onChange={(v) => setHistoryRange(v as HistoryRange)}
          />
        </div>
      </div>

      {isLoadBalancerSelected ? (
        <LoadBalancerDashboard stats={loadBalancing} />
      ) : (
        <>
          <div className="grid grid-cols-1 md:grid-cols-3 gap-4 md:gap-6 mb-6">
            <div className="glass-panel rounded-xl p-6 flex flex-col items-center justify-center h-32 md:h-auto relative overflow-hidden">
              <div className="absolute inset-0 bg-gradient-to-b from-white/[0.02] to-transparent pointer-events-none" />
              <span className="text-[#737373] text-[11px] mb-2 font-bold tracking-widest uppercase">
                {now ? now.toLocaleDateString('pt-BR').replace(/\//g, '-') : 'Carregando...'}
              </span>
              <span className="text-5xl md:text-6xl font-light tracking-tight text-white/90 drop-shadow-md">
                {now ? now.toLocaleTimeString('pt-BR') : '00:00:00'}
              </span>
            </div>

            <TopWidget title="Disponibilidade Global" value={isUp ? 'UP' : 'DOWN'} unit="" type={isUp ? 'success' : 'danger'} />

            <TopWidget title="Containers em Execução" value={filteredContainers.length} unit="Ativos" type="warning" />
          </div>

          {selectedServerId === 'all' && <LoadBalancerFlow stats={loadBalancing} />}

          <div className="grid grid-cols-1 md:grid-cols-3 gap-4 md:gap-6 mb-6 h-56">
            <Gauge value={cpuPercent} title="CPU do Host %" color="#10b981" />
            <Gauge value={memPercent} title="Memória %" color="#10b981" />
            <Gauge value={diskPercent} title="Disco %" color="#10b981" />
          </div>

          <div className="grid grid-cols-1 md:grid-cols-2 gap-4 md:gap-6">
            <div className="glass-panel rounded-xl p-6 flex flex-col relative h-80 overflow-hidden">
              <div className="flex items-center justify-between mb-4 border-b border-white/5 pb-3">
                <span className="uppercase tracking-widest text-[11px] font-bold text-[#737373]">Consumo (Ao Vivo)</span>
                <div className="flex gap-2 items-center bg-[#10b981]/10 px-2 py-1 rounded border border-[#10b981]/20">
                  <span className="w-1.5 h-1.5 rounded-full bg-[#10b981] animate-pulse" />
                  <span className="text-[10px] text-[#10b981] font-bold tracking-widest uppercase">Live</span>
                </div>
              </div>
              <div className="flex-1 overflow-y-auto pr-2 custom-scrollbar">
                <table className="w-full text-sm text-left border-collapse">
                  <thead className="text-[10px] text-[#737373] uppercase tracking-widest sticky top-0 bg-[#0c0c0e] shadow-[0_4px_10px_rgba(0,0,0,0.5)] z-10">
                    <tr>
                      <th className="py-3 px-2 rounded-l">Container</th>
                      <th className="py-3 px-2 text-right">CPU</th>
                      <th className="py-3 px-2 text-right rounded-r">Memória</th>
                    </tr>
                  </thead>
                  <tbody>
                    {filteredContainers.length === 0 && (
                      <tr>
                        <td colSpan={3} className="text-center py-8 text-[#737373] text-xs">Buscando dados no motor Go...</td>
                      </tr>
                    )}
                    {filteredContainers.map((c) => (
                      <tr key={c.docker_id} className="border-b border-white/[0.03] hover:bg-white/[0.05] transition-all duration-300">
                        <td className="py-3 px-2 text-[#f59e0b] font-mono text-[12px]">{c.name}</td>
                        <td className="py-3 px-2 text-right font-medium text-white/90">{c.cpu.toFixed(2)}%</td>
                        <td className="py-3 px-2 text-right font-medium text-white/90">{formatBytes(c.mem_used)}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>

            <div className="glass-panel rounded-xl p-6 flex flex-col relative h-80 overflow-hidden">
              <div className="flex items-center justify-between mb-4 border-b border-white/5 pb-3">
                <span className="uppercase tracking-widest text-[11px] font-bold text-[#737373]">Espaço de Disco Ocupado</span>
                <span className="text-[10px] text-[#737373] tracking-widest uppercase">
                  {activeServer ? `${activeServer.host_ip} · ${historyRange}` : 'Cluster'}
                </span>
              </div>

              {activeServer ? (
                <div className="flex-1 mt-2">
                  {diskHistory.length === 0 ? (
                    <div className="h-full flex items-center justify-center text-xs text-[#737373]">Sem histórico de disco na janela selecionada.</div>
                  ) : (
                    <ResponsiveContainer width="100%" height="100%">
                      <AreaChart data={diskHistory} margin={{ top: 10, right: 0, left: -20, bottom: 0 }}>
                        <defs>
                          <linearGradient id="colorDisk" x1="0" y1="0" x2="0" y2="1">
                            <stop offset="5%" stopColor="#10b981" stopOpacity={0.5} />
                            <stop offset="95%" stopColor="#10b981" stopOpacity={0.0} />
                          </linearGradient>
                        </defs>
                        <CartesianGrid strokeDasharray="3 3" stroke="rgba(255,255,255,0.04)" vertical={false} />
                        <XAxis dataKey="time" stroke="#404040" fontSize={10} tickLine={false} axisLine={false} minTickGap={30} />
                        <YAxis stroke="#404040" fontSize={10} tickLine={false} axisLine={false} domain={[0, 100]} tickFormatter={(v) => `${v}%`} />
                        <Tooltip
                          contentStyle={{ backgroundColor: 'rgba(12,12,14,0.9)', borderColor: 'rgba(255,255,255,0.1)', borderRadius: '8px', backdropFilter: 'blur(10px)' }}
                          itemStyle={{ color: '#10b981', fontWeight: 'bold' }}
                          formatter={(value) => [`${value}%`, 'Disco']}
                        />
                        <Area type="monotone" dataKey="value" stroke="#10b981" strokeWidth={2} fill="url(#colorDisk)" isAnimationActive={false} />
                      </AreaChart>
                    </ResponsiveContainer>
                  )}
                </div>
              ) : (
                <div className="flex-1 mt-2 overflow-y-auto custom-scrollbar flex flex-col gap-3">
                  {onlineServers.length === 0 && <div className="text-xs text-[#737373]">Nenhum host online reportando disco.</div>}
                  {onlineServers.map((s) => {
                    const pct = s.disk_total > 0 ? (s.disk_used / s.disk_total) * 100 : 0;
                    return (
                      <div key={s.id} className="flex flex-col gap-1">
                        <div className="flex items-center justify-between text-[11px]">
                          <span className="flex items-center gap-2 text-white/80">
                            <HardDrive className="w-3 h-3 text-[#10b981]" />
                            <span className="font-mono selectable">{s.host_ip}</span>
                          </span>
                          <span className="font-mono text-[#10b981]">
                            {formatGB(s.disk_used)} / {formatGB(s.disk_total)} GB
                          </span>
                        </div>
                        <div className="w-full h-1.5 bg-white/10 rounded-full overflow-hidden">
                          <div className={`h-full ${pct > 85 ? 'bg-red-500' : 'bg-[#10b981]'}`} style={{ width: `${Math.min(pct, 100)}%` }} />
                        </div>
                      </div>
                    );
                  })}
                </div>
              )}
            </div>
          </div>
        </>
      )}
    </div>
  );
}

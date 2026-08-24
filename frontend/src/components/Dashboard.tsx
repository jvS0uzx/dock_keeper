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

// Mesmos limiares das telas de estação e detalhe, para o dashboard não pintar
// de verde o que a lista pinta de âmbar.
const USAGE_WARN = 75;
const USAGE_CRITICAL = 90;

// IPs das VPS de destino do Load Balancer. Vem do .env porque muda por ambiente.
const TARGET_IPS: string[] = (import.meta.env.VITE_TARGET_VPS_IPS || '')
  .split(',')
  .map((ip: string) => ip.trim())
  .filter(Boolean);

const LB_IP: string = import.meta.env.VITE_LB_IP || '';

// Distribui os nós de upstream verticalmente no diagrama de fluxo (em %).
const nodeY = (index: number, total: number) => (total === 1 ? 50 : 15 + index * (70 / (total - 1)));

const gaugeVar = (value: number) => {
  if (value >= USAGE_CRITICAL) return 'var(--color-crit)';
  if (value >= USAGE_WARN) return 'var(--color-warn)';
  return 'var(--color-ok)';
};

const Gauge = ({ value, title }: { value: number; title: string }) => {
  const clamped = Math.max(0, Math.min(value, 100));
  const color = gaugeVar(clamped);
  const data = [
    { name: 'value', value: clamped },
    { name: 'empty', value: 100 - clamped },
  ];
  return (
    <div className="stat-card flex flex-col items-center justify-center relative h-full">
      <span className="eyebrow absolute top-4">{title}</span>
      <div className="w-full h-32 mt-6 relative">
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
              <Cell fill="var(--color-ink-750)" />
            </Pie>
          </PieChart>
        </ResponsiveContainer>
        <div className="absolute bottom-0 w-full flex justify-center">
          <span className="stat-value text-3xl">{value.toFixed(1)}%</span>
        </div>
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
    <div className="panel p-6 mb-6 overflow-hidden relative">
      <div className="flex items-center justify-between mb-8 pb-3 border-b border-line">
        <span className="eyebrow flex items-center gap-2">
          <Activity size={16} strokeWidth={1.75} className="text-accent" />
          Malha de roteamento (cluster NGINX)
        </span>
        <span className={`badge ${total > 0 ? 'badge-ok' : 'badge-muted'}`}>
          <span className={`w-1.5 h-1.5 rounded-full ${total > 0 ? 'bg-ok animate-pulse' : 'bg-text-faint'}`} />
          {total} req / 5s
        </span>
      </div>

      <div className="relative w-full max-w-4xl mx-auto flex items-center justify-between min-h-[192px] md:px-4">
        <div className="z-10 flex flex-col items-center justify-center min-w-[80px]">
          <div className="w-14 h-14 rounded-full bg-ink-800 border border-line flex items-center justify-center transition-colors hover:border-line-hi">
            <Globe size={22} strokeWidth={1.75} className="text-text-mut" />
          </div>
          <span className="eyebrow mt-3">Cloudflare</span>
        </div>

        <div className="flex-1 relative flex items-center -mx-4 z-0">
          <svg className="w-full h-12 overflow-visible" viewBox="0 0 100 100" preserveAspectRatio="none" aria-hidden="true">
            <path id="path-in" d="M 0,50 L 100,50" fill="none" stroke="var(--color-line)" strokeWidth="1" />
            {total > 0 && (
              <circle r="2" fill="var(--color-accent)">
                <animateMotion dur="1s" repeatCount="indefinite">
                  <mpath href="#path-in" />
                </animateMotion>
              </circle>
            )}
          </svg>
          <div className="absolute left-1/2 -translate-x-1/2 -top-4 bg-ink-950 px-2 py-0.5 rounded-full border border-line text-[10px] text-text-mut mono-data flex items-center gap-1">
            {total} <ArrowRight size={12} strokeWidth={1.75} className="text-accent" />
          </div>
        </div>

        <div className="z-10 flex flex-col items-center justify-center min-w-[80px]">
          <div
            className={`w-16 h-16 rounded-card bg-ink-800 border flex items-center justify-center transition-colors ${
              total > 0 ? 'border-accent/40' : 'border-line'
            }`}
          >
            <Server size={26} strokeWidth={1.75} className={total > 0 ? 'text-accent' : 'text-text-mut'} />
          </div>
          <span className="eyebrow mt-3">Load Balancer</span>
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
                    stroke={node.reqs > 0 ? 'var(--color-accent)' : 'var(--color-line)'}
                    strokeOpacity={node.reqs > 0 ? 0.35 : 1}
                    strokeWidth="1"
                  />
                  {node.reqs > 0 &&
                    Array.from({ length: Math.min(node.reqs, 5) }).map((_, i) => (
                      <circle key={`v-${idx}-${i}`} r="2" fill="var(--color-accent)">
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
              className={`absolute right-[15%] bg-ink-950 px-2 py-0.5 rounded-full border text-[10px] mono-data translate-y-[-50%] ${
                node.reqs > 0 ? 'border-accent/30 text-accent' : 'border-line text-text-faint'
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
              className={`panel panel-hover w-full p-3 flex items-center gap-4 h-[64px] ${
                node.reqs > 0 ? 'border-accent/30' : ''
              }`}
              title={node.reqs > 0 ? `${node.reqs} req / 5s` : 'Sem tráfego na janela'}
            >
              <div className="w-10 h-10 rounded-ctrl bg-ink-800 flex items-center justify-center flex-shrink-0">
                <Database
                  size={18}
                  strokeWidth={1.75}
                  className={node.reqs > 0 ? 'text-accent' : 'text-text-faint'}
                />
              </div>
              <div className="flex flex-col">
                <span className="eyebrow">Node {idx + 1}</span>
                <span className="text-[11px] text-text mono-data mt-0.5 selectable">{node.addr}</span>
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
    <div className="flex flex-col gap-6 anim-rise">
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4 md:gap-6">
        <div className="stat-card">
          <span className="eyebrow">Erros HTTP na janela (5s)</span>
          <div className="flex items-baseline gap-3 mt-2">
            <span className={`stat-value text-4xl ${totalErrors > 0 ? 'text-crit' : 'text-ok'}`}>{totalErrors}</span>
            <span className="text-xs text-text-mut">respostas 5xx/4xx</span>
          </div>
          {totalErrors > 0 && (
            <span className="text-xs text-crit mt-2 block">Verifique a tela de Logs para o detalhe das falhas.</span>
          )}
        </div>

        <div className="stat-card flex flex-col justify-center">
          <span className="eyebrow mb-3">Algoritmos ativos (NGINX)</span>
          <div className="flex flex-col gap-2">
            <div className="flex justify-between items-center bg-ink-950 px-3 py-1.5 rounded-ctrl border border-line">
              <span className="text-xs text-text">Tráfego HTTP/API</span>
              <span className="text-xs mono-data text-text-mut">LEAST_CONN</span>
            </div>
            <div className="flex justify-between items-center bg-ink-950 px-3 py-1.5 rounded-ctrl border border-line">
              <span className="text-xs text-text">WebSockets (Kanban)</span>
              <span className="text-xs mono-data text-text-mut">IP_HASH</span>
            </div>
          </div>
        </div>
      </div>

      <div className="panel p-6 overflow-hidden">
        <div className="flex items-center justify-between mb-4 border-b border-line pb-3">
          <span className="eyebrow flex items-center gap-2">
            <Server size={16} strokeWidth={1.75} className="text-accent" />
            Comportamento por sistema (projetos)
          </span>
        </div>

        <div className="overflow-x-auto">
          <table className="table-base">
            <thead>
              <tr>
                <th>Domínio / Sistema</th>
                <th>Algoritmo</th>
                {nodes.map((node, idx) => (
                  <th key={node.addr} className="text-right" title={node.addr}>Node {idx + 1}</th>
                ))}
                <th className="text-right">Local/Cache</th>
                <th className="text-right">Total req/5s</th>
                <th className="text-right">Erros</th>
              </tr>
            </thead>
            <tbody>
              {projects.length === 0 && (
                <tr>
                  <td colSpan={5 + nodes.length} className="text-center py-8 text-text-faint text-xs">
                    Sem tráfego nos últimos 5 segundos.
                  </td>
                </tr>
              )}
              {projects.map((p) => (
                <tr key={p.name}>
                  <td className="text-text-hi font-medium">{p.name}</td>
                  <td>
                    <span className="badge badge-muted mono-data">{p.algo}</span>
                  </td>
                  {nodes.map((node) => (
                    <td key={node.addr} className="text-right mono-data text-text-mut">{p.vpsCounts[node.addr] || 0}</td>
                  ))}
                  <td className="text-right mono-data text-text-mut">{p.local > 0 ? p.local : '-'}</td>
                  <td className="text-right mono-data text-text-hi font-semibold">{p.total}</td>
                  <td className={`text-right mono-data font-semibold ${p.errors > 0 ? 'text-crit' : 'text-text-faint'}`}>{p.errors}</td>
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
  const offlineCount = scopedServers.length - onlineServers.length;

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
    <div className="min-h-full px-4 pb-4 pt-2 md:px-6 md:pb-6 md:pt-3 lg:px-8 lg:pb-8 lg:pt-4 anim-rise">
      <div className="panel p-4 flex flex-wrap items-center gap-6 mb-6 relative z-50">
        <div className="flex items-center gap-3">
          <Filter size={16} strokeWidth={1.75} className="text-text-faint" />
          <span className="eyebrow">Filtro IP</span>
          <Select ariaLabel="Filtrar por IP" options={vpsOptions} value={selectedServerId} onChange={setSelectedServerId} />
        </div>

        <div className="h-6 w-px bg-line" />

        <div className="flex items-center gap-3">
          <Clock size={16} strokeWidth={1.75} className="text-text-faint" />
          <span className="eyebrow">Histórico</span>
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
          <div className="grid grid-cols-1 lg:grid-cols-3 gap-4 md:gap-6 mb-6 stagger">
            <div className="stat-card lg:col-span-2 flex flex-col justify-between">
              <div className="flex items-center justify-between">
                <span className="eyebrow">Estado do cluster</span>
                <span className={`badge ${isUp ? 'badge-ok' : 'badge-crit'}`}>
                  <span className={`w-1.5 h-1.5 rounded-full ${isUp ? 'bg-ok animate-pulse' : 'bg-crit'}`} />
                  {isUp ? 'Operacional' : 'Indisponível'}
                </span>
              </div>
              <div className="grid grid-cols-3 gap-4 mt-4">
                <div>
                  <div className={`stat-value ${isUp ? '' : 'text-crit'}`}>{onlineServers.length}</div>
                  <div className="text-xs text-text-mut mt-1">hosts online</div>
                </div>
                <div>
                  <div className={`stat-value ${offlineCount > 0 ? 'text-crit' : 'text-text-faint'}`}>{offlineCount}</div>
                  <div className="text-xs text-text-mut mt-1">hosts offline</div>
                </div>
                <div>
                  <div className="stat-value">{filteredContainers.length}</div>
                  <div className="text-xs text-text-mut mt-1">containers ativos</div>
                </div>
              </div>
            </div>

            <div className="stat-card flex flex-col items-center justify-center">
              <span className="eyebrow">
                {now ? now.toLocaleDateString('pt-BR').replace(/\//g, '-') : 'Carregando...'}
              </span>
              <span className="stat-value text-4xl mt-2">
                {now ? now.toLocaleTimeString('pt-BR') : '00:00:00'}
              </span>
            </div>
          </div>

          {selectedServerId === 'all' && <LoadBalancerFlow stats={loadBalancing} />}

          <div className="grid grid-cols-1 md:grid-cols-3 gap-4 md:gap-6 mb-6 h-56 stagger">
            <Gauge value={cpuPercent} title="CPU do host" />
            <Gauge value={memPercent} title="Memória" />
            <Gauge value={diskPercent} title="Disco" />
          </div>

          <div className="grid grid-cols-1 md:grid-cols-2 gap-4 md:gap-6">
            <div className="panel p-6 flex flex-col relative h-80 overflow-hidden">
              <div className="flex items-center justify-between mb-4 border-b border-line pb-3">
                <span className="eyebrow">Consumo (ao vivo)</span>
                <span className="badge badge-ok">
                  <span className="w-1.5 h-1.5 rounded-full bg-ok animate-pulse" />
                  Live
                </span>
              </div>
              <div className="flex-1 overflow-y-auto pr-2 custom-scrollbar">
                <table className="table-base">
                  <thead className="sticky top-0 bg-ink-900 z-10">
                    <tr>
                      <th>Container</th>
                      <th className="text-right">CPU</th>
                      <th className="text-right">Memória</th>
                    </tr>
                  </thead>
                  <tbody>
                    {filteredContainers.length === 0 && (
                      <tr>
                        <td colSpan={3} className="text-center py-8 text-text-faint text-xs">Buscando dados no motor Go...</td>
                      </tr>
                    )}
                    {filteredContainers.map((c) => (
                      <tr key={c.docker_id}>
                        <td className="mono-data text-text selectable">{c.name}</td>
                        <td className="text-right mono-data text-text-hi">{c.cpu.toFixed(2)}%</td>
                        <td className="text-right mono-data text-text-hi">{formatBytes(c.mem_used)}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>

            <div className="panel p-6 flex flex-col relative h-80 overflow-hidden">
              <div className="flex items-center justify-between mb-4 border-b border-line pb-3">
                <span className="eyebrow">Espaço de disco ocupado</span>
                <span className="text-[11px] text-text-faint mono-data">
                  {activeServer ? `${activeServer.host_ip} · ${historyRange}` : 'Cluster'}
                </span>
              </div>

              {activeServer ? (
                <div className="flex-1 mt-2">
                  {diskHistory.length === 0 ? (
                    <div className="h-full flex items-center justify-center text-xs text-text-faint">
                      Sem histórico de disco na janela selecionada.
                    </div>
                  ) : (
                    <ResponsiveContainer width="100%" height="100%">
                      <AreaChart data={diskHistory} margin={{ top: 10, right: 0, left: -20, bottom: 0 }}>
                        <defs>
                          <linearGradient id="colorDisk" x1="0" y1="0" x2="0" y2="1">
                            <stop offset="5%" stopColor="var(--color-accent)" stopOpacity={0.12} />
                            <stop offset="95%" stopColor="var(--color-accent)" stopOpacity={0} />
                          </linearGradient>
                        </defs>
                        <CartesianGrid strokeDasharray="3 3" stroke="var(--color-line)" strokeOpacity={0.5} vertical={false} />
                        <XAxis
                          dataKey="time"
                          tick={{ fill: 'var(--color-text-faint)', fontSize: 11 }}
                          tickLine={false}
                          axisLine={false}
                          minTickGap={30}
                        />
                        <YAxis
                          tick={{ fill: 'var(--color-text-faint)', fontSize: 11 }}
                          tickLine={false}
                          axisLine={false}
                          domain={[0, 100]}
                          tickFormatter={(v) => `${v}%`}
                        />
                        <Tooltip
                          contentStyle={{
                            backgroundColor: 'var(--color-ink-800)',
                            border: '1px solid var(--color-line-hi)',
                            borderRadius: 10,
                            fontSize: 12,
                            fontFamily: 'var(--font-mono)',
                          }}
                          labelStyle={{ color: 'var(--color-text-mut)' }}
                          itemStyle={{ color: 'var(--color-text-hi)' }}
                          formatter={(value) => [`${value}%`, 'Disco']}
                        />
                        <Area
                          type="monotone"
                          dataKey="value"
                          stroke="var(--color-accent)"
                          strokeWidth={2}
                          fill="url(#colorDisk)"
                          isAnimationActive={false}
                        />
                      </AreaChart>
                    </ResponsiveContainer>
                  )}
                </div>
              ) : (
                <div className="flex-1 mt-2 overflow-y-auto custom-scrollbar flex flex-col gap-3">
                  {onlineServers.length === 0 && (
                    <div className="text-xs text-text-faint">Nenhum host online reportando disco.</div>
                  )}
                  {onlineServers.map((s) => {
                    const pct = s.disk_total > 0 ? (s.disk_used / s.disk_total) * 100 : 0;
                    return (
                      <div key={s.id} className="flex flex-col gap-1">
                        <div className="flex items-center justify-between text-[11px]">
                          <span className="flex items-center gap-2 text-text">
                            <HardDrive size={12} strokeWidth={1.75} className="text-text-faint" />
                            <span className="mono-data selectable">{s.host_ip}</span>
                          </span>
                          <span className="mono-data text-text-mut">
                            {formatGB(s.disk_used)} / {formatGB(s.disk_total)} GB
                          </span>
                        </div>
                        <div className="w-full h-1.5 bg-ink-750 rounded-full overflow-hidden">
                          <div
                            className={`h-full ${pct > 85 ? 'bg-crit' : 'bg-ok'}`}
                            style={{ width: `${Math.min(pct, 100)}%` }}
                          />
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

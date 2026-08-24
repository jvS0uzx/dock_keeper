import { useState, useEffect, useCallback, useMemo } from 'react';
import {
  ResponsiveContainer, AreaChart, Area, XAxis, YAxis, CartesianGrid, Tooltip,
} from 'recharts';
import {
  ArrowLeft, Cpu, MemoryStick, HardDrive, Thermometer, Activity, Clock,
  User, Wifi, Network, MonitorSmartphone, Terminal, ScrollText, RefreshCw,
} from 'lucide-react';
import {
  api,
  type HistoryMetric,
  type HistoryRange,
  type LogEntryRecord,
  type NetworkHostView,
  type ServerLiveStat,
  type Site,
} from '../lib/api';
import { formatGB, formatDateTime } from '../lib/format';
import {
  HANDSHAKE_LABEL,
  NO_HANDSHAKE_HINT,
  NO_TEMPERATURE_HINT,
  formatHandshake,
  formatTemperature,
} from '../lib/metrics';
import Select from './ui/Select';
import { useNavigation } from './ui/navigation-context';

interface MachineDetailViewProps {
  serverId: string;
}

const LIVE_POLL_MS = 10000;
const HISTORY_POLL_MS = 30000;
const LOG_LIMIT = 50;

// Limiares de alerta visual. Mesmos valores usados na lista de estações, para
// o operador não ver uma máquina "amarela" na lista e "normal" no detalhe.
const USAGE_WARN = 75;
const USAGE_CRITICAL = 90;
const TEMP_WARN = 70;
const TEMP_CRITICAL = 85;

const METRICS: { key: HistoryMetric; label: string; unit: string }[] = [
  { key: 'cpu', label: 'CPU', unit: '%' },
  { key: 'mem', label: 'Memória', unit: '%' },
  { key: 'disk', label: 'Disco', unit: '%' },
  { key: 'load', label: 'Load', unit: '' },
  // Temperatura só passou a valer no gráfico agora: antes o backend
  // recusava a métrica e apenas as estações tinham o dado. O stream SSH
  // passou a ler os sensores do host (achado 5 do QA).
  { key: 'temperature', label: 'Temperatura', unit: '°C' },
  // A chave enviada à API continua 'latency': renomeá-la quebraria o endpoint
  // de histórico, que a recebe como parâmetro. Só o rótulo foi corrigido.
  { key: 'latency', label: HANDSHAKE_LABEL, unit: 'ms' },
];

const RANGES: HistoryRange[] = ['1h', '6h', '24h', '7d'];

// Mesmos rótulos do inventário de rede, para o tipo não mudar de nome entre
// as telas.
const DEVICE_LABELS: Record<string, string> = {
  printer: 'Impressora',
  windows: 'Estação Windows',
  nas: 'NAS',
  linux: 'Linux',
  'web-device': 'Dispositivo Web',
  unknown: 'Desconhecido',
};

const usageColor = (pct: number) => {
  if (pct >= USAGE_CRITICAL) return 'text-rose-400';
  if (pct >= USAGE_WARN) return 'text-amber-400';
  return 'text-white';
};

const tempColor = (celsius: number) => {
  if (celsius >= TEMP_CRITICAL) return 'text-rose-400';
  if (celsius >= TEMP_WARN) return 'text-amber-400';
  return 'text-emerald-400';
};

const formatUptime = (seconds: number) => {
  if (!seconds) return '—';
  const days = Math.floor(seconds / 86400);
  const hours = Math.floor((seconds % 86400) / 3600);
  if (days > 0) return `${days}d ${hours}h`;
  if (hours > 0) return `${hours}h`;
  return `${Math.floor(seconds / 60)}min`;
};

const fmtTime = (iso: string, range: HistoryRange) => {
  const d = new Date(iso);
  if (range === '7d' || range === '24h') {
    return d.toLocaleString('pt-BR', { day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit' });
  }
  return d.toLocaleTimeString('pt-BR', { hour: '2-digit', minute: '2-digit' });
};

interface StatProps {
  label: string;
  value: string;
  hint?: string;
  accent?: string;
  // Explica por que o valor está ausente, sem gastar linha no cartão.
  title?: string;
  Icon: typeof Cpu;
}

const Stat = ({ label, value, hint, accent = 'text-white', title, Icon }: StatProps) => (
  <div className="glass-panel rounded-xl p-4 border border-white/5 bg-white/[0.02]" title={title}>
    <div className="flex items-center gap-2 mb-2">
      <Icon size={13} className="text-[#737373]" />
      <span className="text-[10px] text-[#737373] uppercase tracking-widest">{label}</span>
    </div>
    <div className={`text-2xl font-bold ${accent}`}>{value}</div>
    {hint && <div className="text-[10px] text-[#737373] mt-1">{hint}</div>}
  </div>
);

const Panel = ({ title, Icon, children }: { title: string; Icon: typeof Cpu; children: React.ReactNode }) => (
  <div className="glass-panel rounded-xl border border-white/5 bg-white/[0.02] overflow-hidden">
    <div className="flex items-center gap-2 px-4 py-3 border-b border-white/5">
      <Icon size={14} className="text-[#10b981]" />
      <h2 className="text-xs font-bold tracking-widest text-[#737373] uppercase">{title}</h2>
    </div>
    <div className="p-4">{children}</div>
  </div>
);

const Field = ({ label, value }: { label: string; value: string }) => (
  <div>
    <div className="text-[10px] text-[#737373] uppercase tracking-widest">{label}</div>
    <div className="text-sm text-white/90 mt-0.5">{value || <span className="text-gray-600">—</span>}</div>
  </div>
);

const MachineDetailView = ({ serverId }: MachineDetailViewProps) => {
  const { goBack } = useNavigation();

  const [machine, setMachine] = useState<ServerLiveStat | null>(null);
  const [sites, setSites] = useState<Site[]>([]);
  const [inventory, setInventory] = useState<NetworkHostView | null>(null);
  const [logs, setLogs] = useState<LogEntryRecord[]>([]);
  const [loading, setLoading] = useState(true);

  const [metric, setMetric] = useState<HistoryMetric>('cpu');
  const [range, setRange] = useState<HistoryRange>('1h');
  const [history, setHistory] = useState<{ time: string; value: number }[]>([]);
  const [loadingHistory, setLoadingHistory] = useState(false);

  const activeMetric = METRICS.find((m) => m.key === metric) ?? METRICS[0];

  // Estado ao vivo da máquina. O endpoint devolve o parque inteiro; o recorte
  // por id acontece aqui porque não há rota de servidor individual.
  const fetchLive = useCallback(async (signal?: AbortSignal) => {
    try {
      const data = await api.liveMetrics(signal);
      setMachine(data.servers.find((s) => s.id === serverId) ?? null);
    } catch (err) {
      if (!signal?.aborted) console.error(err);
    } finally {
      setLoading(false);
    }
  }, [serverId]);

  useEffect(() => {
    const controller = new AbortController();
    fetchLive(controller.signal);
    const interval = setInterval(() => fetchLive(controller.signal), LIVE_POLL_MS);
    return () => {
      clearInterval(interval);
      controller.abort();
    };
  }, [fetchLive]);

  // Unidade e logs mudam pouco: uma leitura por máquina aberta basta.
  //
  // Nem sites() nem searchLogs() aceitam AbortSignal, então a guarda de
  // desmontagem é uma flag — abortar aqui seria um controller sem ouvinte.
  useEffect(() => {
    let cancelled = false;

    api.sites()
      .then((list) => { if (!cancelled) setSites(list); })
      .catch(() => {});
    api.searchLogs({ server_id: serverId, limit: String(LOG_LIMIT) })
      .then((list) => { if (!cancelled) setLogs(list); })
      .catch(() => {});

    return () => { cancelled = true; };
  }, [serverId]);

  // O inventário é indexado por IP, não por id de servidor: o cruzamento só é
  // possível depois que o estado ao vivo trouxe o endereço da máquina.
  useEffect(() => {
    if (!machine?.host_ip) return;
    const controller = new AbortController();
    api.networkHosts(controller.signal)
      .then((inv) => setInventory(inv.hosts.find((h) => h.ip === machine.host_ip) ?? null))
      .catch(() => {});
    return () => controller.abort();
  }, [machine?.host_ip]);

  const fetchHistory = useCallback(async (signal?: AbortSignal) => {
    setLoadingHistory(true);
    try {
      const points = await api.history(serverId, metric, range, signal);
      setHistory(points.map((p) => ({
        time: fmtTime(p.ts, range),
        value: Number(p.value.toFixed(2)),
      })));
    } catch (err) {
      if (!signal?.aborted) {
        console.error(err);
        setHistory([]);
      }
    } finally {
      setLoadingHistory(false);
    }
  }, [serverId, metric, range]);

  useEffect(() => {
    const controller = new AbortController();
    fetchHistory(controller.signal);
    const interval = setInterval(() => fetchHistory(controller.signal), HISTORY_POLL_MS);
    return () => {
      clearInterval(interval);
      controller.abort();
    };
  }, [fetchHistory]);

  const siteName = useMemo(() => {
    if (!machine?.site_id) return 'Sem unidade';
    return sites.find((s) => s.id === machine.site_id)?.name ?? 'Sem unidade';
  }, [machine?.site_id, sites]);

  const backButton = (
    <button
      onClick={goBack}
      className="flex items-center gap-2 text-xs font-bold uppercase tracking-widest text-[#737373] hover:text-white transition-colors"
    >
      <ArrowLeft size={14} />
      Voltar
    </button>
  );

  if (loading) {
    return <div className="p-8 text-sm text-[#737373]">Carregando máquina...</div>;
  }

  if (!machine) {
    return (
      <div className="p-8 flex flex-col items-start gap-4">
        {backButton}
        <div className="glass-panel rounded-xl border border-white/5 bg-white/[0.02] p-8 text-sm text-[#737373]">
          Máquina não encontrada. Ela pode ter sido removida do painel ou estar
          fora do seu alcance de unidade.
        </div>
      </div>
    );
  }

  // Gráfico vazio tem duas causas muito diferentes: ou não houve coleta no
  // período, ou esta fonte simplesmente não mede a métrica escolhida. Dizer
  // "sem dados" nos dois casos manda o operador procurar defeito onde não há.
  let emptyChartMessage = 'Sem dados no período selecionado.';
  if (metric === 'latency' && machine.kind !== 'ssh') {
    emptyChartMessage = NO_HANDSHAKE_HINT;
  } else if (metric === 'temperature' && machine.temperature_c === null) {
    // A leitura instantânea nula é a melhor pista de que a máquina não tem
    // sensor: se não mede agora, também não mediu no período do gráfico.
    emptyChartMessage = NO_TEMPERATURE_HINT;
  }

  const memPct = machine.mem_total > 0 ? (machine.mem_used / machine.mem_total) * 100 : 0;
  const diskPct = machine.disk_total > 0 ? (machine.disk_used / machine.disk_total) * 100 : 0;

  return (
    <div className="p-4 md:p-8 flex flex-col gap-6">
      <div className="flex flex-col gap-4">
        {backButton}

        <div className="flex flex-col md:flex-row md:items-center justify-between gap-3">
          <div>
            <h1 className="text-2xl font-light text-white mb-2 flex items-center gap-3">
              <MonitorSmartphone className="text-[#10b981]" />
              <span className="font-bold">{machine.name}</span>
            </h1>
            <div className="flex items-center gap-3 flex-wrap text-sm">
              <span className="font-mono text-[#737373] selectable">{machine.host_ip}</span>
              <span className="text-[#737373]">·</span>
              <span className="text-gray-400">{siteName}</span>
            </div>
          </div>

          <div className="flex items-center gap-2">
            <span
              className={`px-2 py-1 rounded text-[10px] font-bold uppercase tracking-widest border ${
                machine.online
                  ? 'text-emerald-400 bg-emerald-400/10 border-emerald-400/20'
                  : 'text-[#737373] bg-white/5 border-white/10'
              }`}
            >
              {machine.online ? 'Online' : 'Offline'}
            </span>
            <span className="px-2 py-1 rounded text-[10px] font-bold uppercase tracking-widest border border-white/10 text-gray-400 bg-white/5">
              {machine.kind === 'agent' ? 'Agente' : 'SSH'}
            </span>
          </div>
        </div>
      </div>

      <div className="grid grid-cols-2 md:grid-cols-4 gap-3">
        <Stat
          label="CPU"
          Icon={Cpu}
          value={machine.online ? `${machine.cpu.toFixed(0)}%` : '—'}
          accent={machine.online ? usageColor(machine.cpu) : 'text-gray-600'}
        />
        <Stat
          label="Memória"
          Icon={MemoryStick}
          value={machine.online && machine.mem_total > 0 ? `${memPct.toFixed(0)}%` : '—'}
          hint={machine.mem_total > 0 ? `${formatGB(machine.mem_used)} / ${formatGB(machine.mem_total)} GB` : undefined}
          accent={machine.online ? usageColor(memPct) : 'text-gray-600'}
        />
        <Stat
          label="Disco"
          Icon={HardDrive}
          value={machine.disk_total > 0 ? `${diskPct.toFixed(0)}%` : '—'}
          hint={machine.disk_total > 0 ? `${formatGB(machine.disk_used)} / ${formatGB(machine.disk_total)} GB` : undefined}
          accent={usageColor(diskPct)}
        />
        <Stat
          label="Temperatura"
          Icon={Thermometer}
          value={formatTemperature(machine.temperature_c)}
          accent={machine.temperature_c !== null ? tempColor(machine.temperature_c) : 'text-gray-600'}
          title={machine.temperature_c === null ? NO_TEMPERATURE_HINT : undefined}
        />
        <Stat
          label="Load (1min)"
          Icon={Activity}
          value={machine.online ? machine.load1.toFixed(2) : '—'}
          accent={machine.online ? 'text-white' : 'text-gray-600'}
        />
        <Stat
          label="Ligada há"
          Icon={Clock}
          value={machine.online ? formatUptime(machine.uptime) : '—'}
          accent={machine.online ? 'text-white' : 'text-gray-600'}
        />
        <Stat
          label="Usuário logado"
          Icon={User}
          value={machine.last_user || '—'}
          accent={machine.last_user ? 'text-white' : 'text-gray-600'}
        />
        <Stat
          label={HANDSHAKE_LABEL}
          Icon={Wifi}
          value={formatHandshake(machine.ssh_handshake_ms)}
          accent={machine.ssh_handshake_ms !== null ? 'text-white' : 'text-gray-600'}
          title={machine.ssh_handshake_ms === null ? NO_HANDSHAKE_HINT : undefined}
        />
      </div>

      <div className="glass-panel rounded-xl border border-white/5 bg-white/[0.02] p-4 md:p-6">
        <div className="flex flex-wrap items-end gap-4 mb-6">
          <div className="flex flex-col gap-1">
            <label htmlFor="machine-metric" className="text-[10px] text-[#737373] uppercase tracking-widest">
              Métrica
            </label>
            <Select
              id="machine-metric"
              value={metric}
              onChange={(v) => setMetric(v as HistoryMetric)}
              className="min-w-[160px]"
              options={METRICS.map((m) => ({ value: m.key, label: m.label }))}
            />
          </div>

          <div className="flex flex-col gap-1">
            <span className="text-[10px] text-[#737373] uppercase tracking-widest">Período</span>
            <div className="flex gap-1">
              {RANGES.map((r) => (
                <button
                  key={r}
                  onClick={() => setRange(r)}
                  className={`px-3 py-2 rounded-lg text-xs font-bold tracking-widest uppercase transition-all border ${
                    range === r
                      ? 'bg-[#10b981]/20 border-[#10b981]/50 text-[#10b981]'
                      : 'bg-black/40 border-white/10 text-[#737373] hover:text-white'
                  }`}
                >
                  {r}
                </button>
              ))}
            </div>
          </div>

          <button
            onClick={() => fetchHistory()}
            className="ml-auto flex items-center gap-2 text-xs text-[#737373] hover:text-[#10b981] transition-colors"
          >
            <RefreshCw size={14} className={loadingHistory ? 'animate-spin' : ''} />
            Atualizar
          </button>
        </div>

        <div className="h-[320px] w-full">
          {loadingHistory && history.length === 0 ? (
            <div className="h-full flex items-center justify-center text-sm text-[#737373]">Carregando...</div>
          ) : history.length === 0 ? (
            <div className="h-full flex items-center justify-center px-6 text-center text-sm text-[#737373]">
              {emptyChartMessage}
            </div>
          ) : (
            <ResponsiveContainer width="100%" height="100%">
              <AreaChart data={history} margin={{ top: 10, right: 20, left: 0, bottom: 0 }}>
                <defs>
                  <linearGradient id="machineMetricFill" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="0%" stopColor="#10b981" stopOpacity={0.35} />
                    <stop offset="100%" stopColor="#10b981" stopOpacity={0} />
                  </linearGradient>
                </defs>
                <CartesianGrid strokeDasharray="3 3" stroke="rgba(255,255,255,0.05)" />
                <XAxis
                  dataKey="time"
                  tick={{ fill: '#737373', fontSize: 11 }}
                  stroke="rgba(255,255,255,0.1)"
                  minTickGap={30}
                />
                <YAxis
                  tick={{ fill: '#737373', fontSize: 11 }}
                  stroke="rgba(255,255,255,0.1)"
                  width={48}
                  unit={activeMetric.unit}
                />
                <Tooltip
                  contentStyle={{
                    background: '#0c0c0e',
                    border: '1px solid rgba(255,255,255,0.1)',
                    borderRadius: 8,
                    color: '#fff',
                    fontSize: 12,
                  }}
                  labelStyle={{ color: '#737373' }}
                  formatter={(value) => [`${value}${activeMetric.unit}`, activeMetric.label]}
                />
                <Area
                  type="monotone"
                  dataKey="value"
                  stroke="#10b981"
                  strokeWidth={2}
                  fill="url(#machineMetricFill)"
                  dot={false}
                  isAnimationActive={false}
                />
              </AreaChart>
            </ResponsiveContainer>
          )}
        </div>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <Panel title="Inventário de Rede" Icon={Network}>
          {!inventory ? (
            <p className="text-sm text-[#737373]">
              O endereço {machine.host_ip} não aparece no inventário. Ou a varredura
              ainda não passou por esta faixa, ou a máquina está fora dela.
            </p>
          ) : (
            <div className="flex flex-col gap-4">
              <div className="grid grid-cols-2 gap-4">
                <Field label="MAC" value={inventory.mac} />
                <Field label="Tipo" value={DEVICE_LABELS[inventory.device_type] ?? inventory.device_type} />
              </div>

              <div>
                <div className="text-[10px] text-[#737373] uppercase tracking-widest mb-1.5">Portas abertas</div>
                {inventory.open_ports.length === 0 ? (
                  <span className="text-sm text-gray-600">—</span>
                ) : (
                  <div className="flex gap-1 flex-wrap">
                    {inventory.open_ports.map((port) => (
                      <span
                        key={port}
                        className="bg-white/5 text-gray-400 px-1.5 py-0.5 rounded text-[10px] font-mono border border-white/5"
                      >
                        {port}
                      </span>
                    ))}
                  </div>
                )}
              </div>

              <div className="grid grid-cols-2 gap-4 pt-3 border-t border-white/5">
                <Field label="Andar" value={inventory.floor} />
                <Field label="Setor" value={inventory.sector} />
                <Field label="Sala" value={inventory.room} />
                <Field label="Rack" value={inventory.rack} />
                <Field label="Patrimônio" value={inventory.asset_tag} />
                <Field label="Responsável" value={inventory.owner} />
              </div>

              {inventory.notes && (
                <div className="pt-3 border-t border-white/5">
                  <Field label="Observações" value={inventory.notes} />
                </div>
              )}
            </div>
          )}
        </Panel>

        <Panel title="Sistema" Icon={Terminal}>
          <div className="grid grid-cols-2 gap-4">
            <Field label="Sistema operacional" value={machine.os} />
            <Field label="Plataforma" value={machine.platform} />
            <Field label="Arquitetura" value={machine.arch} />
            <Field
              label="Versão do agente"
              value={machine.agent_version || (machine.kind === 'ssh' ? 'coletada por SSH' : '')}
            />
          </div>
        </Panel>
      </div>

      <Panel title={`Últimas linhas de log (${logs.length})`} Icon={ScrollText}>
        {logs.length === 0 ? (
          <p className="text-sm text-[#737373]">Nenhuma linha de log registrada para esta máquina.</p>
        ) : (
          <div className="max-h-80 overflow-y-auto custom-scrollbar font-mono text-[11px] flex flex-col gap-1">
            {logs.map((log) => (
              <div key={log.id} className="flex gap-3 break-all selectable">
                <span className="text-gray-600 shrink-0">{formatDateTime(log.timestamp)}</span>
                <span className="text-[#737373] shrink-0">{log.container || log.source}</span>
                <span className="text-white/80">{log.line}</span>
              </div>
            ))}
          </div>
        )}
      </Panel>
    </div>
  );
};

export default MachineDetailView;

import { useState, useEffect, useCallback, useMemo } from 'react';
import {
  ResponsiveContainer, AreaChart, Area, XAxis, YAxis, CartesianGrid, Tooltip, ReferenceLine,
} from 'recharts';
import {
  ArrowLeft, Cpu, MemoryStick, HardDrive, Thermometer, Activity, Clock,
  User, Wifi, Network, MonitorSmartphone, Terminal, ScrollText, RefreshCw,
  ArrowDownToLine, ArrowUpFromLine, Timer,
} from 'lucide-react';
import {
  api,
  apiErrorMessage,
  type HistoryRange,
  type LogEntryRecord,
  type NetworkHostView,
  type ServerLiveStat,
  type Site,
} from '../lib/api';
import { formatGB, formatDateTime, formatLatency, formatLoad, formatPercent, formatRate } from '../lib/format';
import {
  HANDSHAKE_LABEL,
  NO_HANDSHAKE_HINT,
  NO_NETWORK_HINT,
  NO_RTT_HINT,
  NO_TEMPERATURE_HINT,
  formatHandshake,
  ehTaxa,
  formatarPorUnidade,
  metricasDoHistorico,
  formatTemperature,
} from '../lib/metrics';
import Select from './ui/Select';
import { useNavigation } from './ui/navigation-context';
import LoadNotice from './ui/LoadNotice';
import MachineAdminPanel from './MachineAdminPanel';
import { useCatalogo } from './ui/useCatalogo';
import { useLoadStatus } from './ui/load-status';
import { POLL } from '../lib/polling';

interface MachineDetailViewProps {
  serverId: string;
}

const LOG_LIMIT = 50;

const USAGE_WARN = 75;
const USAGE_CRITICAL = 90;
const TEMP_WARN = 70;
const TEMP_CRITICAL = 85;

const RANGES: HistoryRange[] = ['1h', '6h', '24h', '7d'];

const LIMIAR_POR_UNIDADE: Record<string, number> = {
  '%': USAGE_WARN,
  '°C': TEMP_WARN,
};

const DEVICE_LABELS: Record<string, string> = {
  printer: 'Impressora',
  windows: 'Estação Windows',
  nas: 'NAS',
  linux: 'Linux',
  'web-device': 'Dispositivo Web',
  unknown: 'Desconhecido',
};

const usageColor = (pct: number) => {
  if (pct >= USAGE_CRITICAL) return 'text-crit';
  if (pct >= USAGE_WARN) return 'text-warn';
  return 'text-text-hi';
};

const tempColor = (celsius: number) => {
  if (celsius >= TEMP_CRITICAL) return 'text-crit';
  if (celsius >= TEMP_WARN) return 'text-warn';
  return 'text-ok';
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
  title?: string;
  Icon: typeof Cpu;
}

const Stat = ({ label, value, hint, accent = '', title, Icon }: StatProps) => (
  <div className="stat-card !p-4" title={title}>
    <div className="flex items-center gap-2 mb-2">
      <Icon size={13} strokeWidth={1.75} className="text-text-faint" />
      <span className="eyebrow">{label}</span>
    </div>
    <div className={`stat-value ${accent}`}>{value}</div>
    {hint && <div className="text-[11px] text-text-faint mono-data mt-1">{hint}</div>}
  </div>
);

const Panel = ({ title, Icon, children }: { title: string; Icon: typeof Cpu; children: React.ReactNode }) => (
  <div className="panel overflow-hidden">
    <div className="flex items-center gap-2 px-4 py-3 border-b border-line">
      <Icon size={16} strokeWidth={1.75} className="text-accent" />
      <h2 className="eyebrow">{title}</h2>
    </div>
    <div className="p-4">{children}</div>
  </div>
);

const Field = ({ label, value }: { label: string; value: string }) => (
  <div>
    <div className="eyebrow">{label}</div>
    <div className="text-sm text-text mt-0.5">{value || <span className="text-text-faint">—</span>}</div>
  </div>
);

const MachineDetailView = ({ serverId }: MachineDetailViewProps) => {
  const { goBack } = useNavigation();

  const [machine, setMachine] = useState<ServerLiveStat | null>(null);
  const [sites, setSites] = useState<Site[]>([]);
  const [inventory, setInventory] = useState<NetworkHostView | null>(null);
  const [logs, setLogs] = useState<LogEntryRecord[]>([]);
  const [loading, setLoading] = useState(true);
  const live = useLoadStatus();
  const { ok: liveOk, fail: liveFail } = live;
  const [sitesError, setSitesError] = useState<string | null>(null);
  const [logsError, setLogsError] = useState<string | null>(null);
  const [inventoryError, setInventoryError] = useState<string | null>(null);
  const [historyError, setHistoryError] = useState<string | null>(null);

  const catalogo = useCatalogo();
  const opcoesDeMetrica = useMemo(() => metricasDoHistorico(catalogo.metricas), [catalogo.metricas]);
  const [escolhida, setMetric] = useState('');
  const metric = escolhida || (opcoesDeMetrica[0]?.nome ?? '');
  const [range, setRange] = useState<HistoryRange>('1h');
  const [history, setHistory] = useState<{ time: string; value: number }[]>([]);
  const [loadingHistory, setLoadingHistory] = useState(false);

  const unidade = catalogo.unidade(metric);
  const threshold = LIMIAR_POR_UNIDADE[unidade];

  const fetchLive = useCallback(async (signal?: AbortSignal) => {
    try {
      const data = await api.liveMetrics(signal);
      setMachine(data.servers.find((s) => s.id === serverId) ?? null);
      liveOk();
    } catch (err) {
      if (!signal?.aborted) liveFail(err, 'Falha ao ler a máquina.');
    } finally {
      setLoading(false);
    }
  }, [serverId, liveOk, liveFail]);

  useEffect(() => {
    const controller = new AbortController();
    fetchLive(controller.signal);
    const interval = setInterval(() => fetchLive(controller.signal), POLL.maquina);
    return () => {
      clearInterval(interval);
      controller.abort();
    };
  }, [fetchLive]);

  useEffect(() => {
    let cancelled = false;

    api.sites()
      .then((list) => {
        if (cancelled) return;
        setSites(list);
        setSitesError(null);
      })
      .catch((err) => { if (!cancelled) setSitesError(apiErrorMessage(err, 'Falha ao ler as unidades.')); });
    api.searchLogs({ server_id: serverId, limit: String(LOG_LIMIT) })
      .then((list) => {
        if (cancelled) return;
        setLogs(list);
        setLogsError(null);
      })
      .catch((err) => { if (!cancelled) setLogsError(apiErrorMessage(err, 'Falha ao ler os logs.')); });

    return () => { cancelled = true; };
  }, [serverId]);

  useEffect(() => {
    if (!machine?.host_ip) return;
    const controller = new AbortController();
    api.networkHosts(controller.signal)
      .then((inv) => {
        setInventory(inv.hosts.find((h) => h.ip === machine.host_ip) ?? null);
        setInventoryError(null);
      })
      .catch((err) => {
        if (!controller.signal.aborted) setInventoryError(apiErrorMessage(err, 'Falha ao ler o inventário.'));
      });
    return () => controller.abort();
  }, [machine?.host_ip]);

  const fetchHistory = useCallback(async (signal?: AbortSignal) => {
    if (!metric) return;
    setLoadingHistory(true);
    try {
      const points = await api.history(serverId, metric, range, signal);
      setHistory(points.map((p) => ({
        time: fmtTime(p.ts, range),
        value: Number(p.value.toFixed(2)),
      })));
      setHistoryError(null);
    } catch (err) {
      if (!signal?.aborted) {
        setHistory([]);
        setHistoryError(apiErrorMessage(err, 'Falha ao ler o histórico.'));
      }
    } finally {
      setLoadingHistory(false);
    }
  }, [serverId, metric, range]);

  useEffect(() => {
    const controller = new AbortController();
    fetchHistory(controller.signal);
    const interval = setInterval(() => fetchHistory(controller.signal), POLL.historicoDaMaquina);
    return () => {
      clearInterval(interval);
      controller.abort();
    };
  }, [fetchHistory]);

  const siteName = useMemo(() => {
    if (!machine?.site_id) return 'Sem unidade';
    if (sitesError) return 'Unidade indisponível';
    return sites.find((s) => s.id === machine.site_id)?.name ?? 'Sem unidade';
  }, [machine?.site_id, sites, sitesError]);

  const backButton = (
    <button
      onClick={goBack}
      className="flex items-center gap-2 text-xs font-semibold text-text-mut hover:text-text-hi transition-colors"
    >
      <ArrowLeft size={14} strokeWidth={1.75} />
      Voltar
    </button>
  );

  if (loading) {
    return <div className="p-8 text-sm text-text-mut">Carregando máquina...</div>;
  }

  if (!machine) {
    return (
      <div className="p-8 flex flex-col items-start gap-4">
        {backButton}
        <div className="panel p-8 text-sm text-text-mut">
          {live.error ? (
            <LoadNotice error={live.error} />
          ) : (
            <>
              Máquina não encontrada. Ela pode ter sido removida do painel ou estar
              fora do seu alcance de unidade.
            </>
          )}
        </div>
      </div>
    );
  }

  let emptyChartMessage = 'Sem dados no período selecionado.';
  if (metric === 'latency' && machine.kind !== 'ssh') {
    emptyChartMessage = NO_HANDSHAKE_HINT;
  } else if (metric === 'temperature' && machine.temperature_c === null) {
    emptyChartMessage = NO_TEMPERATURE_HINT;
  } else if (metric === 'rtt' && machine.kind !== 'ssh') {
    emptyChartMessage = NO_RTT_HINT;
  } else if (ehTaxa(unidade) && machine.net_rx_bps === null && machine.net_tx_bps === null) {
    emptyChartMessage = NO_NETWORK_HINT;
  }

  const memPct = machine.mem_total > 0 ? (machine.mem_used / machine.mem_total) * 100 : 0;
  const diskPct = machine.disk_total > 0 ? (machine.disk_used / machine.disk_total) * 100 : 0;

  return (
    <div className="p-4 md:p-8 flex flex-col gap-6 anim-rise">
      <div className="flex flex-col gap-4">
        {backButton}

        <LoadNotice error={live.error} lastOk={live.lastOk} />

        <div className="page-header !mb-0">
          <div>
            <h1 className="page-title flex items-center gap-3">
              <MonitorSmartphone size={22} strokeWidth={1.75} className="text-accent" />
              {machine.name}
            </h1>
            <div className="flex items-center gap-3 flex-wrap text-sm mt-1.5">
              <span className="mono-data text-text-mut selectable">{machine.host_ip}</span>
              <span className="text-text-faint">·</span>
              <span className="text-text-mut">{siteName}</span>
            </div>
          </div>

          <div className="flex items-center gap-2">
            <span className={`badge ${machine.online ? 'badge-ok' : 'badge-muted'}`}>
              <span className={`w-1.5 h-1.5 rounded-full ${machine.online ? 'bg-ok animate-pulse' : 'bg-text-faint'}`} />
              {machine.online ? 'Online' : 'Offline'}
            </span>
            <span className="badge badge-muted">
              {machine.kind === 'agent' ? 'Agente' : 'SSH'}
            </span>
          </div>
        </div>
      </div>

      <div className="grid grid-cols-2 md:grid-cols-4 xl:grid-cols-5 gap-3 stagger">
        <Stat
          label="CPU"
          Icon={Cpu}
          value={machine.online ? formatPercent(machine.cpu) : '—'}
          accent={machine.online && machine.cpu !== null ? usageColor(machine.cpu) : 'text-text-faint'}
        />
        <Stat
          label="Memória"
          Icon={MemoryStick}
          value={machine.online && machine.mem_total > 0 ? `${memPct.toFixed(0)}%` : '—'}
          hint={machine.mem_total > 0 ? `${formatGB(machine.mem_used)} / ${formatGB(machine.mem_total)} GB` : undefined}
          accent={machine.online ? usageColor(memPct) : 'text-text-faint'}
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
          accent={machine.temperature_c !== null ? tempColor(machine.temperature_c) : 'text-text-faint'}
          title={machine.temperature_c === null ? NO_TEMPERATURE_HINT : undefined}
        />
        <Stat
          label="Load (1min)"
          Icon={Activity}
          value={machine.online ? formatLoad(machine.load1) : '—'}
          accent={machine.online && machine.load1 !== null ? '' : 'text-text-faint'}
        />
        <Stat
          label="Ligada há"
          Icon={Clock}
          value={machine.online ? formatUptime(machine.uptime) : '—'}
          accent={machine.online ? '' : 'text-text-faint'}
        />
        <Stat
          label="Usuário logado"
          Icon={User}
          value={machine.last_user || '—'}
          accent={machine.last_user ? '' : 'text-text-faint'}
        />
        <Stat
          label={HANDSHAKE_LABEL}
          Icon={Wifi}
          value={formatHandshake(machine.ssh_handshake_ms)}
          accent={machine.ssh_handshake_ms !== null ? '' : 'text-text-faint'}
          title={machine.ssh_handshake_ms === null ? NO_HANDSHAKE_HINT : undefined}
        />
        <Stat
          label="Latência"
          Icon={Timer}
          value={formatLatency(machine.rtt_ms)}
          accent={machine.rtt_ms !== null ? '' : 'text-text-faint'}
          title={machine.rtt_ms === null ? NO_RTT_HINT : undefined}
        />
        <Stat
          label="Rede RX"
          Icon={ArrowDownToLine}
          value={formatRate(machine.net_rx_bps)}
          accent={machine.net_rx_bps !== null ? '' : 'text-text-faint'}
          title={machine.net_rx_bps === null ? NO_NETWORK_HINT : undefined}
        />
        <Stat
          label="Rede TX"
          Icon={ArrowUpFromLine}
          value={formatRate(machine.net_tx_bps)}
          accent={machine.net_tx_bps !== null ? '' : 'text-text-faint'}
          title={machine.net_tx_bps === null ? NO_NETWORK_HINT : undefined}
        />
      </div>

      <div className="panel p-4 md:p-6">
        <LoadNotice error={catalogo.erro} className="mb-4" />
        <div className="flex flex-wrap items-end gap-4 mb-6">
          <div className="flex flex-col gap-1.5">
            <label htmlFor="machine-metric" className="eyebrow">
              Métrica
            </label>
            <Select
              id="machine-metric"
              value={metric}
              onChange={setMetric}
              className="min-w-[160px]"
              options={opcoesDeMetrica.map((m) => ({ value: m.nome, label: m.rotulo }))}
            />
          </div>

          <div className="flex flex-col gap-1.5">
            <span className="eyebrow">Período</span>
            <div className="flex gap-1">
              {RANGES.map((r) => (
                <button
                  key={r}
                  onClick={() => setRange(r)}
                  className={`btn text-xs ${
                    range === r
                      ? 'bg-accent/10 border border-accent/40 text-accent'
                      : 'btn-ghost'
                  }`}
                >
                  {r}
                </button>
              ))}
            </div>
          </div>

          <button
            onClick={() => fetchHistory()}
            className="btn btn-ghost ml-auto text-xs"
          >
            <RefreshCw size={14} strokeWidth={1.75} className={loadingHistory ? 'animate-spin' : ''} />
            Atualizar
          </button>
        </div>

        <div className="h-[320px] w-full">
          {loadingHistory && history.length === 0 ? (
            <div className="h-full flex items-center justify-center text-sm text-text-faint">Carregando...</div>
          ) : history.length === 0 ? (
            <div className="h-full flex items-center justify-center px-6 text-center text-sm text-text-faint">
              {historyError ? <LoadNotice error={historyError} /> : emptyChartMessage}
            </div>
          ) : (
            <ResponsiveContainer width="100%" height="100%">
              <AreaChart data={history} margin={{ top: 10, right: 20, left: 0, bottom: 0 }}>
                <defs>
                  <linearGradient id="machineMetricFill" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="0%" stopColor="var(--color-accent)" stopOpacity={0.12} />
                    <stop offset="100%" stopColor="var(--color-accent)" stopOpacity={0} />
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
                  width={ehTaxa(unidade) ? 72 : 48}
                  tickFormatter={(v: number) => formatarPorUnidade(unidade, v)}
                />
                <Tooltip
                  contentStyle={{
                    background: 'var(--color-ink-800)',
                    border: '1px solid var(--color-line-hi)',
                    borderRadius: 10,
                    fontSize: 12,
                    fontFamily: 'var(--font-mono)',
                  }}
                  labelStyle={{ color: 'var(--color-text-mut)' }}
                  itemStyle={{ color: 'var(--color-text-hi)' }}
                  formatter={(value) => [formatarPorUnidade(unidade, Number(value)), catalogo.rotulo(metric)]}
                />
                {threshold !== undefined && (
                  <ReferenceLine
                    y={threshold}
                    stroke="var(--color-warn)"
                    strokeDasharray="4 4"
                    strokeOpacity={0.7}
                  />
                )}
                <Area
                  type="monotone"
                  dataKey="value"
                  stroke="var(--color-accent)"
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
          {inventoryError ? (
            <LoadNotice error={inventoryError} />
          ) : !inventory ? (
            <p className="text-sm text-text-mut">
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
                <div className="eyebrow mb-1.5">Portas abertas</div>
                {inventory.open_ports.length === 0 ? (
                  <span className="text-sm text-text-faint">—</span>
                ) : (
                  <div className="flex gap-1 flex-wrap">
                    {inventory.open_ports.map((port) => (
                      <span
                        key={port}
                        className="bg-ink-800 text-text-mut px-1.5 py-0.5 rounded-md text-[10px] mono-data border border-line"
                      >
                        {port}
                      </span>
                    ))}
                  </div>
                )}
              </div>

              <div className="grid grid-cols-2 gap-4 pt-3 border-t border-line">
                <Field label="Andar" value={inventory.floor} />
                <Field label="Setor" value={inventory.sector} />
                <Field label="Sala" value={inventory.room} />
                <Field label="Rack" value={inventory.rack} />
                <Field label="Patrimônio" value={inventory.asset_tag} />
                <Field label="Responsável" value={inventory.owner} />
              </div>

              {inventory.notes && (
                <div className="pt-3 border-t border-line">
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

      <MachineAdminPanel machine={machine} />

      <Panel title={`Últimas linhas de log (${logs.length})`} Icon={ScrollText}>
        {logsError ? (
          <LoadNotice error={logsError} />
        ) : logs.length === 0 ? (
          <p className="text-sm text-text-mut">Nenhuma linha de log registrada para esta máquina.</p>
        ) : (
          <div className="max-h-80 overflow-y-auto custom-scrollbar font-mono text-[11px] flex flex-col gap-1">
            {logs.map((log) => (
              <div key={log.id} className="flex gap-3 break-all selectable">
                <span className="text-text-faint shrink-0">{formatDateTime(log.timestamp)}</span>
                <span className="text-text-mut shrink-0">{log.container || log.source}</span>
                <span className="text-text">{log.line}</span>
              </div>
            ))}
          </div>
        )}
      </Panel>
    </div>
  );
};

export default MachineDetailView;

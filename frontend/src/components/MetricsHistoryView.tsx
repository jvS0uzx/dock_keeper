import { useState, useEffect, useCallback } from 'react';
import {
  ResponsiveContainer,
  AreaChart,
  Area,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
} from 'recharts';
import { RefreshCw } from 'lucide-react';
import { api, type HistoryMetric, type HistoryRange } from '../lib/api';
import { HANDSHAKE_LABEL } from '../lib/metrics';
import Select from './ui/Select';

interface ServerOption {
  id: string;
  name: string;
}

interface ChartRow {
  ts: string;
  time: string;
  value: number;
}

const METRICS: { key: HistoryMetric; label: string; unit: string }[] = [
  { key: 'cpu', label: 'CPU', unit: '%' },
  { key: 'mem', label: 'Memória', unit: '%' },
  { key: 'disk', label: 'Disco', unit: '%' },
  { key: 'load', label: 'Load', unit: '' },
  // Temperatura só passou a valer no gráfico agora: antes o backend
  // recusava a métrica e apenas as estações tinham o dado. O stream SSH
  // passou a ler os sensores do host (achado 5 do QA).
  { key: 'temperature', label: 'Temperatura', unit: '°C' },
  // A chave 'latency' continua sendo o que a API espera; renomeá-la quebraria o
  // endpoint de histórico. O rótulo é que estava errado: o valor é o handshake
  // SSH completo, não latência de rede.
  { key: 'latency', label: HANDSHAKE_LABEL, unit: 'ms' },
];

const RANGES: HistoryRange[] = ['1h', '6h', '24h', '7d'];

const REFRESH_MS = 15000;

const fmtTime = (iso: string, range: HistoryRange) => {
  const d = new Date(iso);
  if (range === '7d' || range === '24h') {
    return d.toLocaleString('pt-BR', { day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit' });
  }
  return d.toLocaleTimeString('pt-BR', { hour: '2-digit', minute: '2-digit' });
};

const MetricsHistoryView = () => {
  const [servers, setServers] = useState<ServerOption[]>([]);
  const [serverId, setServerId] = useState('');
  const [metric, setMetric] = useState<HistoryMetric>('cpu');
  const [range, setRange] = useState<HistoryRange>('1h');
  const [data, setData] = useState<ChartRow[]>([]);
  const [loading, setLoading] = useState(false);

  const activeMetric = METRICS.find((m) => m.key === metric) || METRICS[0];

  const fetchServers = useCallback(async () => {
    try {
      const data = await api.liveMetrics();
      const opts = data.servers.map(({ id, name }) => ({ id, name }));
      setServers(opts);
      setServerId((prev) => prev || (opts[0]?.id ?? ''));
    } catch (err) {
      console.error(err);
    }
  }, []);

  const fetchHistory = useCallback(async () => {
    if (!serverId) return;
    setLoading(true);
    try {
      const points = await api.history(serverId, metric, range);
      setData(points.map((p) => ({
        ts: p.ts,
        time: fmtTime(p.ts, range),
        value: Number(p.value.toFixed(2)),
      })));
    } catch (err) {
      console.error(err);
      setData([]);
    } finally {
      setLoading(false);
    }
  }, [serverId, metric, range]);

  useEffect(() => {
    fetchServers();
  }, [fetchServers]);

  useEffect(() => {
    fetchHistory();
    const interval = setInterval(fetchHistory, REFRESH_MS);
    return () => clearInterval(interval);
  }, [fetchHistory]);

  return (
    <div className="p-4 md:p-8 anim-rise">
      <div className="page-header">
        <div>
          <h1 className="page-title">Histórico de métricas</h1>
          <p className="page-desc">
            Séries temporais agregadas por servidor. Atualização automática a cada 15 segundos.
          </p>
        </div>
      </div>

      <div className="panel p-6">
        <div className="flex flex-wrap items-center gap-4 mb-6">
          <div className="flex flex-col gap-1.5">
            <label htmlFor="history-server" className="eyebrow">Servidor</label>
            <Select
              id="history-server"
              value={serverId}
              onChange={setServerId}
              className="min-w-[220px]"
              placeholder="Nenhum servidor"
              options={servers.map((s) => ({ value: s.id, label: s.name }))}
            />
          </div>

          <div className="flex flex-col gap-1.5">
            <label htmlFor="history-metric" className="eyebrow">Métrica</label>
            <Select
              id="history-metric"
              value={metric}
              onChange={(v) => setMetric(v as HistoryMetric)}
              className="min-w-[160px]"
              options={METRICS.map((m) => ({ value: m.key, label: m.label }))}
            />
          </div>

          <div className="flex flex-col gap-1.5">
            <label className="eyebrow">Período</label>
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
            onClick={fetchHistory}
            className="btn btn-ghost ml-auto self-end text-xs"
          >
            <RefreshCw size={14} strokeWidth={1.75} className={loading ? 'animate-spin' : ''} />
            Atualizar
          </button>
        </div>

        <div className="h-[420px] w-full">
          {loading && data.length === 0 ? (
            <div className="h-full flex items-center justify-center text-sm text-text-faint">Carregando...</div>
          ) : data.length === 0 ? (
            <div className="h-full flex items-center justify-center text-sm text-text-faint">
              Sem dados no período selecionado.
            </div>
          ) : (
            <ResponsiveContainer width="100%" height="100%">
              <AreaChart data={data} margin={{ top: 10, right: 20, left: 0, bottom: 0 }}>
                <defs>
                  <linearGradient id="metricFill" x1="0" y1="0" x2="0" y2="1">
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
                  width={48}
                  unit={activeMetric.unit}
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
                  formatter={(value) => [`${value}${activeMetric.unit}`, activeMetric.label]}
                />
                <Area
                  type="monotone"
                  dataKey="value"
                  stroke="var(--color-accent)"
                  strokeWidth={2}
                  fill="url(#metricFill)"
                  dot={false}
                  isAnimationActive={false}
                />
              </AreaChart>
            </ResponsiveContainer>
          )}
        </div>
      </div>
    </div>
  );
};

export default MetricsHistoryView;

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
import { Activity, RefreshCw } from 'lucide-react';
import { API_URL } from '../config';

interface ServerOption {
  id: string;
  name: string;
}

interface HistoryPoint {
  ts: string;
  value: number;
}

interface ChartRow {
  ts: string;
  time: string;
  value: number;
}

const METRICS = [
  { key: 'cpu', label: 'CPU', unit: '%' },
  { key: 'mem', label: 'Memória', unit: '%' },
  { key: 'disk', label: 'Disco', unit: '%' },
  { key: 'load', label: 'Load', unit: '' },
  { key: 'latency', label: 'Latência', unit: 'ms' },
];

const RANGES = ['1h', '6h', '24h', '7d'];

const fmtTime = (iso: string, range: string) => {
  const d = new Date(iso);
  if (range === '7d' || range === '24h') {
    return d.toLocaleString('pt-BR', { day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit' });
  }
  return d.toLocaleTimeString('pt-BR', { hour: '2-digit', minute: '2-digit' });
};

const MetricsHistoryView = () => {
  const [servers, setServers] = useState<ServerOption[]>([]);
  const [serverId, setServerId] = useState('');
  const [metric, setMetric] = useState('cpu');
  const [range, setRange] = useState('1h');
  const [data, setData] = useState<ChartRow[]>([]);
  const [loading, setLoading] = useState(false);

  const activeMetric = METRICS.find((m) => m.key === metric) || METRICS[0];

  const fetchServers = useCallback(async () => {
    try {
      const res = await fetch(API_URL + '/api/metrics/live');
      const json = await res.json();
      const opts: ServerOption[] = (json.servers || []).map((s: { id: string; name: string }) => ({
        id: s.id,
        name: s.name,
      }));
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
      const url = `${API_URL}/api/metrics/history?server_id=${serverId}&metric=${metric}&range=${range}`;
      const res = await fetch(url);
      const json: HistoryPoint[] = await res.json();
      const rows: ChartRow[] = (json || []).map((p) => ({
        ts: p.ts,
        time: fmtTime(p.ts, range),
        value: Number(p.value?.toFixed?.(2) ?? p.value),
      }));
      setData(rows);
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
    const interval = setInterval(fetchHistory, 15000);
    return () => clearInterval(interval);
  }, [fetchHistory]);

  const selectClass =
    'bg-black/40 border border-white/10 rounded-lg px-4 py-2 text-sm text-white focus:outline-none focus:border-[#10b981] transition-colors';

  return (
    <div className="p-8">
      <div className="flex items-center gap-3 mb-2">
        <Activity size={22} className="text-[#10b981]" />
        <h1 className="text-2xl font-light text-white">
          Histórico de <span className="font-bold">Métricas</span>
        </h1>
      </div>
      <p className="text-[#737373] text-sm mb-8">
        Séries temporais agregadas por servidor. Atualização automática a cada 15 segundos.
      </p>

      <div className="glass-panel p-6 rounded-xl border border-white/5 bg-white/[0.02]">
        <div className="flex flex-wrap items-center gap-4 mb-6">
          <div className="flex flex-col gap-1">
            <label className="text-[10px] text-[#737373] uppercase tracking-widest">Servidor</label>
            <select value={serverId} onChange={(e) => setServerId(e.target.value)} className={selectClass}>
              {servers.length === 0 && <option value="">Nenhum servidor</option>}
              {servers.map((s) => (
                <option key={s.id} value={s.id}>
                  {s.name}
                </option>
              ))}
            </select>
          </div>

          <div className="flex flex-col gap-1">
            <label className="text-[10px] text-[#737373] uppercase tracking-widest">Métrica</label>
            <select value={metric} onChange={(e) => setMetric(e.target.value)} className={selectClass}>
              {METRICS.map((m) => (
                <option key={m.key} value={m.key}>
                  {m.label}
                </option>
              ))}
            </select>
          </div>

          <div className="flex flex-col gap-1">
            <label className="text-[10px] text-[#737373] uppercase tracking-widest">Período</label>
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
            onClick={fetchHistory}
            className="ml-auto self-end flex items-center gap-2 text-xs text-[#737373] hover:text-[#10b981] transition-colors"
          >
            <RefreshCw size={14} className={loading ? 'animate-spin' : ''} />
            Atualizar
          </button>
        </div>

        <div className="h-[420px] w-full">
          {loading && data.length === 0 ? (
            <div className="h-full flex items-center justify-center text-sm text-[#737373]">Carregando...</div>
          ) : data.length === 0 ? (
            <div className="h-full flex items-center justify-center text-sm text-[#737373]">
              Sem dados no período selecionado.
            </div>
          ) : (
            <ResponsiveContainer width="100%" height="100%">
              <AreaChart data={data} margin={{ top: 10, right: 20, left: 0, bottom: 0 }}>
                <defs>
                  <linearGradient id="metricFill" x1="0" y1="0" x2="0" y2="1">
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
                  formatter={(value: number) => [`${value}${activeMetric.unit}`, activeMetric.label]}
                />
                <Area
                  type="monotone"
                  dataKey="value"
                  stroke="#10b981"
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

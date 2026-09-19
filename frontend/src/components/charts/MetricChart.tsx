import { useId } from 'react';
import {
  ResponsiveContainer, AreaChart, Area, XAxis, YAxis, CartesianGrid, Tooltip, ReferenceLine,
} from 'recharts';
import { AlertTriangle } from 'lucide-react';
import type { Annotation, HistoryMetric, HistoryPoint } from '../../lib/api';
import { formatDateTime } from '../../lib/format';
import { formatMetricValue, isRateMetric } from '../../lib/metrics';

interface MetricChartProps {
  points: HistoryPoint[];
  metric: HistoryMetric;
  label: string;
  annotations?: Annotation[];
  loading?: boolean;
  error?: string | null;
  emptyMessage?: string;
}

const DAY_MS = 24 * 60 * 60 * 1000;

const tickFormatter = (spanMs: number) => (ms: number) => {
  const d = new Date(ms);
  if (spanMs > 3 * DAY_MS) return d.toLocaleDateString('pt-BR', { day: '2-digit', month: '2-digit' });
  if (spanMs > DAY_MS) {
    return d.toLocaleString('pt-BR', { day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit' });
  }
  return d.toLocaleTimeString('pt-BR', { hour: '2-digit', minute: '2-digit' });
};

interface MarkerProps {
  viewBox?: { x?: number; y?: number };
  annotation: Annotation;
}

const AnnotationMarker = ({ viewBox, annotation }: MarkerProps) => {
  const x = viewBox?.x ?? 0;
  const y = viewBox?.y ?? 0;
  const who = annotation.server_id === null ? 'global' : annotation.author;
  return (
    <g className="cursor-help">
      <title>{`${annotation.text} (${who}, ${formatDateTime(annotation.at)})`}</title>
      <circle cx={x} cy={y + 5} r={4} fill="var(--color-info)" stroke="var(--color-ink-900)" strokeWidth={1.5} />
    </g>
  );
};

const MetricChart = ({
  points, metric, label, annotations = [], loading = false, error = null,
  emptyMessage = 'Sem dados no período selecionado.',
}: MetricChartProps) => {
  const gradientId = `fill-${useId().replace(/:/g, '')}`;

  if (error) {
    return (
      <div role="alert" className="h-full flex items-center justify-center gap-2 px-6 text-center text-sm text-crit">
        <AlertTriangle size={16} strokeWidth={1.75} />
        <span>{error}</span>
      </div>
    );
  }
  if (points.length === 0) {
    return (
      <div className="h-full flex items-center justify-center px-6 text-center text-sm text-text-faint">
        {loading ? 'Carregando...' : emptyMessage}
      </div>
    );
  }

  const data = points.map((p) => ({ t: new Date(p.ts).getTime(), value: Number(p.value.toFixed(2)) }));
  const spanMs = data[data.length - 1].t - data[0].t;
  const formatTick = tickFormatter(spanMs);

  return (
    <ResponsiveContainer width="100%" height="100%">
      <AreaChart data={data} margin={{ top: 14, right: 20, left: 0, bottom: 0 }}>
        <defs>
          <linearGradient id={gradientId} x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor="var(--color-accent)" stopOpacity={0.12} />
            <stop offset="100%" stopColor="var(--color-accent)" stopOpacity={0} />
          </linearGradient>
        </defs>
        <CartesianGrid strokeDasharray="3 3" stroke="var(--color-line)" strokeOpacity={0.5} vertical={false} />
        <XAxis
          dataKey="t"
          type="number"
          scale="time"
          domain={['dataMin', 'dataMax']}
          tickFormatter={formatTick}
          tick={{ fill: 'var(--color-text-faint)', fontSize: 11 }}
          tickLine={false}
          axisLine={false}
          minTickGap={30}
        />
        <YAxis
          tick={{ fill: 'var(--color-text-faint)', fontSize: 11 }}
          tickLine={false}
          axisLine={false}
          width={isRateMetric(metric) ? 72 : 48}
          tickFormatter={(v: number) => formatMetricValue(metric, v)}
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
          labelFormatter={(ms) => formatDateTime(new Date(Number(ms)).toISOString())}
          formatter={(value) => [formatMetricValue(metric, Number(value)), label]}
        />
        {annotations.map((a) => (
          <ReferenceLine
            key={a.id}
            x={new Date(a.at).getTime()}
            stroke="var(--color-info)"
            strokeDasharray="2 3"
            strokeOpacity={0.8}
            label={<AnnotationMarker annotation={a} />}
          />
        ))}
        <Area
          type="monotone"
          dataKey="value"
          stroke="var(--color-accent)"
          strokeWidth={2}
          fill={`url(#${gradientId})`}
          dot={false}
          isAnimationActive={false}
        />
      </AreaChart>
    </ResponsiveContainer>
  );
};

export default MetricChart;

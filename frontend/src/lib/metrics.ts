import type { DashboardMetric, HistoryMetric, HistoryRange, HistoryWindow } from './api';
import { formatLatency, formatRate } from './format';

export const NO_TEMPERATURE = 'sem sensor';
export const NO_TEMPERATURE_HINT =
  'Esta fonte de coleta não informa temperatura para esta máquina.';

export const NO_HANDSHAKE = 'não disponível';
export const NO_HANDSHAKE_HINT =
  'Só máquinas coletadas por SSH têm tempo de handshake. Estações com agente não abrem sessão.';

export const HANDSHAKE_LABEL = 'Handshake SSH';

export const NO_RTT_HINT =
  'Só servidores coletados por SSH têm latência medida pelo painel. Estações com agente não são sondadas.';

export const NO_NETWORK_HINT =
  'Esta fonte de coleta ainda não informa tráfego de rede para esta máquina.';

export const formatTemperature = (celsius: number | null): string =>
  celsius === null ? NO_TEMPERATURE : `${celsius.toFixed(0)}°C`;

export const formatHandshake = (ms: number | null): string =>
  ms === null ? NO_HANDSHAKE : `${ms.toFixed(0)} ms`;

export const isAbove = (value: number | null, threshold: number): boolean =>
  value !== null && value >= threshold;

export interface HistoryMetricDefinition {
  key: HistoryMetric;
  label: string;
  unit: string;
}

export const HISTORY_METRICS: HistoryMetricDefinition[] = [
  { key: 'cpu', label: 'CPU', unit: '%' },
  { key: 'mem', label: 'Memória', unit: '%' },
  { key: 'disk', label: 'Disco', unit: '%' },
  { key: 'load', label: 'Load', unit: '' },
  { key: 'temperature', label: 'Temperatura', unit: '°C' },
  { key: 'latency', label: HANDSHAKE_LABEL, unit: 'ms' },
  { key: 'net_rx', label: 'Rede RX', unit: 'B/s' },
  { key: 'net_tx', label: 'Rede TX', unit: 'B/s' },
  { key: 'rtt', label: 'Latência (ms)', unit: 'ms' },
];

const RATE_METRICS = new Set<string>(['net_rx', 'net_tx']);

export const isRateMetric = (metric: string): boolean => RATE_METRICS.has(metric);

export const formatMetricValue = (metric: string, value: number): string => {
  if (isRateMetric(metric)) return formatRate(value);
  if (metric === 'rtt') return formatLatency(value);
  const unit = HISTORY_METRICS.find((m) => m.key === metric)?.unit ?? '';
  return `${value}${unit}`;
};

export const RULE_METRIC_LABELS: Record<string, string> = {
  cpu: 'CPU (%)',
  mem: 'Memória (%)',
  disk: 'Disco (%)',
  load: 'Load',
  temperature: 'Temperatura (°C)',
  net_rx: 'Rede RX (bytes/s)',
  net_tx: 'Rede TX (bytes/s)',
  rtt: 'Latência (ms)',
};

export const formatRuleThreshold = (metric: string, threshold: number): string => {
  if (isRateMetric(metric)) return formatRate(threshold);
  if (metric === 'rtt') return `${threshold} ms`;
  return String(threshold);
};

export const HISTORY_RANGES: HistoryRange[] = ['1h', '6h', '24h', '7d', '30d', '90d'];

const HOUR_MS = 60 * 60 * 1000;

export const RANGE_MS: Record<HistoryRange, number> = {
  '1h': HOUR_MS,
  '6h': 6 * HOUR_MS,
  '24h': 24 * HOUR_MS,
  '7d': 7 * 24 * HOUR_MS,
  '30d': 30 * 24 * HOUR_MS,
  '90d': 90 * 24 * HOUR_MS,
};

export const windowBounds = (janela: HistoryWindow, now: number = Date.now()): { from: string; to: string } => {
  if (typeof janela === 'string') {
    return { from: new Date(now - RANGE_MS[janela]).toISOString(), to: new Date(now).toISOString() };
  }
  return { from: janela.from, to: janela.to ?? new Date(now).toISOString() };
};

export const DASHBOARD_METRICS = HISTORY_METRICS.filter(
  (m): m is HistoryMetricDefinition & { key: DashboardMetric } => m.key !== 'latency',
);

export const metricLabel = (metric: string): string =>
  HISTORY_METRICS.find((m) => m.key === metric)?.label ?? metric;

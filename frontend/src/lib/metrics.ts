import type { HistoryRange, HistoryWindow, MetricaDoCatalogo } from './api';
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

export const ESCOPO_CONTAINER = 'container';

const UNIDADE_TAXA = 'bytes/s';
const UNIDADE_TEMPO = 'ms';

export const ehTaxa = (unidade: string): boolean => unidade === UNIDADE_TAXA;

export const formatarPorUnidade = (unidade: string, value: number): string => {
  if (unidade === UNIDADE_TAXA) return formatRate(value);
  if (unidade === UNIDADE_TEMPO) return formatLatency(value);
  return `${value}${unidade}`;
};

export const formatarLimiar = (unidade: string, threshold: number): string => {
  if (unidade === UNIDADE_TAXA) return formatRate(threshold);
  if (unidade === UNIDADE_TEMPO) return `${threshold} ${UNIDADE_TEMPO}`;
  return String(threshold);
};

export const rotuloComUnidade = (metrica: MetricaDoCatalogo): string =>
  metrica.unidade ? `${metrica.rotulo} (${metrica.unidade})` : metrica.rotulo;

export const metricasDoHistorico = (catalogo: MetricaDoCatalogo[]): MetricaDoCatalogo[] =>
  catalogo.filter((m) => m.escopo !== ESCOPO_CONTAINER);

export const metricasDePainelERegra = (catalogo: MetricaDoCatalogo[]): MetricaDoCatalogo[] =>
  metricasDoHistorico(catalogo).filter((m) => m.em_regra);

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

const MAIOR_JANELA_SEM_TENDENCIA_MS = RANGE_MS['7d'];

export const AVISO_SEM_TENDENCIA =
  'Esta métrica não entra na tendência: o painel só guarda amostras brutas por 7 dias, então 30d e 90d não têm o que mostrar.';

export const janelaIndisponivel = (metrica: MetricaDoCatalogo | undefined, range: HistoryRange): boolean =>
  metrica !== undefined && !metrica.tem_tendencia && RANGE_MS[range] > MAIOR_JANELA_SEM_TENDENCIA_MS;

export const windowBounds = (janela: HistoryWindow, now: number = Date.now()): { from: string; to: string } => {
  if (typeof janela === 'string') {
    return { from: new Date(now - RANGE_MS[janela]).toISOString(), to: new Date(now).toISOString() };
  }
  return { from: janela.from, to: janela.to ?? new Date(now).toISOString() };
};

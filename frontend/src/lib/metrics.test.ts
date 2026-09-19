import { describe, expect, it } from 'vitest';
import {
  DASHBOARD_METRICS,
  HISTORY_METRICS,
  NO_HANDSHAKE,
  NO_TEMPERATURE,
  RULE_METRIC_LABELS,
  formatHandshake,
  formatMetricValue,
  formatRuleThreshold,
  formatTemperature,
  isAbove,
} from './metrics';

describe('formatTemperature', () => {
  it('distingue ausência de sensor de leitura zero', () => {
    expect(formatTemperature(null)).toBe(NO_TEMPERATURE);
    expect(formatTemperature(0)).toBe('0°C');
  });

  it('arredonda para grau inteiro', () => {
    expect(formatTemperature(71.6)).toBe('72°C');
  });

  it('preserva leitura negativa em vez de tratá-la como ausente', () => {
    expect(formatTemperature(-3)).toBe('-3°C');
  });
});

describe('formatHandshake', () => {
  it('distingue fonte sem SSH de handshake instantâneo', () => {
    expect(formatHandshake(null)).toBe(NO_HANDSHAKE);
    expect(formatHandshake(0)).toBe('0 ms');
  });

  it('mostra o valor em milissegundos inteiros', () => {
    expect(formatHandshake(1284.7)).toBe('1285 ms');
  });
});

describe('isAbove', () => {
  it('não conta ausência de leitura como abaixo do limiar', () => {
    expect(isAbove(null, 70)).toBe(false);
  });

  it('inclui o próprio limiar', () => {
    expect(isAbove(70, 70)).toBe(true);
    expect(isAbove(69.9, 70)).toBe(false);
  });
});

describe('métricas de histórico e de regra', () => {
  it('o histórico oferece Rede RX e Rede TX', () => {
    const rotulos = Object.fromEntries(HISTORY_METRICS.map((m) => [m.key, m.label]));
    expect(rotulos.net_rx).toBe('Rede RX');
    expect(rotulos.net_tx).toBe('Rede TX');
  });

  it('formata o valor conforme a métrica', () => {
    expect(formatMetricValue('net_rx', 1536)).toBe('1.5 KB/s');
    expect(formatMetricValue('net_tx', 0)).toBe('0 B/s');
    expect(formatMetricValue('cpu', 42.12)).toBe('42.12%');
    expect(formatMetricValue('temperature', 55)).toBe('55°C');
    expect(formatMetricValue('load', 1.5)).toBe('1.5');
  });

  it('as regras aceitam temperatura e taxa de rede', () => {
    expect(RULE_METRIC_LABELS.temperature).toBe('Temperatura (°C)');
    expect(RULE_METRIC_LABELS.net_rx).toBe('Rede RX (bytes/s)');
    expect(RULE_METRIC_LABELS.net_tx).toBe('Rede TX (bytes/s)');
    expect(RULE_METRIC_LABELS.cpu).toBe('CPU (%)');
  });

  it('limiar de taxa aparece em unidade legível', () => {
    expect(formatRuleThreshold('net_rx', 10 * 1024 * 1024)).toBe('10 MB/s');
    expect(formatRuleThreshold('cpu', 80)).toBe('80');
  });
});

describe('latência de rede (rtt)', () => {
  it('aparece no histórico, nos painéis e nas regras', () => {
    expect(HISTORY_METRICS.find((m) => m.key === 'rtt')?.label).toBe('Latência (ms)');
    expect(DASHBOARD_METRICS.some((m) => m.key === 'rtt')).toBe(true);
    expect(RULE_METRIC_LABELS.rtt).toBe('Latência (ms)');
  });

  it('formata valor e limiar em ms, e segundos acima de 1000 ms', () => {
    expect(formatMetricValue('rtt', 12)).toBe('12 ms');
    expect(formatMetricValue('rtt', 1200)).toBe('1,2 s');
    expect(formatRuleThreshold('rtt', 250)).toBe('250 ms');
  });
});

import { describe, expect, it } from 'vitest';

import { formatLatency, formatRate } from './format';

describe('formatRate', () => {
  it('ausência de medição aparece como travessão, nunca como zero', () => {
    expect(formatRate(null)).toBe('—');
  });

  it('zero medido continua zero', () => {
    expect(formatRate(0)).toBe('0 B/s');
  });

  it('escala em B/s, KB/s e MB/s', () => {
    expect(formatRate(512)).toBe('512 B/s');
    expect(formatRate(1536)).toBe('1.5 KB/s');
    expect(formatRate(5 * 1024 * 1024)).toBe('5 MB/s');
  });
});

describe('formatLatency', () => {
  it('ausência de medição aparece como travessão, nunca como zero', () => {
    expect(formatLatency(null)).toBe('—');
  });

  it('abaixo de um segundo fica em ms inteiros', () => {
    expect(formatLatency(0)).toBe('0 ms');
    expect(formatLatency(12.4)).toBe('12 ms');
    expect(formatLatency(999)).toBe('999 ms');
  });

  it('a partir de um segundo passa para segundos com vírgula', () => {
    expect(formatLatency(1000)).toBe('1,0 s');
    expect(formatLatency(1200)).toBe('1,2 s');
    expect(formatLatency(15480)).toBe('15,5 s');
  });
});

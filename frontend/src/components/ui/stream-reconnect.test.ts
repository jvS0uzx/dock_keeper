import { describe, expect, it } from 'vitest';

import { esperaDoBackoff } from './stream-reconnect';

describe('esperaDoBackoff', () => {
  const semSorteio = () => 0.5;

  it('dobra a cada tentativa até o teto de 30 s', () => {
    expect(esperaDoBackoff(0, semSorteio)).toBe(1000);
    expect(esperaDoBackoff(1, semSorteio)).toBe(2000);
    expect(esperaDoBackoff(2, semSorteio)).toBe(4000);
    expect(esperaDoBackoff(3, semSorteio)).toBe(8000);
    expect(esperaDoBackoff(4, semSorteio)).toBe(16000);
    expect(esperaDoBackoff(5, semSorteio)).toBe(30000);
    expect(esperaDoBackoff(20, semSorteio)).toBe(30000);
  });

  it('espalha com jitter de 20 % para os dois lados', () => {
    expect(esperaDoBackoff(0, () => 0)).toBe(800);
    expect(esperaDoBackoff(0, () => 1)).toBe(1200);
  });

  it('nunca devolve laço apertado', () => {
    for (let tentativa = 0; tentativa < 12; tentativa++) {
      for (const sorteio of [0, 0.5, 1]) {
        expect(esperaDoBackoff(tentativa, () => sorteio)).toBeGreaterThanOrEqual(800);
      }
    }
  });
});

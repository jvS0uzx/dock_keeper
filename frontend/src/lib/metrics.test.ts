import { describe, expect, it } from 'vitest';
import { NO_HANDSHAKE, NO_TEMPERATURE, formatHandshake, formatTemperature, isAbove } from './metrics';

// A regressão que estes testes travam: enquanto as colunas eram zero, o painel
// exibia "0 °C" e "0 ms" para máquina que a fonte de coleta nem mede. Zero
// medido e ausência de medição precisam ficar visualmente diferentes.
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

import { describe, expect, it } from 'vitest';

import {
  ALTURA_CARTAO,
  ALTURA_MAXIMA_MALHA,
  ALTURA_MINIMA_MALHA,
  PASSO_BALANCEADOR,
  PASSO_CARTAO,
  alturaDoConteudo,
  alturaVisivel,
  centroDaLinha,
} from './malhaLayout';

describe('layout da malha', () => {
  it('reserva passo maior que o cartão', () => {
    expect(PASSO_CARTAO).toBeGreaterThan(ALTURA_CARTAO);
  });

  it('nunca encolhe abaixo do piso', () => {
    expect(alturaDoConteudo(1, 1)).toBe(ALTURA_MINIMA_MALHA);
    expect(alturaDoConteudo(0, 0)).toBe(ALTURA_MINIMA_MALHA);
  });

  it('cresce com o número de receptores e de balanceadores', () => {
    expect(alturaDoConteudo(8, 1)).toBe(8 * PASSO_CARTAO);
    expect(alturaDoConteudo(1, 4)).toBe(4 * PASSO_BALANCEADOR);
    expect(alturaDoConteudo(9, 1)).toBeGreaterThan(alturaDoConteudo(8, 1));
  });

  it('passando do teto, a caixa para de crescer e o conteúdo continua', () => {
    const conteudo = alturaDoConteudo(20, 1);
    expect(conteudo).toBeGreaterThan(ALTURA_MAXIMA_MALHA);
    expect(alturaVisivel(conteudo)).toBe(ALTURA_MAXIMA_MALHA);
    expect(alturaVisivel(alturaDoConteudo(8, 1))).toBe(8 * PASSO_CARTAO);
  });

  it('distribui as linhas com o passo exato, sem sobreposição', () => {
    const total = 8;
    const altura = alturaDoConteudo(total, 1);
    const centros = Array.from({ length: total }, (_, i) => centroDaLinha(i, total, altura, PASSO_CARTAO));
    for (let i = 1; i < centros.length; i += 1) {
      expect(centros[i] - centros[i - 1]).toBe(PASSO_CARTAO);
    }
    expect(centros[0] - ALTURA_CARTAO / 2).toBeGreaterThanOrEqual(0);
    expect(centros[total - 1] + ALTURA_CARTAO / 2).toBeLessThanOrEqual(altura);
  });

  it('centra a coluna quando sobra espaço', () => {
    const altura = alturaDoConteudo(1, 1);
    expect(centroDaLinha(0, 1, altura, PASSO_CARTAO)).toBe(altura / 2);
  });
});

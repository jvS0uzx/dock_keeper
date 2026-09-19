import { describe, expect, it } from 'vitest';

import { compararNumeros, mediaDefinida } from './agregado';

describe('mediaDefinida', () => {
  it('ignora quem não tem medição e conta de quantos a média veio', () => {
    expect(mediaDefinida([40, null, 60])).toEqual({ media: 50, considerados: 2, total: 3 });
  });

  it('trata zero medido como medição, não como ausência', () => {
    expect(mediaDefinida([0, 100])).toEqual({ media: 50, considerados: 2, total: 2 });
  });

  it('devolve média nula quando ninguém mediu', () => {
    expect(mediaDefinida([null, null])).toEqual({ media: null, considerados: 0, total: 2 });
    expect(mediaDefinida([])).toEqual({ media: null, considerados: 0, total: 0 });
  });
});

describe('compararNumeros', () => {
  it('põe o nulo no fim nos dois sentidos', () => {
    const valores: (number | null)[] = [3, null, 1, null, 2];

    const crescente = [...valores].sort((a, b) => compararNumeros(a, b, 1));
    expect(crescente).toEqual([1, 2, 3, null, null]);

    const decrescente = [...valores].sort((a, b) => compararNumeros(a, b, -1));
    expect(decrescente).toEqual([3, 2, 1, null, null]);
  });

  it('mantém o zero medido antes do nulo', () => {
    const ordenado = [null, 0].sort((a, b) => compararNumeros(a, b, 1));
    expect(ordenado).toEqual([0, null]);
  });
});

export interface MediaDefinida {
  media: number | null;
  considerados: number;
  total: number;
}

export const mediaDefinida = (valores: (number | null | undefined)[]): MediaDefinida => {
  const medidos = valores.filter((v): v is number => typeof v === 'number' && Number.isFinite(v));
  if (medidos.length === 0) return { media: null, considerados: 0, total: valores.length };
  const soma = medidos.reduce((acc, v) => acc + v, 0);
  return { media: soma / medidos.length, considerados: medidos.length, total: valores.length };
};

export const compararNumeros = (a: number | null, b: number | null, sinal: 1 | -1): number => {
  if (a === null && b === null) return 0;
  if (a === null) return 1;
  if (b === null) return -1;
  return sinal * (a - b);
};

export const percentualDeUso = (usado: number, total: number): number | null =>
  total > 0 ? (usado / total) * 100 : null;

import { describe, expect, it } from 'vitest';

const fontes = import.meta.glob(['./**/*.{ts,tsx}', '!./**/*.test.{ts,tsx}', '!./test/**', '!./lib/polling.ts'], {
  query: '?raw',
  import: 'default',
  eager: true,
}) as Record<string, string>;

describe('intervalo de atualização mora em lib/polling.ts', () => {
  it('nenhum setInterval de tela usa literal de segundos', () => {
    const soltos = Object.entries(fontes).flatMap(([nome, texto]) =>
      [...texto.matchAll(/setInterval\([^;]*?,\s*(\d{4,})\s*\)/g)]
        .filter((m) => Number(m[1]) >= 2000)
        .map((m) => `${nome}: ${m[1]}`),
    );
    expect(soltos).toEqual([]);
  });

  it('nenhuma tela declara constante própria de polling', () => {
    const proprias = Object.entries(fontes).flatMap(([nome, texto]) =>
      [...texto.matchAll(/const (\w*(?:POLL|INTERVALO|REFRESH|RESUMO)\w*_MS) =/g)].map((m) => `${nome}: ${m[1]}`),
    );
    expect(proprias).toEqual([]);
  });
});

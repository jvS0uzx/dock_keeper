import { describe, expect, it } from 'vitest';

import { CATALOGO } from '../test/catalogo';
import {
  ehTaxa,
  formatarLimiar,
  formatarPorUnidade,
  janelaIndisponivel,
  metricasDePainelERegra,
  metricasDoHistorico,
  rotuloComUnidade,
} from './metrics';

type LeitorDeArquivo = { readFileSync(caminho: URL, codificacao: 'utf8'): string };

const fs = (
  globalThis as unknown as { process: { getBuiltinModule(id: 'node:fs'): LeitorDeArquivo } }
).process.getBuiltinModule('node:fs');

const ler = (caminho: string) => fs.readFileSync(new URL(caminho, import.meta.url), 'utf8');

const tagsJsonDoStruct = (fonte: string, nome: string): string[] => {
  const corpo = new RegExp(`type ${nome} struct \\{([\\s\\S]*?)\\n\\}`).exec(fonte);
  if (!corpo) throw new Error(`struct ${nome} não encontrado`);
  return [...corpo[1].matchAll(/json:"([a-z_]+)[",]/g)].map((m) => m[1]);
};

const camposDaInterface = (fonte: string, nome: string): string[] => {
  const corpo = new RegExp(`export interface ${nome} \\{([\\s\\S]*?)\\n\\}`).exec(fonte);
  if (!corpo) throw new Error(`interface ${nome} não encontrada`);
  return [...corpo[1].matchAll(/^\s+([a-z_]+)\??:/gm)].map((m) => m[1]);
};

const buscar = (nome: string) => {
  const metrica = CATALOGO.find((m) => m.nome === nome);
  if (!metrica) throw new Error(`${nome} fora da fixture`);
  return metrica;
};

describe('contrato de GET /api/metrics/catalogo', () => {
  const campos = Object.keys(CATALOGO[0]).sort();

  it('a fixture tem exatamente os campos que o backend serializa', () => {
    const doBackend = tagsJsonDoStruct(
      ler('../../../backend/internal/api/metrics_handler.go'),
      'metricaDoCatalogo',
    ).sort();
    expect(campos).toEqual(doBackend);
  });

  it('a fixture traz as mesmas métricas do registro do backend', () => {
    const registro = ler('../../../backend/internal/metricas/metricas.go');
    const nomes = [...registro.matchAll(/\bNome: "([a-z_]+)"/g)].map((m) => m[1]);
    expect(CATALOGO.map((m) => m.nome)).toEqual(nomes);
  });

  it('MetricaDoCatalogo declara exatamente os campos da fixture', () => {
    expect(camposDaInterface(ler('./api.ts'), 'MetricaDoCatalogo').sort()).toEqual(campos);
  });

  it('o frontend não guarda lista própria de métricas', () => {
    const fonte = ler('./metrics.ts') + ler('./api.ts');
    expect(fonte).not.toMatch(/HISTORY_METRICS|DASHBOARD_METRICS|RULE_METRIC_LABELS/);
    expect(fonte).not.toMatch(/'net_rx'|'latency'|'temperature'/);
  });
});

describe('tudo o que a tela mostra sai do catálogo', () => {
  it('histórico oferece o que tem série de servidor; painel e regra, o que o motor avalia', () => {
    expect(metricasDoHistorico(CATALOGO).map((m) => m.nome)).toContain('latency');
    expect(metricasDePainelERegra(CATALOGO).map((m) => m.nome)).not.toContain('latency');
    expect(metricasDePainelERegra(CATALOGO).map((m) => m.nome)).toContain('rtt');
    expect(metricasDoHistorico([{ ...buscar('cpu'), nome: 'so_container', escopo: 'container' }])).toEqual([]);
  });

  it('janela longa fica indisponível para quem não tem tendência, qualquer que seja o nome', () => {
    expect(janelaIndisponivel(buscar('latency'), '30d')).toBe(true);
    expect(janelaIndisponivel(buscar('latency'), '90d')).toBe(true);
    expect(janelaIndisponivel(buscar('latency'), '7d')).toBe(false);
    expect(janelaIndisponivel(buscar('cpu'), '90d')).toBe(false);
    expect(janelaIndisponivel({ ...buscar('cpu'), tem_tendencia: false }, '30d')).toBe(true);
    expect(janelaIndisponivel(undefined, '30d')).toBe(false);
  });

  it('formata pela unidade, não pelo nome', () => {
    expect(formatarPorUnidade('bytes/s', 1536)).toBe('1.5 KB/s');
    expect(formatarPorUnidade('ms', 1500)).toBe('1,5 s');
    expect(formatarPorUnidade('%', 42.12)).toBe('42.12%');
    expect(formatarPorUnidade('°C', 55)).toBe('55°C');
    expect(formatarPorUnidade('', 1.5)).toBe('1.5');
    expect(ehTaxa('bytes/s')).toBe(true);
    expect(ehTaxa('ms')).toBe(false);
  });

  it('rótulo de regra e limiar levam a unidade do catálogo', () => {
    expect(rotuloComUnidade(buscar('temperature'))).toBe('Temperatura (°C)');
    expect(rotuloComUnidade(buscar('load'))).toBe('Carga (1 min)');
    expect(formatarLimiar('bytes/s', 10 * 1024 * 1024)).toBe('10 MB/s');
    expect(formatarLimiar('ms', 200)).toBe('200 ms');
    expect(formatarLimiar('%', 80)).toBe('80');
  });
});

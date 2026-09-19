import { describe, expect, it } from 'vitest';

import {
  CAMINHOS,
  CAMINHO_LOGIN,
  abaDoCaminho,
  caminhoDaAba,
  caminhoDoDetalhe,
  detalheDoCaminho,
  normalizarCaminho,
  painelDaAba,
} from './rotas';
import { PANELS } from './panels';

describe('mapa de caminhos', () => {
  it('cobre toda aba dos dois painéis, sem caminho repetido', () => {
    const abas = new Set([...PANELS.dev.tabs, ...PANELS.suporte.tabs]);
    for (const aba of abas) {
      expect(CAMINHOS[aba], `aba ${aba} sem caminho`).toBeTruthy();
    }
    const caminhos = Object.values(CAMINHOS);
    expect(new Set(caminhos).size).toBe(caminhos.length);
  });

  it('vai da aba para o caminho e volta', () => {
    expect(caminhoDaAba('servers')).toBe('/servidores');
    expect(abaDoCaminho('/servidores')).toBe('servers');
    expect(abaDoCaminho('/regras')).toBe('alerts');
    expect(abaDoCaminho('/alertas')).toBe('alertas');
  });

  it('ignora barra final e caminho desconhecido', () => {
    expect(abaDoCaminho('/servidores/')).toBe('servers');
    expect(abaDoCaminho('/nao-existe')).toBeNull();
    expect(abaDoCaminho(CAMINHO_LOGIN)).toBeNull();
    expect(normalizarCaminho('/planta/')).toBe('/planta');
    expect(normalizarCaminho('/')).toBe('/');
  });
});

describe('detalhe na URL', () => {
  it('lê unidade e máquina do caminho', () => {
    expect(detalheDoCaminho('/unidades/7')).toEqual({ kind: 'site', id: 7 });
    expect(detalheDoCaminho('/maquinas/srv-1')).toEqual({ kind: 'machine', id: 'srv-1' });
  });

  it('a lista de unidades não é detalhe', () => {
    expect(detalheDoCaminho('/unidades')).toBeNull();
    expect(detalheDoCaminho('/unidades/abc')).toBeNull();
  });

  it('escreve o caminho do detalhe', () => {
    expect(caminhoDoDetalhe({ kind: 'site', id: 7 })).toBe('/unidades/7');
    expect(caminhoDoDetalhe({ kind: 'machine', id: 'srv-1' })).toBe('/maquinas/srv-1');
  });
});

describe('painel vem do caminho', () => {
  it('mantém o painel atual quando a aba pertence aos dois', () => {
    expect(painelDaAba('alertas', 'suporte')).toBe('suporte');
    expect(painelDaAba('alertas', 'dev')).toBe('dev');
  });

  it('troca de painel quando a aba só existe no outro', () => {
    expect(painelDaAba('stations', 'dev')).toBe('suporte');
    expect(painelDaAba('nginx', 'suporte')).toBe('dev');
  });
});

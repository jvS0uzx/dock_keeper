import { describe, expect, it } from 'vitest';
import { deriveUpstreams, splitUpstreams, totalRequests, upstreamHost } from './upstream';
import type { LbStat } from './api';

const stat = (upstream_addr: string, requests_count: number, server_name = 'app.exemplo'): LbStat => ({
  upstream_addr,
  server_name,
  status: '200',
  requests_count,
});

describe('splitUpstreams', () => {
  it('separa o retry que o Nginx registra na mesma linha', () => {
    expect(splitUpstreams('198.51.100.11:80, 198.51.100.12:80')).toEqual([
      '198.51.100.11:80',
      '198.51.100.12:80',
    ]);
  });

  it('descarta o marcador de atendimento local e o vazio', () => {
    expect(splitUpstreams('-')).toEqual([]);
    expect(splitUpstreams('')).toEqual([]);
  });
});

describe('upstreamHost', () => {
  it('remove a porta', () => {
    expect(upstreamHost('198.51.100.12:80')).toBe('198.51.100.12');
  });

  it('devolve o endereço inteiro quando não há porta', () => {
    expect(upstreamHost('backend-interno')).toBe('backend-interno');
  });
});

describe('deriveUpstreams', () => {
  // Regressão: os nós eram montados a partir de VITE_TARGET_VPS_IPS e casados
  // por substring com upstream_addr. Como o Nginx fala com os backends pela
  // rede Tailscale (100.x) e o .env tinha os IPs públicos (82.x), nada casava:
  // o diagrama mostrava os nós zerados, como se não houvesse tráfego.
  it('mostra o tráfego mesmo quando o .env não bate com os endereços reais', () => {
    const stats = [
      stat('198.51.100.11:80', 9),
      stat('198.51.100.12:80', 8),
    ];
    const ipsDesatualizadosNoEnv = ['203.0.113.25', '203.0.113.39'];

    const nodes = deriveUpstreams(stats, ipsDesatualizadosNoEnv);
    const comTrafego = nodes.filter((n) => n.reqs > 0);

    // O que o Nginx reportou aparece, mesmo fora da lista do .env.
    expect(comTrafego.map((n) => n.addr).sort()).toEqual([
      '198.51.100.11:80',
      '198.51.100.12:80',
    ]);
    expect(comTrafego.reduce((acc, n) => acc + n.reqs, 0)).toBe(17);
  });

  // Backend parado é informação: o nó fica no diagrama zerado em vez de sumir e
  // fazer o painel parecer vazio.
  it('mantém o backend conhecido no diagrama mesmo sem tráfego', () => {
    const nodes = deriveUpstreams([stat('10.0.0.1:80', 5)], ['10.0.0.1', '10.0.0.2']);

    expect(nodes.map((n) => n.addr)).toEqual(['10.0.0.1:80', '10.0.0.2:80']);
    expect(nodes[1].reqs).toBe(0);
  });

  it('aceita host com porta explícita no .env', () => {
    const nodes = deriveUpstreams([], ['10.0.0.1:8080']);
    expect(nodes.map((n) => n.addr)).toEqual(['10.0.0.1:8080']);
  });

  it('soma as requisições por endereço', () => {
    const nodes = deriveUpstreams([
      stat('10.0.0.1:80', 3),
      stat('10.0.0.1:80', 4, 'outro.exemplo'),
      stat('10.0.0.2:80', 1),
    ]);

    expect(nodes.find((n) => n.addr === '10.0.0.1:80')?.reqs).toBe(7);
    expect(nodes.find((n) => n.addr === '10.0.0.2:80')?.reqs).toBe(1);
  });

  it('conta o retry nos dois upstreams citados', () => {
    const nodes = deriveUpstreams([stat('10.0.0.1:80, 10.0.0.2:80', 2)]);

    expect(nodes).toHaveLength(2);
    expect(nodes.every((n) => n.reqs === 2)).toBe(true);
  });

  it('ignora as linhas atendidas localmente', () => {
    expect(deriveUpstreams([stat('-', 5)])).toEqual([]);
  });

  it('respeita a ordem preferida do .env quando os endereços batem', () => {
    const nodes = deriveUpstreams(
      [stat('10.0.0.2:80', 50), stat('10.0.0.1:80', 1)],
      ['10.0.0.1', '10.0.0.2'],
    );

    // 10.0.0.1 tem menos tráfego, mas vem primeiro por estar antes no .env.
    expect(nodes.map((n) => n.addr)).toEqual(['10.0.0.1:80', '10.0.0.2:80']);
  });

  it('ordena por tráfego quando o .env não ajuda', () => {
    const nodes = deriveUpstreams([stat('10.0.0.1:80', 1), stat('10.0.0.2:80', 50)], []);
    expect(nodes.map((n) => n.addr)).toEqual(['10.0.0.2:80', '10.0.0.1:80']);
  });

  // Reproduz a topologia do ambiente com endereços de documentação: o Nginx
  // fala com as VPS pela malha privada, e o mapeamento não segue o último
  // octeto do IP público — 198.51.100.12 é a VPS-1 (203.0.113.25), não a
  // VPS-2. É essa inversão que o teste protege.
  it('mantém Node 1 = VPS-1 quando o endereço da malha não casa com o público', () => {
    const nodes = deriveUpstreams(
      [stat('198.51.100.11:80', 20), stat('198.51.100.12:80', 3)],
      ['198.51.100.12', '198.51.100.11'],
    );

    expect(nodes.map((n) => n.addr)).toEqual(['198.51.100.12:80', '198.51.100.11:80']);
  });

  it('sem tráfego nenhum, mostra os conhecidos zerados', () => {
    expect(deriveUpstreams([], ['10.0.0.1'])).toEqual([
      { addr: '10.0.0.1:80', host: '10.0.0.1', reqs: 0 },
    ]);
  });

  it('sem tráfego e sem lista conhecida, não quebra', () => {
    expect(deriveUpstreams([], [])).toEqual([]);
  });
});

describe('totalRequests', () => {
  it('conta cada linha uma vez, inclusive as locais', () => {
    expect(totalRequests([stat('10.0.0.1:80', 9), stat('-', 3)])).toBe(12);
  });
});

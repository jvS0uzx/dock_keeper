import { describe, expect, it } from 'vitest';
import {
  arestasDaMalha,
  balanceadoresDaMalha,
  descobertaComPendencia,
  destinosDaMalha,
  ehPrincipal,
  ehReserva,
  estadoDoNginx,
  papelDoServidor,
  trafegoDaTopologia,
  type ServidorConhecido,
} from './upstream';
import type { LbStat, NginxUpstreamLink } from './api';

const servidor = (extra: Partial<ServidorConhecido> & { id: string; name: string }): ServidorConhecido => ({
  host_ip: '203.0.113.10',
  ...extra,
});

const principal = servidor({
  id: 'lb1',
  name: 'LB Principal',
  host_ip: '203.0.113.10',
  nginx_estado: 'candidato',
  nginx_papel: 'principal',
});

const reserva = servidor({
  id: 'lb2',
  name: 'LB Reserva',
  host_ip: '203.0.113.11',
  nginx_estado: 'candidato',
  nginx_papel: 'reserva',
});

const no1 = servidor({ id: 'n1', name: 'NODE 1', host_ip: '198.51.100.21' });
const no2 = servidor({ id: 'n2', name: 'NODE 2', host_ip: '198.51.100.22' });

const enlace = (server_id: string, destino: string, bloco = 'app'): NginxUpstreamLink => ({
  server_id,
  bloco,
  destino,
  observado_em: '2026-09-20T12:00:00Z',
});

const stat = (upstream_addr: string, requests_count: number, extra: Partial<LbStat> = {}): LbStat => ({
  upstream_addr,
  server_name: 'app.exemplo.com.br',
  status: '200',
  requests_count,
  ...extra,
});

describe('papel e estado do Nginx', () => {
  it('assume nenhum e desconhecido quando o backend ainda não respondeu', () => {
    const cru = servidor({ id: 'x', name: 'sem sonda' });
    expect(papelDoServidor(cru)).toBe('nenhum');
    expect(estadoDoNginx(cru)).toBe('desconhecido');
    expect(ehPrincipal(cru)).toBe(false);
    expect(ehReserva(cru)).toBe(false);
  });

  it('só acusa pendência quando a descoberta falhou e há motivo', () => {
    expect(descobertaComPendencia(principal)).toBe(false);
    expect(
      descobertaComPendencia(
        servidor({ id: 'y', name: 'sem permissão', nginx_estado: 'inativo', nginx_motivo: 'Nginx parado' }),
      ),
    ).toBe(true);
    expect(descobertaComPendencia(servidor({ id: 'z', name: 'sem nginx', nginx_estado: 'ausente' }))).toBe(false);
  });
});

describe('trafegoDaTopologia', () => {
  it('soma por destino e também por balanceador de origem', () => {
    const mapa = trafegoDaTopologia([
      stat('198.51.100.21:80', 5, { server_id: 'lb1' }),
      stat('198.51.100.21:80', 3, { server_id: 'lb2' }),
    ]);
    expect(mapa.get('lb1|198.51.100.21:80')).toBe(5);
    expect(mapa.get('lb2|198.51.100.21:80')).toBe(3);
    expect(mapa.get('|198.51.100.21:80')).toBe(8);
  });

  it('separa o retry que o Nginx registra na mesma linha', () => {
    const mapa = trafegoDaTopologia([stat('198.51.100.21:80, 198.51.100.22:80', 2, { server_id: 'lb1' })]);
    expect(mapa.get('lb1|198.51.100.21:80')).toBe(2);
    expect(mapa.get('lb1|198.51.100.22:80')).toBe(2);
  });
});

describe('arestasDaMalha', () => {
  it('desenha a aresta declarada mesmo sem nenhuma requisição', () => {
    const arestas = arestasDaMalha([enlace('lb1', '198.51.100.21:80')], [principal, no1], []);
    expect(arestas).toHaveLength(1);
    expect(arestas[0].reqs).toBe(0);
    expect(arestas[0].declarada).toBe(true);
    expect(arestas[0].potencial).toBe(false);
    expect(arestas[0].rotulo).toBe('NODE 1');
  });

  it('marca como potencial a aresta que sai da reserva', () => {
    const arestas = arestasDaMalha(
      [enlace('lb1', '198.51.100.21:80'), enlace('lb2', '198.51.100.21:80')],
      [principal, reserva, no1],
      [stat('198.51.100.21:80', 7, { server_id: 'lb1' })],
    );
    const daPrincipal = arestas.find((a) => a.balanceadorId === 'lb1');
    const daReserva = arestas.find((a) => a.balanceadorId === 'lb2');
    expect(daPrincipal?.potencial).toBe(false);
    expect(daPrincipal?.reqs).toBe(7);
    expect(daReserva?.potencial).toBe(true);
    expect(daReserva?.reqs).toBe(0);
  });

  it('credita ao principal o tráfego antigo que não declara o balanceador de origem', () => {
    const arestas = arestasDaMalha(
      [enlace('lb1', '198.51.100.21:80')],
      [principal, no1],
      [stat('198.51.100.21:80', 4)],
    );
    expect(arestas[0].reqs).toBe(4);
  });

  it('inclui o destino que só o tráfego revelou, sem topologia declarada', () => {
    const arestas = arestasDaMalha([], [principal, no2], [stat('198.51.100.22:80', 3, { server_id: 'lb1' })]);
    expect(arestas).toHaveLength(1);
    expect(arestas[0].declarada).toBe(false);
    expect(arestas[0].rotulo).toBe('NODE 2');
    expect(arestas[0].reqs).toBe(3);
  });

  it('não duplica destino que está na topologia e no tráfego', () => {
    const arestas = arestasDaMalha(
      [enlace('lb1', '198.51.100.21:80')],
      [principal, no1],
      [stat('198.51.100.21:80', 2, { server_id: 'lb1' })],
    );
    expect(arestas).toHaveLength(1);
    expect(arestas[0].declarada).toBe(true);
  });

  it('propaga a severidade do status para a aresta', () => {
    const arestas = arestasDaMalha(
      [enlace('lb1', '198.51.100.21:80')],
      [principal, no1],
      [stat('198.51.100.21:80', 1, { server_id: 'lb1', status: '502' })],
    );
    expect(arestas[0].severidade).toBe('error');
  });
});

describe('balanceadoresDaMalha e destinosDaMalha', () => {
  it('agrupa por balanceador e põe o principal antes da reserva', () => {
    const arestas = arestasDaMalha(
      [enlace('lb2', '198.51.100.21:80'), enlace('lb1', '198.51.100.21:80')],
      [reserva, principal, no1],
      [stat('198.51.100.21:80', 6, { server_id: 'lb1' })],
    );
    const grupos = balanceadoresDaMalha(arestas);
    expect(grupos.map((g) => g.id)).toEqual(['lb1', 'lb2']);
    expect(grupos[0].reqs).toBe(6);
    expect(grupos[1].potencial).toBe(true);
  });

  it('soma o mesmo destino visto por dois balanceadores num nó só', () => {
    const arestas = arestasDaMalha(
      [enlace('lb1', '198.51.100.21:80'), enlace('lb2', '198.51.100.21:80')],
      [principal, reserva, no1],
      [stat('198.51.100.21:80', 6, { server_id: 'lb1' })],
    );
    const destinos = destinosDaMalha(balanceadoresDaMalha(arestas));
    expect(destinos).toHaveLength(1);
    expect(destinos[0].reqs).toBe(6);
    expect(destinos[0].rotulo).toBe('NODE 1');
  });
});

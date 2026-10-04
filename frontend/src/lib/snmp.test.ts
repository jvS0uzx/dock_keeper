import { describe, expect, it } from 'vitest';

import { formatBps, formatContador, formatUptime, formatVelocidade } from './format';
import { marcaDaInterface, nomeDaInterface, temSNMP } from './snmp';

describe('formatBps', () => {
  it('ausência de medição aparece como travessão, nunca como zero', () => {
    expect(formatBps(null)).toBe('—');
    expect(formatBps(undefined)).toBe('—');
  });

  it('zero medido continua zero', () => {
    expect(formatBps(0)).toBe('0 bps');
  });

  it('escala em bits decimais: bps, kbps, Mbps e Gbps', () => {
    expect(formatBps(999)).toBe('999 bps');
    expect(formatBps(1234.5)).toBe('1,2 kbps');
    expect(formatBps(25_000_000)).toBe('25 Mbps');
    expect(formatBps(1_500_000_000)).toBe('1,5 Gbps');
  });
});

describe('formatVelocidade', () => {
  it('nulo vira travessão e 1000 Mbps vira 1 Gbps', () => {
    expect(formatVelocidade(null)).toBe('—');
    expect(formatVelocidade(100)).toBe('100 Mbps');
    expect(formatVelocidade(1000)).toBe('1 Gbps');
    expect(formatVelocidade(10000)).toBe('10 Gbps');
  });
});

describe('formatUptime', () => {
  it('nulo vira travessão e o resto fica legível', () => {
    expect(formatUptime(null)).toBe('—');
    expect(formatUptime(300)).toBe('5min');
    expect(formatUptime(3 * 3600 + 25 * 60)).toBe('3h 25min');
    expect(formatUptime(123456)).toBe('1d 10h');
  });
});

describe('formatContador', () => {
  it('nulo vira travessão e zero continua zero', () => {
    expect(formatContador(null)).toBe('—');
    expect(formatContador(0)).toBe('0');
  });
});

describe('marcaDaInterface', () => {
  it('admin down é desativada, distinta de down operacional', () => {
    expect(marcaDaInterface({ oper_status: 'down', admin_status: 'down' }).texto).toBe('Desativada');
    expect(marcaDaInterface({ oper_status: 'down', admin_status: 'up' })).toEqual({ texto: 'Down', classe: 'badge-crit' });
    expect(marcaDaInterface({ oper_status: 'up', admin_status: 'up' })).toEqual({ texto: 'Up', classe: 'badge-ok' });
  });
});

describe('nomeDaInterface', () => {
  it('cai para a descrição e depois para o ifIndex', () => {
    expect(nomeDaInterface({ if_name: 'ge-0/0/1', if_descr: 'x', if_index: 1 })).toBe('ge-0/0/1');
    expect(nomeDaInterface({ if_name: '', if_descr: 'Porta 1', if_index: 1 })).toBe('Porta 1');
    expect(nomeDaInterface({ if_name: '', if_descr: '', if_index: 7 })).toBe('ifIndex 7');
  });
});

describe('temSNMP', () => {
  it('só é verdadeiro quando o host já foi visto ou falhou no SNMP', () => {
    expect(temSNMP({ snmp_visto_em: null, snmp_erro: '' })).toBe(false);
    expect(temSNMP({ snmp_visto_em: '2026-10-03T15:00:00Z', snmp_erro: '' })).toBe(true);
    expect(temSNMP({ snmp_visto_em: null, snmp_erro: 'timeout' })).toBe(true);
  });
});

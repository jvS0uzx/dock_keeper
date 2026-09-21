import { describe, expect, it } from 'vitest';

import { classificarMalha, ehBalanceador, type ServidorConhecido } from './upstream';

const servidor = (nome: string, ip: string, extra: Partial<ServidorConhecido> = {}): ServidorConhecido => ({
  id: nome,
  name: nome,
  host_ip: ip,
  addresses: [ip],
  ...extra,
});

describe('quem é balanceador quando a descoberta ainda não chegou', () => {
  it('a marcação manual continua valendo', () => {
    expect(ehBalanceador(servidor('LB', '203.0.113.38', { collect_nginx: true }))).toBe(true);
  });

  it('o IP configurado no painel continua sendo o balanceador', () => {
    expect(ehBalanceador(servidor('LB', '203.0.113.38'), '203.0.113.38')).toBe(true);
    expect(ehBalanceador(servidor('VPS-1', '203.0.113.25'), '203.0.113.38')).toBe(false);
  });

  it('sem marcação e sem IP configurado, ninguém é balanceador', () => {
    expect(ehBalanceador(servidor('VPS-1', '203.0.113.25'))).toBe(false);
  });
});

describe('quem é balanceador depois que a descoberta responde', () => {
  it('candidato descoberto é balanceador mesmo sem marcação manual', () => {
    const achado = servidor('LB', '203.0.113.38', { nginx_estado: 'candidato', nginx_papel: 'principal' });
    expect(ehBalanceador(achado)).toBe(true);
  });

  it('reserva também é balanceador, porque assumiria o tráfego', () => {
    const reserva = servidor('LB-2', '203.0.113.37', { nginx_estado: 'candidato', nginx_papel: 'reserva' });
    expect(ehBalanceador(reserva)).toBe(true);
  });

  it('a descoberta manda no IP configurado: sem Nginx, não é balanceador', () => {
    const semNginx = servidor('LB', '203.0.113.38', { nginx_estado: 'ausente', nginx_papel: 'nenhum' });
    expect(ehBalanceador(semNginx, '203.0.113.38')).toBe(false);
  });

  it('a sobreposição manual vence a descoberta', () => {
    const forcado = servidor('LB', '203.0.113.38', {
      nginx_estado: 'ausente',
      nginx_papel: 'nenhum',
      collect_nginx: true,
    });
    expect(ehBalanceador(forcado)).toBe(true);
  });

  it('máquina que só serve site não vira balanceador', () => {
    const site = servidor('Site', '203.0.113.60', { nginx_estado: 'sem_upstream', nginx_papel: 'nenhum' });
    expect(ehBalanceador(site)).toBe(false);
  });
});

describe('grupos da malha com papel descoberto', () => {
  const principal = servidor('LB', '203.0.113.38', { nginx_estado: 'candidato', nginx_papel: 'principal' });
  const reserva = servidor('LB-2', '203.0.113.37', { nginx_estado: 'candidato', nginx_papel: 'reserva' });
  const atras = servidor('VPS-1', '203.0.113.25', {
    nginx_estado: 'ausente',
    nginx_papel: 'nenhum',
    behind_lb: true,
  });
  const fora = servidor('VPS E-mail', '203.0.113.50', { nginx_estado: 'ausente', nginx_papel: 'nenhum' });

  it('principal e reserva ficam os dois na coluna de balanceadores', () => {
    const grupos = classificarMalha([principal, reserva, atras, fora], []);

    expect(grupos.balanceadores.map((s) => s.name)).toEqual(['LB', 'LB-2']);
    expect(grupos.atras.map((s) => s.name)).toEqual(['VPS-1']);
    expect(grupos.fora.map((s) => s.name)).toEqual(['VPS E-mail']);
  });
});

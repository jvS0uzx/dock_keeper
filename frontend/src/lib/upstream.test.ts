import { describe, expect, it } from 'vitest';
import {
  carregarFiltroDaMalha,
  classificarMalha,
  deriveUpstreams,
  enderecosDoServidor,
  salvarFiltroDaMalha,
  rotuloDoUpstream,
  splitUpstreams,
  totalRequests,
  upstreamCadastrado,
  upstreamsSemCadastro,
  upstreamHost,
  indiceDeEnderecos,
  nosDaMalha,
} from './upstream';
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
  it('mostra o tráfego mesmo quando o .env não bate com os endereços reais', () => {
    const stats = [
      stat('198.51.100.11:80', 9),
      stat('198.51.100.12:80', 8),
    ];
    const ipsDesatualizadosNoEnv = ['203.0.113.25', '203.0.113.39'];

    const nodes = deriveUpstreams(stats, ipsDesatualizadosNoEnv);
    const comTrafego = nodes.filter((n) => n.reqs > 0);

    expect(comTrafego.map((n) => n.addr).sort()).toEqual([
      '198.51.100.11:80',
      '198.51.100.12:80',
    ]);
    expect(comTrafego.reduce((acc, n) => acc + n.reqs, 0)).toBe(17);
  });

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

    expect(nodes.map((n) => n.addr)).toEqual(['10.0.0.1:80', '10.0.0.2:80']);
  });

  it('ordena por tráfego quando o .env não ajuda', () => {
    const nodes = deriveUpstreams([stat('10.0.0.1:80', 1), stat('10.0.0.2:80', 50)], []);
    expect(nodes.map((n) => n.addr)).toEqual(['10.0.0.2:80', '10.0.0.1:80']);
  });

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

const servidor = (name: string, host_ip: string, collect_nginx = false, kind = 'ssh') => ({
  name,
  host_ip,
  collect_nginx,
  kind,
});

describe('rotuloDoUpstream', () => {
  const cadastrados = [servidor('vps-app', '10.0.0.1'), servidor('vps-banco', '10.0.0.2')];

  it('usa o nome cadastrado quando o endereço casa', () => {
    expect(rotuloDoUpstream({ addr: '10.0.0.2:80', host: '10.0.0.2', reqs: 4 }, cadastrados)).toBe('vps-banco');
  });

  it('não casa por último octeto: o caso real da rede overlay', () => {
    const reais = [servidor('VPS-2', '203.0.113.39'), servidor('VPS-1', '203.0.113.38')];
    const overlay = { addr: '100.100.0.10:80', host: '100.100.0.10', reqs: 5 };

    expect(rotuloDoUpstream(overlay, reais)).toBe('100.100.0.10:80');
    expect(upstreamCadastrado(overlay, reais)).toBe(false);
  });

  it('casa pelo endereço extra que o servidor declara', () => {
    const comOverlay = [
      { ...servidor('VPS-2', '203.0.113.39'), addresses: ['203.0.113.39', '100.100.0.10'] },
      servidor('VPS-1', '203.0.113.38'),
    ];
    const node = { addr: '100.100.0.10:80', host: '100.100.0.10', reqs: 5 };

    expect(rotuloDoUpstream(node, comOverlay)).toBe('VPS-2');
    expect(upstreamCadastrado(node, comOverlay)).toBe(true);
  });

  it('sem addresses no payload, cai no host_ip', () => {
    expect(enderecosDoServidor(servidor('vps-app', '10.0.0.1'))).toEqual(['10.0.0.1']);
    expect(enderecosDoServidor({ ...servidor('vps-app', '10.0.0.1'), addresses: ['10.0.0.1', '100.100.0.21'] }))
      .toEqual(['10.0.0.1', '100.100.0.21']);
  });

  it('devolve o endereço quando não há servidor cadastrado, nunca Node N', () => {
    const rotulo = rotuloDoUpstream({ addr: '198.51.100.9:80', host: '198.51.100.9', reqs: 0 }, cadastrados);
    expect(rotulo).toBe('198.51.100.9:80');
    expect(rotulo).not.toMatch(/Node/);
  });

  it('não adivinha quando dois cadastrados terminam no mesmo octeto', () => {
    const ambiguo = [servidor('vps-a', '10.0.0.7'), servidor('vps-b', '192.168.0.7')];
    expect(rotuloDoUpstream({ addr: '172.16.9.7:80', host: '172.16.9.7', reqs: 0 }, ambiguo)).toBe('172.16.9.7:80');
  });
});

describe('malha com endereço solto', () => {
  const cadastrados = [
    { ...servidor('VPS-1', '203.0.113.38'), addresses: ['203.0.113.38', '100.100.0.11'] },
    servidor('VPS-2', '203.0.113.39'),
  ];

  const nodes = [
    { addr: '100.100.0.11:80', host: '100.100.0.11', reqs: 9 },
    { addr: '100.100.0.10:80', host: '100.100.0.10', reqs: 4 },
  ];

  it('lista só os upstreams sem correspondência', () => {
    expect(upstreamsSemCadastro(nodes, cadastrados).map((n) => n.host)).toEqual(['100.100.0.10']);
  });
});

describe('grupos da malha, com os dados reais do dono', () => {
  const lb = { ...servidor('Load Balancer', '203.0.113.38', true), addresses: ['203.0.113.38'] };
  const vps1 = { ...servidor('VPS-1', '203.0.113.25'), addresses: ['203.0.113.25'], behind_lb: true };
  const vps2 = { ...servidor('VPS-2', '203.0.113.39'), addresses: ['203.0.113.39', '100.100.0.10'], behind_lb: true };
  const email = { ...servidor('VPS E-mail', '203.0.113.50'), addresses: ['203.0.113.50'] };
  const estacao = servidor('estacao-recepcao', '192.168.1.10', false, 'agent');
  const cadastrados = [lb, vps1, vps2, email, estacao];

  const nodes = [
    { addr: '203.0.113.25:80', host: '203.0.113.25', reqs: 12 },
    { addr: '100.100.0.10:80', host: '100.100.0.10', reqs: 7 },
    { addr: '100.100.0.11:80', host: '100.100.0.11', reqs: 3 },
  ];

  it('separa balanceador, quem está atrás dele e quem está fora', () => {
    const grupos = classificarMalha(cadastrados, nodes);

    expect(grupos.balanceadores.map((s) => s.name)).toEqual(['Load Balancer']);
    expect(grupos.atras.map((s) => s.name)).toEqual(['VPS-1', 'VPS-2']);
    expect(grupos.fora.map((s) => s.name)).toEqual(['VPS E-mail']);
  });

  it('estação com agente não entra na malha', () => {
    const grupos = classificarMalha(cadastrados, nodes);
    const todos = [...grupos.balanceadores, ...grupos.atras, ...grupos.fora].map((s) => s.name);
    expect(todos).not.toContain('estacao-recepcao');
  });

  it('o endereço solto não vira máquina de nenhum grupo', () => {
    const grupos = classificarMalha(cadastrados, nodes);
    expect(grupos.soltos.map((n) => n.host)).toEqual(['100.100.0.11']);
  });

  it('quem o backend marca fora do balanceador fica fora, com ou sem tráfego na janela', () => {
    const grupos = classificarMalha([{ ...vps1, behind_lb: false }, email], nodes);
    expect(grupos.atras).toEqual([]);
    expect(grupos.fora.map((s) => s.name)).toEqual(['VPS-1', 'VPS E-mail']);
  });
});

describe('preferência do filtro da malha', () => {
  it('guarda e lê a escolha, com padrão tudo', () => {
    localStorage.clear();
    expect(carregarFiltroDaMalha()).toBe('tudo');

    salvarFiltroDaMalha('fora');
    expect(carregarFiltroDaMalha()).toBe('fora');
  });

  it('valor corrompido volta ao padrão', () => {
    localStorage.setItem('dockkeeper.malha', 'qualquer-coisa');
    expect(carregarFiltroDaMalha()).toBe('tudo');
  });
});

describe('topologia desenhada mesmo sem tráfego', () => {
  const vps1 = { ...servidor('VPS-1', '203.0.113.25'), behind_lb: true, behind_lb_origem: 'trafego' };
  const email = { ...servidor('VPS E-mail', '203.0.113.50'), behind_lb: false, behind_lb_origem: 'nenhum' };
  const lb = servidor('Load Balancer', '203.0.113.38', true);

  it('marcação manual manda no grupo, mesmo com tráfego na janela', () => {
    const upstreams = [{ addr: '203.0.113.50:80', host: '203.0.113.50', reqs: 4 }];
    const grupos = classificarMalha([lb, vps1, email], upstreams);

    expect(grupos.fora.map((s) => s.name)).toEqual(['VPS E-mail']);
    expect(grupos.atras.map((s) => s.name)).toEqual(['VPS-1']);
  });
});

describe('nós da malha por máquina', () => {
  const servidores = [
    { id: 'lb', name: 'Load Balancer', host_ip: '203.0.113.38', addresses: ['203.0.113.38'], collect_nginx: true },
    { id: 'v1', name: 'VPS-1', host_ip: '203.0.113.25', addresses: ['203.0.113.25'], behind_lb: true },
    {
      id: 'v2',
      name: 'VPS-2',
      host_ip: '203.0.113.39',
      addresses: ['203.0.113.39', '100.100.0.11'],
      behind_lb: true,
    },
  ];

  const upstreams = [
    { addr: '203.0.113.25:80', host: '203.0.113.25', reqs: 68 },
    { addr: '203.0.113.39:80', host: '203.0.113.39', reqs: 35 },
    { addr: '100.100.0.11:80', host: '100.100.0.11', reqs: 11 },
  ];

  it('soma os endereços da mesma máquina num nó só', () => {
    const nos = nosDaMalha(servidores, upstreams);

    expect(nos).toHaveLength(2);
    const vps2 = nos.find((n) => n.rotulo === 'VPS-2');
    expect(vps2?.reqs).toBe(46);
    expect(vps2?.enderecos).toEqual(['203.0.113.39:80', '100.100.0.11:80']);
    expect(nos.filter((n) => n.rotulo === 'VPS-2')).toHaveLength(1);
  });

  it('endereço sem cadastro continua sendo um nó por endereço', () => {
    const nos = nosDaMalha(
      servidores.filter((s) => s.id !== 'v2'),
      upstreams,
    );

    const soltos = nos.filter((n) => !n.cadastrado);
    expect(soltos.map((n) => n.rotulo).sort()).toEqual(['100.100.0.11:80', '203.0.113.39:80']);
    expect(soltos.every((n) => n.enderecos.length === 1)).toBe(true);
  });

  it('máquina atrás do balanceador sem tráfego na janela continua no desenho', () => {
    const nos = nosDaMalha(servidores, [upstreams[0]]);

    const vps2 = nos.find((n) => n.rotulo === 'VPS-2');
    expect(vps2?.reqs).toBe(0);
    expect(vps2?.cadastrado).toBe(true);
  });

  it('mapeia cada endereço para o nó da máquina', () => {
    const nos = nosDaMalha(servidores, upstreams);
    const indice = indiceDeEnderecos(nos);

    expect(indice.get('203.0.113.39:80')).toBe(indice.get('100.100.0.11:80'));
    expect(indice.get('203.0.113.25:80')).not.toBe(indice.get('100.100.0.11:80'));
  });
});

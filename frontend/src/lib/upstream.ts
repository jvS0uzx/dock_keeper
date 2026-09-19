import type { LbStat } from './api';

export interface UpstreamNode {
  addr: string;
  host: string;
  reqs: number;
}

export const splitUpstreams = (raw: string): string[] =>
  raw
    .split(',')
    .map((part) => part.trim())
    .filter((part) => part !== '' && part !== '-');

export const upstreamHost = (addr: string): string => {
  const idx = addr.lastIndexOf(':');
  return idx === -1 ? addr : addr.slice(0, idx);
};

export const deriveUpstreams = (stats: LbStat[], knownHosts: string[] = []): UpstreamNode[] => {
  const byAddr = new Map<string, UpstreamNode>();

  for (const host of knownHosts) {
    if (host === '') continue;
    const addr = host.includes(':') ? host : `${host}:80`;
    byAddr.set(addr, { addr, host: upstreamHost(addr), reqs: 0 });
  }

  for (const stat of stats) {
    for (const addr of splitUpstreams(stat.upstream_addr)) {
      const node = byAddr.get(addr) ?? { addr, host: upstreamHost(addr), reqs: 0 };
      node.reqs += stat.requests_count;
      byAddr.set(addr, node);
    }
  }

  const preferredOrder = knownHosts;

  const rank = (node: UpstreamNode) => {
    const i = preferredOrder.findIndex((ip) => ip !== '' && node.host === ip);
    return i === -1 ? preferredOrder.length : i;
  };

  return [...byAddr.values()].sort(
    (a, b) => rank(a) - rank(b) || b.reqs - a.reqs || a.addr.localeCompare(b.addr),
  );
};

export const totalRequests = (stats: LbStat[]): number =>
  stats.reduce((acc, s) => acc + s.requests_count, 0);

export interface ServidorConhecido {
  id?: string;
  name: string;
  host_ip: string;
  addresses?: string[] | null;
  collect_nginx?: boolean;
  kind?: string;
  behind_lb?: boolean;
  behind_lb_origem?: string;
}

export const enderecosDoServidor = (servidor: ServidorConhecido): string[] => {
  const lista = [...(servidor.addresses ?? []), servidor.host_ip].filter((ip) => ip !== '');
  return [...new Set(lista)];
};

export const nomeDoUpstream = (host: string, servers: ServidorConhecido[]): string | null =>
  servers.find((s) => enderecosDoServidor(s).includes(host))?.name ?? null;

export const upstreamCadastrado = (node: UpstreamNode, servers: ServidorConhecido[]): boolean =>
  nomeDoUpstream(node.host, servers) !== null;

export const upstreamsSemCadastro = (
  nodes: UpstreamNode[],
  servers: ServidorConhecido[],
): UpstreamNode[] => nodes.filter((node) => !upstreamCadastrado(node, servers));

export const resumoDaMalha = (
  nodes: UpstreamNode[],
  servers: ServidorConhecido[],
): { conhecidas: number; soltos: number } => {
  const nomes = new Set(
    nodes.map((node) => nomeDoUpstream(node.host, servers)).filter((nome): nome is string => nome !== null),
  );
  return { conhecidas: nomes.size, soltos: upstreamsSemCadastro(nodes, servers).length };
};

export const rotuloDoUpstream = (node: UpstreamNode, servers: ServidorConhecido[]): string =>
  nomeDoUpstream(node.host, servers) ?? node.addr;

export const ehBalanceador = (servidor: ServidorConhecido, lbIp = ''): boolean =>
  servidor.collect_nginx === true || (lbIp !== '' && servidor.host_ip === lbIp);

export const hostsDeServidores = (servers: ServidorConhecido[], fallback: string[]): string[] => {
  const hosts = servers
    .filter((s) => s.kind !== 'agent' && !ehBalanceador(s))
    .map((s) => s.host_ip)
    .filter((ip) => ip !== '');
  return hosts.length > 0 ? hosts : fallback;
};

export type FiltroDaMalha = 'tudo' | 'atras' | 'fora';

const CHAVE_FILTRO = 'dockkeeper.malha';

const FILTROS: FiltroDaMalha[] = ['tudo', 'atras', 'fora'];

export const carregarFiltroDaMalha = (): FiltroDaMalha => {
  try {
    const salvo = localStorage.getItem(CHAVE_FILTRO);
    return FILTROS.find((f) => f === salvo) ?? 'tudo';
  } catch {
    return 'tudo';
  }
};

export const salvarFiltroDaMalha = (filtro: FiltroDaMalha) => {
  try {
    localStorage.setItem(CHAVE_FILTRO, filtro);
  } catch {
  }
};

export interface GruposDaMalha<T extends ServidorConhecido> {
  balanceadores: T[];
  atras: T[];
  fora: T[];
  soltos: UpstreamNode[];
}

export const estaAtrasDoBalanceador = (
  servidor: ServidorConhecido,
  hostsComTrafego: Set<string>,
): boolean =>
  typeof servidor.behind_lb === 'boolean'
    ? servidor.behind_lb
    : enderecosDoServidor(servidor).some((ip) => hostsComTrafego.has(ip));

export const classificarMalha = <T extends ServidorConhecido>(
  servers: T[],
  nodes: UpstreamNode[],
): GruposDaMalha<T> => {
  const hosts = new Set(nodes.map((n) => n.host));
  const naMalha = servers.filter((s) => s.kind !== 'agent');

  const balanceadores = naMalha.filter((s) => ehBalanceador(s));
  const demais = naMalha.filter((s) => !ehBalanceador(s));
  const atras = demais.filter((s) => estaAtrasDoBalanceador(s, hosts));
  const fora = demais.filter((s) => !estaAtrasDoBalanceador(s, hosts));

  return { balanceadores, atras, fora, soltos: upstreamsSemCadastro(nodes, servers) };
};

export const nosDaTopologia = (
  servers: ServidorConhecido[],
  upstreams: UpstreamNode[],
): UpstreamNode[] => {
  const hosts = new Set(upstreams.map((n) => n.host));
  const { atras } = classificarMalha(servers, upstreams);
  const semTrafego = atras
    .filter((s) => !enderecosDoServidor(s).some((ip) => hosts.has(ip)))
    .map((s) => ({ addr: `${s.host_ip}:80`, host: s.host_ip, reqs: 0 }));

  return [...upstreams, ...semTrafego];
};

export interface NoDaMalha {
  id: string;
  rotulo: string;
  host: string;
  enderecos: string[];
  reqs: number;
  cadastrado: boolean;
}

const chaveDoServidor = (servidor: ServidorConhecido): string => `srv:${servidor.id ?? servidor.name}`;

export const nosDaMalha = (servers: ServidorConhecido[], upstreams: UpstreamNode[]): NoDaMalha[] => {
  const nos = new Map<string, NoDaMalha>();

  for (const node of upstreams) {
    const servidor = servers.find((s) => enderecosDoServidor(s).includes(node.host));
    const id = servidor ? chaveDoServidor(servidor) : `addr:${node.addr}`;
    const atual = nos.get(id) ?? {
      id,
      rotulo: servidor?.name ?? node.addr,
      host: node.host,
      enderecos: [],
      reqs: 0,
      cadastrado: servidor !== undefined,
    };
    atual.enderecos.push(node.addr);
    atual.reqs += node.reqs;
    nos.set(id, atual);
  }

  for (const servidor of classificarMalha(servers, upstreams).atras) {
    const id = chaveDoServidor(servidor);
    if (nos.has(id)) continue;
    nos.set(id, {
      id,
      rotulo: servidor.name,
      host: servidor.host_ip,
      enderecos: [`${servidor.host_ip}:80`],
      reqs: 0,
      cadastrado: true,
    });
  }

  return [...nos.values()].sort((a, b) => b.reqs - a.reqs || a.rotulo.localeCompare(b.rotulo));
};

export const indiceDeEnderecos = (nos: NoDaMalha[]): Map<string, string> => {
  const mapa = new Map<string, string>();
  for (const no of nos) {
    for (const addr of no.enderecos) mapa.set(addr, no.id);
  }
  return mapa;
};

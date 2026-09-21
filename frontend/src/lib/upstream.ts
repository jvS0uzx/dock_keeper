import type { LbStat, NginxEstado, NginxPapel, NginxUpstreamLink } from './api';

export interface UpstreamNode {
  addr: string;
  host: string;
  reqs: number;
}

export const JANELA_NAO_INFORMADA = 'janela não informada';

export const rotuloDaJanela = (segundos: number | null | undefined): string =>
  typeof segundos === 'number' && segundos > 0 ? `${segundos}s` : JANELA_NAO_INFORMADA;

export const UPSTREAM_LOCAL = 'Local (Nginx/Cache)';

export const splitUpstreams = (raw: string): string[] =>
  raw
    .split(',')
    .map((part) => part.trim())
    .filter((part) => part !== '' && part !== '-' && part !== UPSTREAM_LOCAL);

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
  nginx_estado?: NginxEstado;
  nginx_motivo?: string;
  nginx_papel?: NginxPapel;
  nginx_checado_em?: string | null;
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

export const rotuloDoUpstream = (node: UpstreamNode, servers: ServidorConhecido[]): string =>
  nomeDoUpstream(node.host, servers) ?? node.addr;

const descobertaDisponivel = (servidor: ServidorConhecido): boolean =>
  servidor.nginx_papel !== undefined || servidor.nginx_estado !== undefined;

export const ehBalanceador = (servidor: ServidorConhecido, lbIp = ''): boolean => {
  if (servidor.collect_nginx === true) return true;
  if (descobertaDisponivel(servidor)) {
    return papelDoServidor(servidor) !== 'nenhum' || estadoDoNginx(servidor) === 'candidato';
  }
  return lbIp !== '' && servidor.host_ip === lbIp;
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

export const estaAtrasDoBalanceador = (servidor: ServidorConhecido): boolean => servidor.behind_lb === true;

export const classificarMalha = <T extends ServidorConhecido>(
  servers: T[],
  nodes: UpstreamNode[],
  lbIp = '',
): GruposDaMalha<T> => {
  const naMalha = servers.filter((s) => s.kind !== 'agent');

  const balanceadores = naMalha.filter((s) => ehBalanceador(s, lbIp));
  const demais = naMalha.filter((s) => !ehBalanceador(s, lbIp));
  const atras = demais.filter((s) => estaAtrasDoBalanceador(s));
  const fora = demais.filter((s) => !estaAtrasDoBalanceador(s));

  return { balanceadores, atras, fora, soltos: upstreamsSemCadastro(nodes, servers) };
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

export const nosDaMalha = (
  servers: ServidorConhecido[],
  upstreams: UpstreamNode[],
  lbIp = '',
): NoDaMalha[] => {
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

  for (const servidor of classificarMalha(servers, upstreams, lbIp).atras) {
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

export const papelDoServidor = (servidor: ServidorConhecido): NginxPapel =>
  servidor.nginx_papel ?? 'nenhum';

export const ehPrincipal = (servidor: ServidorConhecido): boolean =>
  papelDoServidor(servidor) === 'principal';

export const ehReserva = (servidor: ServidorConhecido): boolean =>
  papelDoServidor(servidor) === 'reserva';

export const estadoDoNginx = (servidor: ServidorConhecido): NginxEstado =>
  servidor.nginx_estado ?? 'desconhecido';

export const descobertaComPendencia = (servidor: ServidorConhecido): boolean =>
  estadoDoNginx(servidor) !== 'candidato' && (servidor.nginx_motivo ?? '') !== '';

const SEM_DONO = '';

export const chaveDeTrafego = (balanceadorId: string, destino: string): string =>
  `${balanceadorId}|${destino}`;

export const trafegoDaTopologia = (stats: LbStat[]): Map<string, number> => {
  const mapa = new Map<string, number>();
  const somar = (chave: string, quanto: number) => mapa.set(chave, (mapa.get(chave) ?? 0) + quanto);

  for (const stat of stats) {
    const dono = stat.server_id ?? SEM_DONO;
    for (const destino of splitUpstreams(stat.upstream_addr)) {
      somar(chaveDeTrafego(SEM_DONO, destino), stat.requests_count);
      if (dono !== SEM_DONO) somar(chaveDeTrafego(dono, destino), stat.requests_count);
    }
  }
  return mapa;
};

export type SeveridadeDoDestino = 'ok' | 'warn' | 'error';

const STATUS_DE_ERRO = ['500', '502', '503', '504'];
const STATUS_DE_AVISO = ['400', '404', '429'];

export const severidadePorDestino = (stats: LbStat[]): Map<string, SeveridadeDoDestino> => {
  const mapa = new Map<string, SeveridadeDoDestino>();
  for (const stat of stats) {
    for (const destino of splitUpstreams(stat.upstream_addr)) {
      const atual = mapa.get(destino) ?? 'ok';
      if (STATUS_DE_ERRO.includes(stat.status)) mapa.set(destino, 'error');
      else if (atual !== 'error' && STATUS_DE_AVISO.includes(stat.status)) mapa.set(destino, 'warn');
      else if (!mapa.has(destino)) mapa.set(destino, 'ok');
    }
  }
  return mapa;
};

export interface ArestaDaMalha {
  balanceadorId: string;
  balanceadorNome: string;
  bloco: string;
  destino: string;
  host: string;
  rotulo: string;
  reqs: number;
  potencial: boolean;
  declarada: boolean;
  severidade: SeveridadeDoDestino;
}

const BLOCO_DO_TRAFEGO = 'tráfego observado';

export const arestasDaMalha = <T extends ServidorConhecido>(
  topologia: NginxUpstreamLink[],
  servers: T[],
  stats: LbStat[],
): ArestaDaMalha[] => {
  const trafego = trafegoDaTopologia(stats);
  const severidades = severidadePorDestino(stats);
  const principal = servers.find(ehPrincipal);
  const arestas = new Map<string, ArestaDaMalha>();

  const reqsDe = (balanceadorId: string, destino: string): number => {
    const proprio = trafego.get(chaveDeTrafego(balanceadorId, destino));
    if (proprio !== undefined) return proprio;
    if (principal !== undefined && principal.id === balanceadorId) {
      return trafego.get(chaveDeTrafego(SEM_DONO, destino)) ?? 0;
    }
    return 0;
  };

  const registrar = (aresta: ArestaDaMalha) => {
    const chave = `${aresta.balanceadorId}|${aresta.bloco}|${aresta.destino}`;
    const anterior = arestas.get(chave);
    if (anterior === undefined || (!anterior.declarada && aresta.declarada)) arestas.set(chave, aresta);
  };

  for (const enlace of topologia) {
    const dono = servers.find((s) => s.id === enlace.server_id);
    registrar({
      balanceadorId: enlace.server_id,
      balanceadorNome: dono?.name ?? enlace.server_id,
      bloco: enlace.bloco,
      destino: enlace.destino,
      host: upstreamHost(enlace.destino),
      rotulo: nomeDoUpstream(upstreamHost(enlace.destino), servers) ?? enlace.destino,
      reqs: reqsDe(enlace.server_id, enlace.destino),
      potencial: dono !== undefined && ehReserva(dono),
      declarada: true,
      severidade: severidades.get(enlace.destino) ?? 'ok',
    });
  }

  for (const stat of stats) {
    const donoId = stat.server_id ?? (principal?.id ?? SEM_DONO);
    const dono = servers.find((s) => s.id === donoId);
    for (const destino of splitUpstreams(stat.upstream_addr)) {
      const jaDeclarado = topologia.some(
        (e) => e.destino === destino && (e.server_id === donoId || donoId === SEM_DONO),
      );
      if (jaDeclarado) continue;
      registrar({
        balanceadorId: donoId,
        balanceadorNome: dono?.name ?? 'Balanceador',
        bloco: BLOCO_DO_TRAFEGO,
        destino,
        host: upstreamHost(destino),
        rotulo: nomeDoUpstream(upstreamHost(destino), servers) ?? destino,
        reqs: reqsDe(donoId, destino),
        potencial: dono !== undefined && ehReserva(dono),
        declarada: false,
        severidade: severidades.get(destino) ?? 'ok',
      });
    }
  }

  return [...arestas.values()].sort(
    (a, b) =>
      Number(a.potencial) - Number(b.potencial) ||
      b.reqs - a.reqs ||
      a.destino.localeCompare(b.destino),
  );
};

export interface BalanceadorDaMalha {
  id: string;
  nome: string;
  potencial: boolean;
  reqs: number;
  arestas: ArestaDaMalha[];
}

export const balanceadoresDaMalha = (arestas: ArestaDaMalha[]): BalanceadorDaMalha[] => {
  const mapa = new Map<string, BalanceadorDaMalha>();
  for (const aresta of arestas) {
    const atual = mapa.get(aresta.balanceadorId) ?? {
      id: aresta.balanceadorId,
      nome: aresta.balanceadorNome,
      potencial: aresta.potencial,
      reqs: 0,
      arestas: [],
    };
    atual.reqs += aresta.reqs;
    atual.arestas.push(aresta);
    mapa.set(aresta.balanceadorId, atual);
  }
  return [...mapa.values()].sort(
    (a, b) => Number(a.potencial) - Number(b.potencial) || b.reqs - a.reqs || a.nome.localeCompare(b.nome),
  );
};

export interface DestinoDaMalha {
  destino: string;
  rotulo: string;
  reqs: number;
  severidade: SeveridadeDoDestino;
}

export const destinosDaMalha = (balanceadores: BalanceadorDaMalha[]): DestinoDaMalha[] => {
  const mapa = new Map<string, DestinoDaMalha>();
  for (const balanceador of balanceadores) {
    for (const aresta of balanceador.arestas) {
      const atual = mapa.get(aresta.destino) ?? {
        destino: aresta.destino,
        rotulo: aresta.rotulo,
        reqs: 0,
        severidade: 'ok' as SeveridadeDoDestino,
      };
      atual.reqs += aresta.reqs;
      if (aresta.severidade === 'error') atual.severidade = 'error';
      else if (aresta.severidade === 'warn' && atual.severidade !== 'error') atual.severidade = 'warn';
      mapa.set(aresta.destino, atual);
    }
  }
  return [...mapa.values()].sort((a, b) => b.reqs - a.reqs || a.destino.localeCompare(b.destino));
};

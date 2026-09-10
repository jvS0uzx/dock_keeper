import type { LbStat } from './api';

/** Rótulo de quem foi atendido pelo próprio Nginx, sem ir a um upstream. */
export const LOCAL_LABEL = 'Local (Nginx/Cache)';

export interface UpstreamNode {
  /** Endereço como o Nginx reporta, ex: "198.51.100.12:80". */
  addr: string;
  /** Só o host, sem porta — é o que casa com um IP configurado. */
  host: string;
  reqs: number;
}

/**
 * Separa o campo upstream_addr do Nginx.
 *
 * Quando o Nginx tenta um upstream, falha e refaz em outro, ele registra os
 * dois separados por vírgula na mesma linha: "10.0.0.1:80, 10.0.0.2:80".
 * Tratar isso como um endereço único criaria um nó fantasma que não existe.
 */
export const splitUpstreams = (raw: string): string[] =>
  raw
    .split(',')
    .map((part) => part.trim())
    .filter((part) => part !== '' && part !== '-');

/** Tira a porta: "198.51.100.12:80" vira "198.51.100.12". */
export const upstreamHost = (addr: string): string => {
  const idx = addr.lastIndexOf(':');
  return idx === -1 ? addr : addr.slice(0, idx);
};

/**
 * Monta os nós do balanceador: os backends conhecidos mais os observados.
 *
 * São duas fontes de propósito.
 *
 * Os de `knownHosts` (VITE_TARGET_VPS_IPS) ficam **sempre** no diagrama, mesmo
 * com zero requisição. Um backend que parou de receber tráfego é justamente a
 * informação que o operador precisa ver; sumir com o nó faria o painel parecer
 * vazio quando a rede está parada.
 *
 * Os observados no `upstream_addr` entram mesmo sem estar na lista, para que
 * uma troca de topologia (Tailscale, VPN, faixa nova) não esconda tráfego real
 * só porque o .env ficou para trás.
 */
export const deriveUpstreams = (stats: LbStat[], knownHosts: string[] = []): UpstreamNode[] => {
  const byAddr = new Map<string, UpstreamNode>();

  // Backends conhecidos entram primeiro, zerados: a porta padrão 80 é o que o
  // Nginx reporta quando o upstream não declara outra.
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

/** Total de requisições da janela, contando cada linha uma vez. */
export const totalRequests = (stats: LbStat[]): number =>
  stats.reduce((acc, s) => acc + s.requests_count, 0);

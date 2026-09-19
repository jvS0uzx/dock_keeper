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

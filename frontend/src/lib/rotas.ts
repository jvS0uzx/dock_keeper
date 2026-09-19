import { useCallback, useEffect, useState } from 'react';

import { PANELS, PANEL_IDS, type PanelId } from './panels';

export type Detalhe = { kind: 'site'; id: number } | { kind: 'machine'; id: string };

export const CAMINHO_LOGIN = '/login';

export const CAMINHOS: Record<string, string> = {
  dashboard: '/dashboard',
  history: '/historico',
  dashboards: '/paineis',
  containers: '/containers',
  nginx: '/nginx',
  ssl: '/ssl',
  security: '/seguranca',
  logs: '/logs',
  alertas: '/alertas',
  alerts: '/regras',
  servers: '/servidores',
  users: '/usuarios',
  audit: '/auditoria',
  stations: '/estacoes',
  network: '/inventario',
  floorplan: '/planta',
  sites: '/unidades',
  devices: '/dispositivos',
};

const ABA_POR_CAMINHO: Record<string, string> = Object.fromEntries(
  Object.entries(CAMINHOS).map(([aba, caminho]) => [caminho, aba]),
);

export const normalizarCaminho = (caminho: string): string => {
  const limpo = caminho.split('?')[0].split('#')[0];
  if (limpo === '' || limpo === '/') return '/';
  return limpo.endsWith('/') ? limpo.slice(0, -1) : limpo;
};

export const caminhoDaAba = (aba: string): string => CAMINHOS[aba] ?? CAMINHOS.dashboard;

export const abaDoCaminho = (caminho: string): string | null =>
  ABA_POR_CAMINHO[normalizarCaminho(caminho)] ?? null;

export const detalheDoCaminho = (caminho: string): Detalhe | null => {
  const partes = normalizarCaminho(caminho).split('/').filter(Boolean);
  if (partes.length !== 2) return null;

  const [raiz, id] = partes;
  if (raiz === 'unidades') {
    return /^\d+$/.test(id) ? { kind: 'site', id: Number(id) } : null;
  }
  if (raiz === 'maquinas' && id !== '') {
    return { kind: 'machine', id: decodeURIComponent(id) };
  }
  return null;
};

export const caminhoDoDetalhe = (detalhe: Detalhe): string =>
  detalhe.kind === 'site'
    ? `/unidades/${detalhe.id}`
    : `/maquinas/${encodeURIComponent(detalhe.id)}`;

export const painelDaAba = (aba: string, atual: PanelId): PanelId => {
  if (PANELS[atual].tabs.includes(aba)) return atual;
  return PANEL_IDS.find((id) => PANELS[id].tabs.includes(aba)) ?? atual;
};

export const useRota = () => {
  const [caminho, setCaminho] = useState(() => normalizarCaminho(window.location.pathname));

  useEffect(() => {
    const aoVoltar = () => setCaminho(normalizarCaminho(window.location.pathname));
    window.addEventListener('popstate', aoVoltar);
    return () => window.removeEventListener('popstate', aoVoltar);
  }, []);

  const navegar = useCallback((destino: string, opcoes: { substituir?: boolean } = {}) => {
    const alvo = normalizarCaminho(destino);
    if (alvo === normalizarCaminho(window.location.pathname)) {
      setCaminho(alvo);
      return;
    }
    if (opcoes.substituir) window.history.replaceState({}, '', alvo);
    else window.history.pushState({}, '', alvo);
    setCaminho(alvo);
  }, []);

  return { caminho, navegar };
};

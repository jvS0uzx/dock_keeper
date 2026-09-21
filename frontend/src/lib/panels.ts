import type { Role, SiteAccess } from './session';

export type PanelId = 'dev' | 'suporte';

export interface PanelDefinition {
  id: PanelId;
  label: string;
  description: string;
  tabs: string[];
}

export const PANELS: Record<PanelId, PanelDefinition> = {
  dev: {
    id: 'dev',
    label: 'Infra / Dev',
    description: 'VPS, containers e serviços',
    tabs: ['dashboard', 'history', 'dashboards', 'containers', 'nginx', 'ssl', 'security', 'logs', 'alertas', 'alerts', 'servers', 'users', 'audit'],
  },
  suporte: {
    id: 'suporte',
    label: 'Suporte TI',
    description: 'Unidades, estações e inventário',
    tabs: ['stations', 'network', 'floorplan', 'sites', 'devices', 'alertas', 'alerts', 'logs', 'users'],
  },
};

export const PANEL_IDS = Object.keys(PANELS) as PanelId[];

export const ADMIN_TABS = new Set(['servers', 'users', 'audit', 'devices']);

const roleRank: Record<Role, number> = { viewer: 0, operator: 1, admin: 2 };

export const hasGlobalAdmin = (accesses: SiteAccess[]): boolean =>
  accesses.some((a) => a.site_id === null && roleRank[a.role] >= roleRank.admin);

export const TODAS_AS_UNIDADES = 'all';

export const unidadeEfetiva = (panel: PanelId, siteId: string): number | null =>
  panel === 'suporte' && siteId !== TODAS_AS_UNIDADES ? Number(siteId) : null;

const STORAGE_KEY = 'dockkeeper.panel';

export const loadPanel = (): PanelId => {
  try {
    const saved = localStorage.getItem(STORAGE_KEY);
    if (saved === 'dev' || saved === 'suporte') return saved;
  } catch {
  }
  return 'dev';
};

export const savePanel = (panel: PanelId) => {
  try {
    localStorage.setItem(STORAGE_KEY, panel);
  } catch {
  }
};

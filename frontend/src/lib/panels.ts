/**
 * O painel atende dois públicos com necessidades distintas:
 *
 * - **dev**: infraestrutura de servidor — VPS, containers, Nginx, certificados.
 * - **suporte**: parque de máquinas — unidades, estações, onde cada uma está e
 *   como está de CPU, memória e temperatura.
 *
 * São a mesma aplicação e a mesma autenticação; muda o conjunto de telas, para
 * o técnico de campo não navegar por container e o desenvolvedor não navegar
 * por planta baixa.
 */
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
    tabs: ['dashboard', 'history', 'containers', 'nginx', 'ssl', 'security', 'logs', 'alerts', 'servers', 'users', 'audit'],
  },
  suporte: {
    id: 'suporte',
    label: 'Suporte TI',
    description: 'Unidades, estações e inventário',
    tabs: ['stations', 'network', 'floorplan', 'sites', 'alerts', 'logs', 'users'],
  },
};

export const PANEL_IDS = Object.keys(PANELS) as PanelId[];

/**
 * Abas que exigem administrador GLOBAL: cadastro de servidores SSH, usuários e
 * o log de auditoria.
 *
 * O backend gateia as três com requireGlobalRole(admin) — administrar uma
 * filial não vira administrar o parque. O menu precisa usar a mesma régua:
 * mostrar uma porta trancada só gera chamado de suporte.
 */
export const ADMIN_TABS = new Set(['servers', 'users', 'audit']);

const roleRank: Record<Role, number> = { viewer: 0, operator: 1, admin: 2 };

/**
 * Espelha auth.GlobalRole do backend: o maior papel entre as concessões SEM
 * unidade. Papel de conta não conta aqui — quem é admin só da filial A tem
 * concessão com site_id preenchido, e ela não alcança o parque.
 *
 * Usuário sem nenhuma concessão cadastrada recebe do backend, no login, uma
 * concessão global sintética com o papel da conta, então a lista nunca chega
 * vazia para quem está autenticado.
 */
export const hasGlobalAdmin = (accesses: SiteAccess[]): boolean =>
  accesses.some((a) => a.site_id === null && roleRank[a.role] >= roleRank.admin);

const STORAGE_KEY = 'dockkeeper.panel';

/** Lê o painel escolhido antes. Storage pode estar bloqueado no browser. */
export const loadPanel = (): PanelId => {
  try {
    const saved = localStorage.getItem(STORAGE_KEY);
    if (saved === 'dev' || saved === 'suporte') return saved;
  } catch {
    // Sem acesso ao storage: cai no padrão.
  }
  return 'dev';
};

export const savePanel = (panel: PanelId) => {
  try {
    localStorage.setItem(STORAGE_KEY, panel);
  } catch {
    // Preferência não persistida não impede o uso.
  }
};

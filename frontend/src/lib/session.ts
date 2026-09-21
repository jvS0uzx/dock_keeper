export type Role = 'viewer' | 'operator' | 'admin';

export interface SiteAccess {
  site_id: number | null;
  role: Role;
}

export interface SessionInfo {
  token: string;
  user_id: number;
  username: string;
  nome?: string | null;
  email?: string | null;
  role: Role;
  expires_at: string;
  accesses: SiteAccess[];
}

export const ROLE_LABELS: Record<Role, string> = {
  viewer: 'Visualizador',
  operator: 'Suporte TI',
  admin: 'Administrador',
};

const roleRank: Record<Role, number> = { viewer: 0, operator: 1, admin: 2 };

export const canOperate = (role: Role): boolean => roleRank[role] >= roleRank.operator;
export const canAdmin = (role: Role): boolean => roleRank[role] >= roleRank.admin;

export const podeOperarNaUnidade = (accesses: SiteAccess[], siteId: number | null): boolean =>
  accesses.some(
    (a) => canOperate(a.role) && (a.site_id === null || (siteId !== null && a.site_id === siteId)),
  );

const STORAGE_KEY = 'dockkeeper.session';

export const loadSession = (): SessionInfo | null => {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return null;

    const session = JSON.parse(raw) as SessionInfo;
    if (!session.token || !session.username || !(session.role in roleRank)) return null;
    if (session.expires_at && new Date(session.expires_at).getTime() <= Date.now()) {
      clearSession();
      return null;
    }
    if (!Array.isArray(session.accesses)) session.accesses = [];
    return session;
  } catch {
    return null;
  }
};

export const saveSession = (session: SessionInfo) => {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(session));
  } catch {
  }
};

export const clearSession = () => {
  try {
    localStorage.removeItem(STORAGE_KEY);
  } catch {
  }
};

export const SESSION_EXPIRED_EVENT = 'dockkeeper:session-expired';

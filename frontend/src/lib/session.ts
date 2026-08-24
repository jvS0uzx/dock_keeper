/**
 * Sessão de usuário do painel.
 *
 * Convive com o VITE_API_TOKEN: o token é a credencial de máquina/dev e segue
 * funcionando; a sessão é a credencial de pessoa, com papel e escopo por
 * unidade. Quando as duas existem, a sessão vence — é ela que diz quem está
 * operando.
 */

export type Role = 'viewer' | 'operator' | 'admin';

/** Acesso concedido ao usuário. site_id null = acesso global (todas as unidades). */
export interface SiteAccess {
  site_id: number | null;
  role: Role;
}

export interface SessionInfo {
  token: string;
  user_id: number;
  username: string;
  role: Role;
  expires_at: string;
  accesses: SiteAccess[];
}

/**
 * Rótulos exibidos na interface. Os identificadores da API ficam em inglês
 * (projeto open source); o vocabulário do painel é decisão de produto.
 */
export const ROLE_LABELS: Record<Role, string> = {
  viewer: 'Visualizador',
  operator: 'Suporte TI',
  admin: 'Administrador',
};

const roleRank: Record<Role, number> = { viewer: 0, operator: 1, admin: 2 };

export const canOperate = (role: Role): boolean => roleRank[role] >= roleRank.operator;
export const canAdmin = (role: Role): boolean => roleRank[role] >= roleRank.admin;

const STORAGE_KEY = 'dockkeeper.session';

/** Lê a sessão guardada, descartando a vencida. Storage pode estar bloqueado. */
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
    // Storage bloqueado ou JSON corrompido: trata como deslogado.
    return null;
  }
};

export const saveSession = (session: SessionInfo) => {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(session));
  } catch {
    // Sem storage a sessão vive só nesta aba; o login continua funcionando.
  }
};

export const clearSession = () => {
  try {
    localStorage.removeItem(STORAGE_KEY);
  } catch {
    // Nada a limpar se o storage está bloqueado.
  }
};

/** Nome do evento disparado quando o backend responde 401 para uma sessão ativa. */
export const SESSION_EXPIRED_EVENT = 'vd:session-expired';

import { createContext, useContext } from 'react';
import { canAdmin, canOperate, type Role, type SiteAccess } from '../../lib/session';

/**
 * Quem está usando o painel agora. Alimentado pelo App após o login (ou pelo
 * modo token, quando só o VITE_API_TOKEN existe) e consumido pelas views para
 * esconder o que o papel não permite.
 */
export interface SessionState {
  username: string;
  role: Role;
  accesses: SiteAccess[];
  /** true quando a credencial é o token de máquina/dev, sem pessoa por trás. */
  isToken: boolean;
  logout: () => void;
}

export const SessionContext = createContext<SessionState | null>(null);

export const useSession = (): SessionState => {
  const ctx = useContext(SessionContext);
  if (!ctx) throw new Error('useSession precisa estar dentro de <SessionContext.Provider>');
  return ctx;
};

/** Atalhos de gating usados pelas views. */
export const useRole = () => {
  const { role } = useSession();
  return { role, canOperate: canOperate(role), canAdmin: canAdmin(role) };
};

import { createContext, useContext } from 'react';
import { canAdmin, canOperate, type Role, type SiteAccess } from '../../lib/session';

export interface SessionState {
  username: string;
  nome?: string | null;
  role: Role;
  accesses: SiteAccess[];
  isToken: boolean;
  logout: () => void;
}

export const SessionContext = createContext<SessionState | null>(null);

export const useSession = (): SessionState => {
  const ctx = useContext(SessionContext);
  if (!ctx) throw new Error('useSession precisa estar dentro de <SessionContext.Provider>');
  return ctx;
};

export const useRole = () => {
  const { role } = useSession();
  return { role, canOperate: canOperate(role), canAdmin: canAdmin(role) };
};

import { createContext, useContext } from 'react';

export interface NavigationState {
  openSite: (siteId: number) => void;
  openMachine: (serverId: string) => void;
  goBack: () => void;
}

export const NavigationContext = createContext<NavigationState | null>(null);

export const useNavigation = (): NavigationState => {
  const ctx = useContext(NavigationContext);
  if (!ctx) throw new Error('useNavigation precisa estar dentro de <NavigationContext.Provider>');
  return ctx;
};

import { createContext, useContext } from 'react';

/**
 * Navegação em profundidade dentro de uma aba.
 *
 * O painel é organizado por unidade: unidade → máquinas dela → uma máquina.
 * Isso não cabe na lista plana de abas, e trazer um roteador só para dois
 * níveis de detalhe seria peso desnecessário — a tela de detalhe substitui o
 * conteúdo da aba e `goBack` devolve o operador de onde ele veio.
 */
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

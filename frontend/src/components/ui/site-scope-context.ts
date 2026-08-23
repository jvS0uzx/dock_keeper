import { createContext, useContext } from 'react';
import type { Site } from '../../lib/api';

/** Valor especial: nenhuma unidade escolhida, mostra tudo que o papel alcança. */
export const ALL_SITES = 'all';

/**
 * Unidade selecionada no painel de Suporte TI.
 *
 * Antes cada tela tinha o próprio filtro de unidade e o operador reescolhia a
 * filial a cada aba. O escopo vive aqui: quem trabalha numa unidade escolhe uma
 * vez e todas as telas seguem.
 *
 * `siteId` é string por conveniência do <Select>; `numericSiteId` é o que vai
 * para a API, nulo quando nenhuma unidade está selecionada.
 */
export interface SiteScopeState {
  siteId: string;
  numericSiteId: number | null;
  setSiteId: (value: string) => void;
  sites: Site[];
  /** Nome da unidade pelo id, para as telas rotularem sem refazer a busca. */
  siteName: (id: number | null) => string;
  /** Recarrega a lista após cadastro ou remoção de unidade. */
  reloadSites: () => void;
}

export const SiteScopeContext = createContext<SiteScopeState | null>(null);

export const useSiteScope = (): SiteScopeState => {
  const ctx = useContext(SiteScopeContext);
  if (!ctx) throw new Error('useSiteScope precisa estar dentro de <SiteScopeContext.Provider>');
  return ctx;
};

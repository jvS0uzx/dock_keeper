import { createContext, useContext } from 'react';
import type { Site } from '../../lib/api';

export const ALL_SITES = 'all';

export interface SiteScopeState {
  siteId: string;
  numericSiteId: number | null;
  setSiteId: (value: string) => void;
  sites: Site[];
  siteName: (id: number | null) => string;
  reloadSites: () => void;
  sitesError?: string | null;
}

export const SiteScopeContext = createContext<SiteScopeState | null>(null);

export const useSiteScope = (): SiteScopeState => {
  const ctx = useContext(SiteScopeContext);
  if (!ctx) throw new Error('useSiteScope precisa estar dentro de <SiteScopeContext.Provider>');
  return ctx;
};

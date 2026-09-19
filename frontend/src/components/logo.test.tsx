import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';

import LoginView from './LoginView';
import Sidebar from './Sidebar';
import { SessionContext, type SessionState } from './ui/session-context';
import { SiteScopeContext, type SiteScopeState } from './ui/site-scope-context';

type LeitorDeArquivo = {
  readFileSync(caminho: string, codificacao: 'utf8'): string;
  existsSync(caminho: string): boolean;
};

const fs = (
  globalThis as unknown as { process: { getBuiltinModule(id: 'node:fs'): LeitorDeArquivo } }
).process.getBuiltinModule('node:fs');

const sessao: SessionState = {
  username: 'p',
  role: 'admin',
  accesses: [{ site_id: null, role: 'admin' }],
  isToken: false,
  logout: vi.fn(),
};

const escopo: SiteScopeState = {
  siteId: 'all',
  setSiteId: vi.fn(),
  numericSiteId: null,
  sites: [],
  siteName: () => '',
  reloadSites: vi.fn(),
  sitesError: null,
};

describe('logo do DockKeeper', () => {
  it('aparece no login, com alt descritivo e sem deformar', () => {
    render(<LoginView onLogin={vi.fn()} />);

    const logo = screen.getByAltText(/dockkeeper/i) as HTMLImageElement;
    expect(logo.tagName).toBe('IMG');
    expect(logo.getAttribute('width')).toBeTruthy();
    expect(logo.getAttribute('height')).toBeTruthy();
    expect(logo.getAttribute('width')).toBe(logo.getAttribute('height'));
  });

  it('aparece na barra lateral, ao lado do nome', () => {
    render(
      <SessionContext.Provider value={sessao}>
        <SiteScopeContext.Provider value={escopo}>
          <Sidebar activeTab="dashboard" setActiveTab={vi.fn()} panel="dev" setPanel={vi.fn()} />
        </SiteScopeContext.Provider>
      </SessionContext.Provider>,
    );

    const logo = screen.getByAltText(/dockkeeper/i) as HTMLImageElement;
    const largura = Number(logo.getAttribute('width'));
    expect(largura).toBeGreaterThanOrEqual(28);
    expect(largura).toBeLessThanOrEqual(36);
  });

  it('o index.html aponta para o favicon png e para o apple-touch-icon', () => {
    const html = fs.readFileSync('index.html', 'utf8');

    expect(html).toMatch(/href="\/favicon-32\.png"/);
    expect(html).toMatch(/rel="apple-touch-icon"[^>]*href="\/apple-touch-icon\.png"/);
    expect(fs.existsSync('public/favicon-32.png')).toBe(true);
    expect(fs.existsSync('public/apple-touch-icon.png')).toBe(true);
  });
});

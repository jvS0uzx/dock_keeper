import { describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';

import SslView from './SslView';
import { SessionContext, type SessionState } from './ui/session-context';
import { DialogContext, type DialogApi } from './ui/dialog-context';
import type { Role } from '../lib/session';

vi.mock('../lib/api', () => ({
  api: {
    domains: vi.fn(async () => []),
    discoverDomains: vi.fn(async () => []),
  },
}));

const dialogoFalso: DialogApi = {
  confirm: vi.fn(async () => true),
  prompt: vi.fn(async () => null),
  notify: vi.fn(),
};

const renderComPapel = (role: Role) => {
  const sessao: SessionState = {
    username: 'pessoa-de-teste',
    role,
    accesses: [{ site_id: null, role }],
    isToken: false,
    logout: vi.fn(),
  };

  return render(
    <SessionContext.Provider value={sessao}>
      <DialogContext.Provider value={dialogoFalso}>
        <SslView />
      </DialogContext.Provider>
    </SessionContext.Provider>,
  );
};

describe('SslView — visualizador é somente-leitura', () => {
  it('não oferece a ação de forçar handshake ao visualizador', async () => {
    renderComPapel('viewer');

    await waitFor(() => expect(screen.getByText(/Domínios/)).toBeTruthy());

    expect(screen.queryByRole('button', { name: /forçar handshake/i })).toBeNull();
  });

  it('oferece a ação ao Suporte TI', async () => {
    renderComPapel('operator');

    await waitFor(() =>
      expect(screen.getByRole('button', { name: /forçar handshake/i })).toBeTruthy(),
    );
  });
});

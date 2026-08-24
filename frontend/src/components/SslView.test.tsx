import { describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';

import SslView from './SslView';
import { SessionContext, type SessionState } from './ui/session-context';
import { DialogContext, type DialogApi } from './ui/dialog-context';
import type { Role } from '../lib/session';

// A tela busca domínios e descobertas na montagem. O mock devolve listas vazias:
// o que se mede aqui é o gate de papel, e uma tabela populada só acrescentaria
// texto para as buscas tropeçarem.
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

// "Visualizador não cadastra nada" é regra de produto, e o backend a aplica com
// requireRoleByMethod(viewer, operator). A tela precisa usar a mesma régua:
// oferecer um botão que sempre responde 403 gera chamado de suporte, e some com
// a confiança de quem usa.
describe('SslView — visualizador é somente-leitura', () => {
  it('não oferece a ação de forçar handshake ao visualizador', async () => {
    renderComPapel('viewer');

    // Espera a carga inicial terminar antes de concluir ausência: procurar cedo
    // demais acharia o estado de carregamento e o teste passaria sozinho.
    await waitFor(() => expect(screen.getByText(/Domínios/)).toBeTruthy());

    expect(screen.queryByRole('button', { name: /forçar handshake/i })).toBeNull();
  });

  // O contraponto: sem ele o teste acima seria satisfeito por uma tela que não
  // renderiza o botão para ninguém.
  it('oferece a ação ao Suporte TI', async () => {
    renderComPapel('operator');

    await waitFor(() =>
      expect(screen.getByRole('button', { name: /forçar handshake/i })).toBeTruthy(),
    );
  });
});

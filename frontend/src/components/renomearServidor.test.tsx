import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, within } from '@testing-library/react';
import { semEspera } from '../test/usuario';

import ServersView from './ServersView';
import { DialogContext, type DialogApi } from './ui/dialog-context';
import { SessionContext, type SessionState } from './ui/session-context';
import type { SiteAccess } from '../lib/session';

const cadastrados = [
  { id: 'a', name: 'vps-app', host_ip: '10.0.0.1', user: 'root', port: 22, created_at: '' },
  { id: 'b', name: 'vps-banco', host_ip: '10.0.0.2', user: 'root', port: 22, created_at: '' },
];

const api = vi.hoisted(() => ({
  servers: vi.fn(),
  liveMetrics: vi.fn(),
  renameServer: vi.fn(),
}));

vi.mock('../lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../lib/api')>()),
  api,
}));

const dialogo: DialogApi = { confirm: vi.fn(), prompt: vi.fn(), notify: vi.fn() };

beforeEach(() => {
  api.servers.mockReset().mockResolvedValue(cadastrados);
  api.liveMetrics.mockReset().mockResolvedValue({ servers: [], containers: [], load_balancing: [] });
  api.renameServer.mockReset().mockResolvedValue({ ...cadastrados[0], name: 'vps-producao' });
  dialogo.prompt = vi.fn();
  dialogo.notify = vi.fn();
});

const renderizar = (accesses: SiteAccess[] = [{ site_id: null, role: 'admin' }]) => {
  const sessao: SessionState = {
    username: 'p',
    role: accesses[0].role,
    accesses,
    isToken: false,
    logout: vi.fn(),
  };
  return render(
    <SessionContext.Provider value={sessao}>
      <DialogContext.Provider value={dialogo}>
        <ServersView />
      </DialogContext.Provider>
    </SessionContext.Provider>,
  );
};

const linhaDe = async (nome: string) => {
  const celula = await screen.findByText(nome);
  const linha = celula.closest('tr');
  if (!linha) throw new Error(`linha de ${nome} não encontrada`);
  return within(linha);
};

describe('renomear servidor pela tela de Servidores', () => {
  it('renomeia e recarrega a lista', async () => {
    renderizar();
    (dialogo.prompt as ReturnType<typeof vi.fn>).mockResolvedValue('vps-producao');

    const linha = await linhaDe('vps-app');
    await semEspera().click(linha.getByRole('button', { name: /renomear/i }));

    expect(api.renameServer).toHaveBeenCalledWith('a', 'vps-producao');
    expect(api.servers).toHaveBeenCalledTimes(2);
  });

  it('não chama a API quando a pessoa cancela', async () => {
    renderizar();
    (dialogo.prompt as ReturnType<typeof vi.fn>).mockResolvedValue(null);

    const linha = await linhaDe('vps-app');
    await semEspera().click(linha.getByRole('button', { name: /renomear/i }));

    expect(api.renameServer).not.toHaveBeenCalled();
  });

  it('mostra a mensagem do backend quando o nome já existe', async () => {
    renderizar();
    (dialogo.prompt as ReturnType<typeof vi.fn>).mockResolvedValue('vps-banco');
    api.renameServer.mockRejectedValue(
      new Error(JSON.stringify({ error: 'já existe um servidor com esse nome nesta unidade' })),
    );

    const linha = await linhaDe('vps-app');
    await semEspera().click(linha.getByRole('button', { name: /renomear/i }));

    expect(dialogo.notify).toHaveBeenCalledWith(
      'já existe um servidor com esse nome nesta unidade',
      'error',
    );
  });

  it('admin de uma unidade só não vê o botão de renomear', async () => {
    renderizar([{ site_id: 4, role: 'admin' }]);

    const linha = await linhaDe('vps-app');
    expect(linha.queryByRole('button', { name: /renomear/i })).toBeNull();
  });
});

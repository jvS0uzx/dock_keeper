import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import { semEspera } from '../test/usuario';

import MachineAdminPanel from './MachineAdminPanel';
import { DialogContext, type DialogApi } from './ui/dialog-context';
import { SessionContext, type SessionState } from './ui/session-context';
import type { ServerLiveStat } from '../lib/api';

const api = vi.hoisted(() => ({
  servers: vi.fn(),
  setServerAbsenceAlert: vi.fn(),
  updateServerAliases: vi.fn(),
}));

vi.mock('../lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../lib/api')>()),
  api,
}));

const confirmar = vi.fn();
const notificar = vi.fn();
const dialogo: DialogApi = { confirm: confirmar, prompt: vi.fn(), notify: notificar };

const adminGlobal: SessionState = {
  username: 'p',
  role: 'admin',
  accesses: [{ site_id: null, role: 'admin' }],
  isToken: false,
  logout: vi.fn(),
};
const adminDaFilial: SessionState = { ...adminGlobal, accesses: [{ site_id: 3, role: 'admin' }] };
const viewer: SessionState = { ...adminGlobal, role: 'viewer', accesses: [{ site_id: null, role: 'viewer' }] };

const estacao = { id: 'srv-1', name: 'estacao-01', kind: 'agent', absence_alert: false } as ServerLiveStat;
const servidor = { ...estacao, kind: 'ssh' } as ServerLiveStat;

const cadastro = {
  id: 'srv-1',
  name: 'estacao-01',
  host_ip: '192.0.2.10',
  user: '',
  port: 22,
  created_at: '2026-09-01T00:00:00Z',
  aliases: ['10.0.0.5', '10.0.0.6:8080'],
  addresses: ['192.0.2.10', '10.0.0.5', '10.0.0.6:8080'],
  absence_alert: false,
};

const falha = (mensagem: string) => new Error(JSON.stringify({ error: mensagem }));

const renderizar = (maquina: ServerLiveStat, sessao = adminGlobal) =>
  render(
    <SessionContext.Provider value={sessao}>
      <DialogContext.Provider value={dialogo}>
        <MachineAdminPanel machine={maquina} />
      </DialogContext.Provider>
    </SessionContext.Provider>,
  );

beforeEach(() => {
  vi.clearAllMocks();
  api.servers.mockResolvedValue([cadastro]);
  confirmar.mockResolvedValue(true);
});

describe('aviso de ausência por estação', () => {
  it('nasce desligado, explica o porquê e liga por PATCH', async () => {
    api.setServerAbsenceAlert.mockResolvedValue({ ...cadastro, absence_alert: true });
    renderizar(estacao);

    const controle = await screen.findByRole('checkbox', { name: /Avisar quando esta estação parar de enviar/ });
    expect((controle as HTMLInputElement).checked).toBe(false);
    expect(screen.getByText(/estação desligada fora do expediente geraria aviso/)).toBeTruthy();

    await semEspera().click(controle);
    expect(api.setServerAbsenceAlert).toHaveBeenCalledWith('srv-1', true);
    await waitFor(() => expect((controle as HTMLInputElement).checked).toBe(true));
  });

  it('recusa do backend aparece e o controle volta', async () => {
    api.setServerAbsenceAlert.mockRejectedValue(falha('apenas admin global'));
    renderizar(estacao);

    const controle = await screen.findByRole('checkbox', { name: /Avisar quando/ });
    await semEspera().click(controle);

    await waitFor(() => expect(notificar).toHaveBeenCalledWith('apenas admin global', 'error'));
    expect((controle as HTMLInputElement).checked).toBe(false);
  });

  it('servidor por SSH não oferece o controle', async () => {
    renderizar(servidor);
    await screen.findByText('10.0.0.5');
    expect(screen.queryByRole('checkbox', { name: /Avisar quando/ })).toBeNull();
  });

  it('quem não é admin global não vê nada nem consulta o cadastro', () => {
    const { container: deViewer } = renderizar(estacao, viewer);
    const { container: deFilial } = renderizar(estacao, adminDaFilial);
    expect(deViewer.textContent).toBe('');
    expect(deFilial.textContent).toBe('');
    expect(api.servers).not.toHaveBeenCalled();
  });
});

describe('alias manual', () => {
  it('lista só os manuais, não o endereço principal', async () => {
    renderizar(estacao);
    expect(await screen.findByText('10.0.0.5')).toBeTruthy();
    expect(screen.getByText('10.0.0.6:8080')).toBeTruthy();
    expect(screen.queryByText('192.0.2.10')).toBeNull();
  });

  it('remover confirma e manda a lista sem o alias', async () => {
    api.updateServerAliases.mockResolvedValue({ ...cadastro, aliases: ['10.0.0.6:8080'] });
    renderizar(estacao);

    await semEspera().click(await screen.findByRole('button', { name: 'Remover 10.0.0.5' }));

    expect(confirmar).toHaveBeenCalledWith(expect.objectContaining({ danger: true }));
    expect(api.updateServerAliases).toHaveBeenCalledWith('srv-1', ['10.0.0.6:8080']);
    await waitFor(() => expect(screen.queryByText('10.0.0.5')).toBeNull());
    expect(screen.getByText('10.0.0.6:8080')).toBeTruthy();
  });

  it('remover o último manda lista vazia', async () => {
    api.servers.mockResolvedValue([{ ...cadastro, aliases: ['10.0.0.5'] }]);
    api.updateServerAliases.mockResolvedValue({ ...cadastro, aliases: [] });
    renderizar(estacao);

    await semEspera().click(await screen.findByRole('button', { name: 'Remover 10.0.0.5' }));

    expect(api.updateServerAliases).toHaveBeenCalledWith('srv-1', []);
    expect(await screen.findByText(/Nenhum endereço associado à mão/)).toBeTruthy();
  });

  it('sem confirmação nada é enviado', async () => {
    confirmar.mockResolvedValue(false);
    renderizar(estacao);
    await semEspera().click(await screen.findByRole('button', { name: 'Remover 10.0.0.5' }));
    expect(api.updateServerAliases).not.toHaveBeenCalled();
  });

  it('falha ao remover mostra o erro do backend e mantém o alias', async () => {
    api.updateServerAliases.mockRejectedValue(falha('falha ao gravar'));
    renderizar(estacao);
    await semEspera().click(await screen.findByRole('button', { name: 'Remover 10.0.0.5' }));
    await waitFor(() => expect(notificar).toHaveBeenCalledWith('falha ao gravar', 'error'));
    expect(screen.getByText('10.0.0.5')).toBeTruthy();
  });

  it('falha ao ler o cadastro aparece, não vira lista vazia', async () => {
    api.servers.mockRejectedValue(falha('banco fora do ar'));
    renderizar(estacao);
    expect(await screen.findByText(/banco fora do ar/)).toBeTruthy();
    expect(screen.queryByText(/Nenhum endereço associado/)).toBeNull();
  });
});

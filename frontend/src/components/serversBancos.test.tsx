import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, within } from '@testing-library/react';
import { semEspera } from '../test/usuario';

import ServersView from './ServersView';
import { DialogContext, type DialogApi } from './ui/dialog-context';
import { SessionContext, type SessionState } from './ui/session-context';

const cadastro = [
  { id: 'db1', name: 'VPS Bancos', host_ip: '203.0.113.20', user: 'root', port: 22, created_at: '', collect_bancos: true },
  { id: 'db2', name: 'VPS Sem Sonda', host_ip: '203.0.113.21', user: 'root', port: 22, created_at: '', collect_bancos: false },
  { id: 'db3', name: 'VPS Antiga', host_ip: '203.0.113.22', user: 'root', port: 22, created_at: '' },
];

const api = vi.hoisted(() => ({
  servers: vi.fn(),
  liveMetrics: vi.fn(),
  setServerCollectBancos: vi.fn(),
}));

vi.mock('../lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../lib/api')>()),
  api,
}));

const dialogo: DialogApi = { confirm: vi.fn(), prompt: vi.fn(), notify: vi.fn() };

beforeEach(() => {
  api.servers.mockReset().mockResolvedValue(cadastro);
  api.liveMetrics.mockReset().mockResolvedValue({ servers: [], containers: [], load_balancing: [] });
  api.setServerCollectBancos.mockReset().mockResolvedValue({});
  (dialogo.notify as ReturnType<typeof vi.fn>).mockReset();
});

const renderizar = () => {
  const sessao: SessionState = {
    username: 'p',
    role: 'admin',
    accesses: [{ site_id: null, role: 'admin' }],
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

describe('ServersView — sonda de bancos por servidor', () => {
  it('ganha a coluna Bancos ao lado da do Nginx', async () => {
    renderizar();

    const cabecalhos = (await screen.findAllByRole('columnheader')).map((th) => th.textContent);
    expect(cabecalhos.indexOf('Bancos')).toBe(cabecalhos.indexOf('Nginx') + 1);
  });

  it('desliga a sonda chamando o PATCH com collect_bancos false', async () => {
    const usuario = semEspera();
    renderizar();

    const linha = await linhaDe('VPS Bancos');
    expect(linha.getByText('Sonda de bancos ligada')).toBeTruthy();
    await usuario.click(linha.getByRole('button', { name: 'Desligar sonda de bancos' }));

    expect(api.setServerCollectBancos).toHaveBeenCalledWith('db1', false);
    expect(api.servers).toHaveBeenCalledTimes(2);
  });

  it('religa a sonda desligada', async () => {
    const usuario = semEspera();
    renderizar();

    const linha = await linhaDe('VPS Sem Sonda');
    expect(linha.getByText('Sonda de bancos desligada')).toBeTruthy();
    await usuario.click(linha.getByRole('button', { name: 'Ligar sonda de bancos' }));

    expect(api.setServerCollectBancos).toHaveBeenCalledWith('db2', true);
  });

  it('servidor sem o campo é tratado como ligado, que é o padrão', async () => {
    renderizar();

    const linha = await linhaDe('VPS Antiga');
    expect(linha.getByText('Sonda de bancos ligada')).toBeTruthy();
  });

  it('avisa quando o PATCH falha', async () => {
    const usuario = semEspera();
    api.setServerCollectBancos.mockRejectedValueOnce(new Error('falhou'));
    renderizar();

    const linha = await linhaDe('VPS Bancos');
    await usuario.click(linha.getByRole('button', { name: 'Desligar sonda de bancos' }));

    expect(dialogo.notify).toHaveBeenCalledWith(expect.any(String), 'error');
  });
});

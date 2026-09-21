import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, within } from '@testing-library/react';
import { semEspera } from '../test/usuario';

import ServersView from './ServersView';
import { DialogContext, type DialogApi } from './ui/dialog-context';
import { SessionContext, type SessionState } from './ui/session-context';

const cadastro = [
  { id: 'lb1', name: 'LB Principal', host_ip: '203.0.113.10', user: 'root', port: 22, created_at: '' },
  { id: 'lb2', name: 'LB Reserva', host_ip: '203.0.113.11', user: 'root', port: 22, created_at: '' },
  { id: 'n1', name: 'NODE 1', host_ip: '198.51.100.21', user: 'root', port: 22, created_at: '' },
  { id: 'n2', name: 'NODE 2', host_ip: '198.51.100.22', user: 'root', port: 22, created_at: '' },
];

const vivos = [
  { id: 'lb1', online: true, cpu: 1, mem_used: 1, mem_total: 2, load1: 0.1, rtt_ms: 10, collect_nginx: false, nginx_estado: 'candidato', nginx_papel: 'principal', nginx_motivo: '' },
  { id: 'lb2', online: true, cpu: 1, mem_used: 1, mem_total: 2, load1: 0.1, rtt_ms: 10, collect_nginx: false, nginx_estado: 'candidato', nginx_papel: 'reserva', nginx_motivo: '' },
  { id: 'n1', online: true, cpu: 1, mem_used: 1, mem_total: 2, load1: 0.1, rtt_ms: 10, collect_nginx: false, nginx_estado: 'inativo', nginx_papel: 'nenhum', nginx_motivo: 'Nginx ativo, log sem permissão de leitura para o usuário deploy' },
  { id: 'n2', online: true, cpu: 1, mem_used: 1, mem_total: 2, load1: 0.1, rtt_ms: 10, collect_nginx: true, nginx_estado: 'ausente', nginx_papel: 'nenhum', nginx_motivo: '' },
];

const api = vi.hoisted(() => ({
  servers: vi.fn(),
  liveMetrics: vi.fn(),
  setServerCollectNginx: vi.fn(),
}));

vi.mock('../lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../lib/api')>()),
  api,
}));

const dialogo: DialogApi = { confirm: vi.fn(), prompt: vi.fn(), notify: vi.fn() };

beforeEach(() => {
  api.servers.mockReset().mockResolvedValue(cadastro);
  api.liveMetrics.mockReset().mockResolvedValue({ servers: vivos, containers: [], load_balancing: [] });
  api.setServerCollectNginx.mockReset().mockResolvedValue({});
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

describe('ServersView — descoberta do Nginx', () => {
  it('separa principal de reserva em vez de mostrar os dois iguais', async () => {
    renderizar();

    expect((await linhaDe('LB Principal')).getByText('Principal')).toBeTruthy();
    expect((await linhaDe('LB Reserva')).getByText('Reserva')).toBeTruthy();
  });

  it('mostra o motivo quando a descoberta não conseguiu classificar', async () => {
    renderizar();

    const linha = await linhaDe('NODE 1');
    expect(linha.getByText('Nginx parado')).toBeTruthy();
    expect(linha.getByText(/log sem permissão de leitura para o usuário deploy/)).toBeTruthy();
  });

  it('trata ausência de Nginx como informação neutra, não como erro', async () => {
    renderizar();

    const linha = await linhaDe('NODE 2');
    const marca = linha.getByText('Sem Nginx');
    expect(marca.className).toContain('badge-muted');
    expect(marca.className).not.toContain('badge-warn');
  });

  it('liga a coleta do log pelo cartão do servidor', async () => {
    const usuario = semEspera();
    renderizar();

    const linha = await linhaDe('NODE 1');
    await usuario.click(linha.getByRole('button', { name: 'Coletar log do Nginx' }));

    expect(api.setServerCollectNginx).toHaveBeenCalledWith('n1', true);
  });

  it('mostra que a coleta está ligada na mão e permite desligar', async () => {
    const usuario = semEspera();
    renderizar();

    const linha = await linhaDe('NODE 2');
    expect(linha.getByText('coleta manual')).toBeTruthy();
    await usuario.click(linha.getByRole('button', { name: 'Parar de coletar o log' }));

    expect(api.setServerCollectNginx).toHaveBeenCalledWith('n2', false);
  });
});

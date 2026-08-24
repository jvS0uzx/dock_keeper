import { useState, useEffect, type FormEvent } from 'react';
import { ShieldOff } from 'lucide-react';
import { api, type ServerLiveStat, type ServerRecord as Server } from '../lib/api';
import { formatGB } from '../lib/format';
import { useDialog } from './ui/dialog-context';
import { useRole } from './ui/session-context';

const emptyForm = { name: '', host_ip: '', user: 'root' };

const ServersView = () => {
  const dialog = useDialog();
  const { canAdmin } = useRole();
  const [servers, setServers] = useState<Server[]>([]);
  const [liveStats, setLiveStats] = useState<Record<string, ServerLiveStat>>({});
  const [loading, setLoading] = useState(true);
  const [form, setForm] = useState({ ...emptyForm });

  const fetchServers = async () => {
    try {
      setServers(await api.servers());
    } catch (err) {
      console.error(err);
    } finally {
      setLoading(false);
    }
  };

  const fetchLiveStatus = async (signal?: AbortSignal) => {
    try {
      const data = await api.liveMetrics(signal);
      setLiveStats(Object.fromEntries(data.servers.map(s => [s.id, s])));
    } catch {
      // Falha de polling não interrompe a tela: a próxima rodada tenta de novo.
    }
  };

  useEffect(() => {
    const controller = new AbortController();
    fetchServers();
    fetchLiveStatus(controller.signal);
    const interval = setInterval(() => fetchLiveStatus(controller.signal), 5000);
    return () => {
      clearInterval(interval);
      controller.abort();
    };
  }, []);

  const handleAdd = async (e: FormEvent) => {
    e.preventDefault();
    if (!form.host_ip || !form.name) return;

    try {
      await api.createServer(form);
      setForm({ ...emptyForm });
      fetchServers();
      dialog.notify(`${form.name} entrou no monitoramento.`, 'success');
    } catch (err) {
      console.error(err);
      dialog.notify('Erro ao cadastrar o servidor.', 'error');
    }
  };

  const handleDelete = async (server: Server) => {
    const confirmed = await dialog.confirm({
      title: `Remover ${server.name}?`,
      message: 'A coleta via SSH para e os gráficos deste host param imediatamente.',
      confirmLabel: 'Remover',
      danger: true,
    });
    if (!confirmed) return;
    try {
      await api.deleteServer(server.id);
      fetchServers();
    } catch (err) {
      console.error(err);
      dialog.notify('Erro ao remover o servidor.', 'error');
    }
  };

  // Servidor cadastrado entrega SSH root: tela restrita a administrador. A aba
  // já some para os demais; isto cobre acesso por estado antigo.
  if (!canAdmin) {
    return (
      <div className="p-8 h-full flex flex-col items-center justify-center text-text-mut gap-3">
        <ShieldOff size={32} strokeWidth={1.75} className="opacity-40" />
        <p className="text-sm">Seu perfil não permite esta tela.</p>
      </div>
    );
  }

  return (
    <div className="p-4 md:p-8 anim-rise">
      <div className="page-header">
        <div>
          <h1 className="page-title">Servidores</h1>
          <p className="page-desc">
            VPS monitoradas por SSH. Cadastro novo abre a conexão na hora (hot-plug), sem reiniciar o painel.
          </p>
        </div>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        <div className="panel p-6 col-span-1 h-fit">
          <h2 className="eyebrow mb-6">Adicionar servidor</h2>
          <form onSubmit={handleAdd} className="flex flex-col gap-4">
            <div>
              <label htmlFor="server-name" className="eyebrow block mb-1.5">Nome de identificação</label>
              <input
                id="server-name"
                type="text"
                value={form.name}
                onChange={(e) => setForm({...form, name: e.target.value})}
                className="input-base w-full"
                placeholder="Ex: VPS Produção"
                required
              />
            </div>
            <div>
              <label htmlFor="server-ip" className="eyebrow block mb-1.5">Endereço IP</label>
              <input
                id="server-ip"
                type="text"
                value={form.host_ip}
                onChange={(e) => setForm({...form, host_ip: e.target.value})}
                className="input-base w-full mono-data"
                placeholder="Ex: 203.0.113.10"
                required
              />
            </div>
            <div>
              <label htmlFor="server-user" className="eyebrow block mb-1.5">Usuário SSH</label>
              <input
                id="server-user"
                type="text"
                value={form.user}
                onChange={(e) => setForm({...form, user: e.target.value})}
                className="input-base w-full mono-data"
                placeholder="Padrão: root"
              />
            </div>
            <button type="submit" className="btn btn-primary mt-2">
              Conectar VPS
            </button>
          </form>
        </div>

        <div className="panel p-6 col-span-1 lg:col-span-2">
          <h2 className="eyebrow mb-6">Servidores ativos</h2>
          {loading ? (
            <p className="text-sm text-text-mut">Carregando...</p>
          ) : servers.length === 0 ? (
            <p className="text-sm text-text-mut">Nenhum servidor cadastrado.</p>
          ) : (
            <div className="overflow-x-auto custom-scrollbar">
              <table className="table-base whitespace-nowrap">
                <thead>
                  <tr>
                    <th>Status</th>
                    <th>Nome</th>
                    <th>IP</th>
                    <th className="text-right">CPU</th>
                    <th className="text-right">RAM</th>
                    <th className="text-right">Load</th>
                    <th className="text-right">Ação</th>
                  </tr>
                </thead>
                <tbody>
                  {servers.map((s) => {
                    const live = liveStats[s.id];
                    const isOnline = !!live && live.online;
                    return (
                    <tr key={s.id}>
                      <td>
                        <span className={`badge ${isOnline ? 'badge-ok' : 'badge-crit'}`}>
                          <span className={`w-1.5 h-1.5 rounded-full ${isOnline ? 'bg-ok animate-pulse' : 'bg-crit'}`} />
                          {isOnline ? 'Online' : 'Offline'}
                        </span>
                      </td>
                      <td className="font-medium text-text-hi">{s.name}</td>
                      <td className="mono-data text-text-mut">{s.host_ip}</td>
                      <td className="text-right mono-data text-text-hi">
                        {isOnline ? `${live.cpu.toFixed(0)}%` : <span className="text-text-faint">-</span>}
                      </td>
                      <td className="text-right mono-data text-text-hi">
                        {isOnline && live.mem_total > 0
                          ? <span>{formatGB(live.mem_used)}<span className="text-text-faint text-xs">/{formatGB(live.mem_total)}GB</span></span>
                          : <span className="text-text-faint">-</span>}
                      </td>
                      <td className="text-right mono-data text-text-hi">
                        {isOnline ? live.load1.toFixed(2) : <span className="text-text-faint">-</span>}
                      </td>
                      <td className="text-right">
                        <button onClick={() => handleDelete(s)} className="btn btn-danger btn-sm">
                          Remover
                        </button>
                      </td>
                    </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          )}
        </div>
      </div>
    </div>
  );
};

export default ServersView;

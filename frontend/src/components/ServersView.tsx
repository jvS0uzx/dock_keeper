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
      <div className="p-8 h-full flex flex-col items-center justify-center text-[#737373] gap-3">
        <ShieldOff size={32} className="opacity-40" />
        <p className="text-sm">Seu perfil não permite esta tela.</p>
      </div>
    );
  }

  return (
    <div className="p-8">
      <h1 className="text-2xl font-light text-white mb-2">Gestão de <span className="font-bold">Servidores</span></h1>
      <p className="text-[#737373] text-sm mb-8">Adicione ou remova servidores VPS para monitoramento. As conexões SSH são iniciadas instantaneamente de forma Hot-Plug.</p>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        <div className="glass-panel p-6 rounded-xl border border-white/5 bg-white/[0.02] col-span-1 h-fit">
          <h2 className="text-sm font-bold tracking-widest text-[#737373] uppercase mb-6">Adicionar Servidor</h2>
          <form onSubmit={handleAdd} className="flex flex-col gap-4">
            <div>
              <label htmlFor="server-name" className="text-xs text-[#737373] block mb-1">Nome de Identificação</label>
              <input
                id="server-name"
                type="text"
                value={form.name}
                onChange={(e) => setForm({...form, name: e.target.value})} 
                className="w-full bg-black/40 border border-white/10 rounded-lg px-4 py-2 text-sm text-white focus:outline-none focus:border-[#10b981] transition-colors"
                placeholder="Ex: VPS Produção"
                required
              />
            </div>
            <div>
              <label htmlFor="server-ip" className="text-xs text-[#737373] block mb-1">Endereço IP</label>
              <input
                id="server-ip"
                type="text"
                value={form.host_ip}
                onChange={(e) => setForm({...form, host_ip: e.target.value})} 
                className="w-full bg-black/40 border border-white/10 rounded-lg px-4 py-2 text-sm text-white focus:outline-none focus:border-[#10b981] transition-colors"
                placeholder="Ex: 203.0.113.10"
                required
              />
            </div>
            <div>
              <label htmlFor="server-user" className="text-xs text-[#737373] block mb-1">Usuário SSH</label>
              <input
                id="server-user"
                type="text"
                value={form.user}
                onChange={(e) => setForm({...form, user: e.target.value})} 
                className="w-full bg-black/40 border border-white/10 rounded-lg px-4 py-2 text-sm text-white focus:outline-none focus:border-[#10b981] transition-colors"
                placeholder="Padrão: root"
              />
            </div>
            <button 
              type="submit" 
              className="mt-2 bg-[#10b981]/20 hover:bg-[#10b981]/30 border border-[#10b981]/50 text-[#10b981] font-bold text-xs uppercase tracking-widest py-3 rounded-lg transition-all"
            >
              + Conectar VPS
            </button>
          </form>
        </div>

        <div className="glass-panel p-6 rounded-xl border border-white/5 bg-white/[0.02] col-span-2">
          <h2 className="text-sm font-bold tracking-widest text-[#737373] uppercase mb-6">Servidores Ativos</h2>
          {loading ? (
            <p className="text-sm text-[#737373]">Carregando...</p>
          ) : servers.length === 0 ? (
            <p className="text-sm text-[#737373]">Nenhum servidor cadastrado.</p>
          ) : (
            <div className="overflow-x-auto custom-scrollbar">
              <table className="w-full text-sm text-left border-collapse">
                <thead className="text-[10px] text-[#737373] uppercase tracking-widest bg-[#0c0c0e]">
                  <tr>
                    <th className="py-3 px-4 rounded-l">Status</th>
                    <th className="py-3 px-4">Nome</th>
                    <th className="py-3 px-4">IP</th>
                    <th className="py-3 px-4 text-right">CPU</th>
                    <th className="py-3 px-4 text-right">RAM</th>
                    <th className="py-3 px-4 text-right">Load</th>
                    <th className="py-3 px-4 text-right rounded-r">Ação</th>
                  </tr>
                </thead>
                <tbody>
                  {servers.map((s) => {
                    const live = liveStats[s.id];
                    const isOnline = !!live && live.online;
                    return (
                    <tr key={s.id} className="border-b border-white/[0.03] hover:bg-white/[0.05] transition-all">
                      <td className="py-4 px-4">
                        <div className="flex items-center gap-2">
                          <span className={`w-2 h-2 rounded-full ${isOnline ? 'bg-[#10b981] animate-pulse' : 'bg-red-500'}`}></span>
                          <span className={`text-[10px] ${isOnline ? 'text-[#10b981]' : 'text-red-500'} font-bold tracking-widest uppercase`}>
                            {isOnline ? 'Online' : 'Offline'}
                          </span>
                        </div>
                      </td>
                      <td className="py-4 px-4 font-medium text-white/90">{s.name}</td>
                      <td className="py-4 px-4 text-[#737373] font-mono">{s.host_ip}</td>
                      <td className="py-4 px-4 text-right font-medium text-white/90">
                        {isOnline ? `${live.cpu.toFixed(0)}%` : <span className="text-[#737373]">-</span>}
                      </td>
                      <td className="py-4 px-4 text-right text-white/90">
                        {isOnline && live.mem_total > 0
                          ? <span>{formatGB(live.mem_used)}<span className="text-[#737373] text-xs">/{formatGB(live.mem_total)}GB</span></span>
                          : <span className="text-[#737373]">-</span>}
                      </td>
                      <td className="py-4 px-4 text-right text-white/90">
                        {isOnline ? live.load1.toFixed(2) : <span className="text-[#737373]">-</span>}
                      </td>
                      <td className="py-4 px-4 text-right">
                        <button 
                          onClick={() => handleDelete(s)}
                          className="text-xs text-red-400/80 hover:text-red-400 hover:underline tracking-wider"
                        >
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

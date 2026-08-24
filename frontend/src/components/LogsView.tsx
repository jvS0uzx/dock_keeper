import { useState, useEffect, type FormEvent } from 'react';
import { Search, ScrollText, Server as ServerIcon, ShieldAlert, Box, Loader2 } from 'lucide-react';
import { api, type LogEntryRecord as LogEntry } from '../lib/api';
import { formatDateTime } from '../lib/format';
import Select, { type SelectOption } from './ui/Select';

interface ServerOption {
  id: string;
  name: string;
}

const RESULT_LIMIT = 500;

const LOG_SOURCES: SelectOption[] = [
  { value: '', label: 'Todas' },
  { value: 'auth', label: 'Autenticação' },
  { value: 'container', label: 'Container' },
];

const LogsView = () => {
  const [servers, setServers] = useState<ServerOption[]>([]);
  const [serverId, setServerId] = useState('');
  const [source, setSource] = useState('');
  const [q, setQ] = useState('');

  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [loading, setLoading] = useState(false);
  const [searched, setSearched] = useState(false);

  useEffect(() => {
    const controller = new AbortController();
    api.liveMetrics(controller.signal)
      .then((data) => setServers(data.servers.map(({ id, name }) => ({ id, name }))))
      .catch(() => {});
    return () => controller.abort();
  }, []);

  const serverName = (id: string) => servers.find((s) => s.id === id)?.name || id.slice(0, 8);

  const handleSearch = async (e?: FormEvent) => {
    if (e) e.preventDefault();
    setLoading(true);
    setSearched(true);
    try {
      const params: Record<string, string> = { limit: String(RESULT_LIMIT) };
      if (serverId) params.server_id = serverId;
      if (source) params.source = source;
      if (q) params.q = q;

      setLogs(await api.searchLogs(params));
    } catch (err) {
      console.error(err);
      setLogs([]);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="p-8">
      <h1 className="text-2xl font-light text-white mb-2 flex items-center gap-3">
        <ScrollText className="w-6 h-6 text-[#10b981]" />
        Busca de <span className="font-bold">Logs</span>
      </h1>
      <p className="text-[#737373] text-sm mb-8">
        Consulte o histórico de logs de autenticação e containers coletados dos servidores monitorados.
      </p>

      <form onSubmit={handleSearch} className="glass-panel p-6 rounded-xl border border-white/5 bg-white/[0.02] mb-6">
        <div className="grid grid-cols-1 md:grid-cols-4 gap-4">
          <div>
            <label htmlFor="logs-server" className="text-xs text-[#737373] block mb-1">Servidor</label>
            <Select
              id="logs-server"
              value={serverId}
              onChange={setServerId}
              options={[{ value: '', label: 'Todos' }, ...servers.map((s) => ({ value: s.id, label: s.name }))]}
            />
          </div>
          <div>
            <label htmlFor="logs-source" className="text-xs text-[#737373] block mb-1">Origem</label>
            <Select
              id="logs-source"
              value={source}
              onChange={setSource}
              options={LOG_SOURCES}
            />
          </div>
          <div className="md:col-span-2">
            <label htmlFor="logs-query" className="text-xs text-[#737373] block mb-1">Filtro de texto</label>
            <input
              id="logs-query"
              type="text"
              value={q}
              onChange={(e) => setQ(e.target.value)}
              className="w-full bg-black/40 border border-white/10 rounded-lg px-4 py-2 text-sm text-white focus:outline-none focus:border-[#10b981] transition-colors"
              placeholder="Ex: Failed password, error, restart..."
            />
          </div>
        </div>
        <button
          type="submit"
          disabled={loading}
          className="mt-4 flex items-center gap-2 bg-[#10b981]/20 hover:bg-[#10b981]/30 border border-[#10b981]/50 text-[#10b981] font-bold text-xs uppercase tracking-widest py-3 px-6 rounded-lg transition-all disabled:opacity-50"
        >
          {loading ? <Loader2 className="w-4 h-4 animate-spin" /> : <Search className="w-4 h-4" />}
          Buscar
        </button>
      </form>

      <div className="glass-panel p-6 rounded-xl border border-white/5 bg-white/[0.02]">
        <h2 className="text-sm font-bold tracking-widest text-[#737373] uppercase mb-6">
          Resultados {logs.length > 0 && <span className="text-[#10b981]">({logs.length})</span>}
        </h2>

        {loading ? (
          <p className="text-sm text-[#737373]">Carregando...</p>
        ) : !searched ? (
          <p className="text-sm text-[#737373]">Defina os filtros e clique em Buscar.</p>
        ) : logs.length === 0 ? (
          <p className="text-sm text-[#737373]">Nenhum log encontrado para os filtros informados.</p>
        ) : (
          <div className="overflow-x-auto custom-scrollbar">
            <table className="w-full text-xs text-left border-collapse font-mono">
              <thead className="text-[10px] text-[#737373] uppercase tracking-widest bg-[#0c0c0e]">
                <tr>
                  <th className="py-3 px-4 rounded-l whitespace-nowrap">Timestamp</th>
                  <th className="py-3 px-4 whitespace-nowrap">Servidor</th>
                  <th className="py-3 px-4 whitespace-nowrap">Origem</th>
                  <th className="py-3 px-4 rounded-r">Linha</th>
                </tr>
              </thead>
              <tbody>
                {logs.map((l) => (
                  <tr key={l.id} className="border-b border-white/[0.03] hover:bg-white/[0.05] transition-all align-top">
                    <td className="py-2 px-4 text-[#737373] whitespace-nowrap">{formatDateTime(l.timestamp)}</td>
                    <td className="py-2 px-4 text-white/70 whitespace-nowrap flex items-center gap-1">
                      <ServerIcon className="w-3 h-3 text-[#737373]" />
                      {serverName(l.server_id)}
                    </td>
                    <td className="py-2 px-4 whitespace-nowrap">
                      <span className="inline-flex items-center gap-1 text-[10px] uppercase tracking-wider">
                        {l.source === 'auth' ? (
                          <><ShieldAlert className="w-3 h-3 text-amber-400" /><span className="text-amber-400">auth</span></>
                        ) : (
                          <><Box className="w-3 h-3 text-[#10b981]" /><span className="text-[#10b981]">{l.container || 'container'}</span></>
                        )}
                      </span>
                    </td>
                    <td className="py-2 px-4 text-white/90 break-all">{l.line}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  );
};

export default LogsView;

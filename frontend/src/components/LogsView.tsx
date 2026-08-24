import { useState, useEffect, type FormEvent } from 'react';
import { Search, Server as ServerIcon, ShieldAlert, Box, Loader2 } from 'lucide-react';
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
    <div className="p-8 anim-rise">
      <div className="page-header">
        <div>
          <h1 className="page-title">Busca de logs</h1>
          <p className="page-desc">
            Histórico de logs de autenticação e de containers coletados dos servidores monitorados.
          </p>
        </div>
      </div>

      <form onSubmit={handleSearch} className="panel p-5 mb-6">
        <div className="grid grid-cols-1 md:grid-cols-4 gap-4">
          <div>
            <label htmlFor="logs-server" className="eyebrow block mb-1.5">Servidor</label>
            <Select
              id="logs-server"
              value={serverId}
              onChange={setServerId}
              options={[{ value: '', label: 'Todos' }, ...servers.map((s) => ({ value: s.id, label: s.name }))]}
            />
          </div>
          <div>
            <label htmlFor="logs-source" className="eyebrow block mb-1.5">Origem</label>
            <Select
              id="logs-source"
              value={source}
              onChange={setSource}
              options={LOG_SOURCES}
            />
          </div>
          <div className="md:col-span-2">
            <label htmlFor="logs-query" className="eyebrow block mb-1.5">Filtro de texto</label>
            <input
              id="logs-query"
              type="text"
              value={q}
              onChange={(e) => setQ(e.target.value)}
              className="input-base w-full"
              placeholder="Ex: Failed password, error, restart..."
            />
          </div>
        </div>
        <button type="submit" disabled={loading} className="btn btn-primary mt-4 disabled:opacity-50">
          {loading ? <Loader2 size={16} strokeWidth={1.75} className="animate-spin" /> : <Search size={16} strokeWidth={1.75} />}
          Buscar
        </button>
      </form>

      <div className="panel p-5">
        <h2 className="eyebrow mb-4">
          Resultados{logs.length > 0 && <span className="mono-data text-text-mut normal-case tracking-normal"> · {logs.length}</span>}
        </h2>

        {loading ? (
          <p className="text-sm text-text-mut">Carregando...</p>
        ) : !searched ? (
          <p className="text-sm text-text-mut">Defina os filtros e clique em Buscar.</p>
        ) : logs.length === 0 ? (
          <p className="text-sm text-text-mut">Nenhum log encontrado para os filtros informados.</p>
        ) : (
          <div className="overflow-x-auto custom-scrollbar">
            <table className="table-base min-w-[720px]">
              <thead>
                <tr>
                  <th className="whitespace-nowrap">Timestamp</th>
                  <th className="whitespace-nowrap">Servidor</th>
                  <th className="whitespace-nowrap">Origem</th>
                  <th>Linha</th>
                </tr>
              </thead>
              <tbody>
                {logs.map((l) => (
                  <tr key={l.id} className="align-top">
                    <td className="mono-data text-text-faint whitespace-nowrap">{formatDateTime(l.timestamp)}</td>
                    <td className="whitespace-nowrap">
                      <span className="inline-flex items-center gap-1.5 text-text-mut">
                        <ServerIcon size={13} strokeWidth={1.75} className="text-text-faint" />
                        {serverName(l.server_id)}
                      </span>
                    </td>
                    <td className="whitespace-nowrap">
                      {l.source === 'auth' ? (
                        <span className="inline-flex items-center gap-1.5 text-warn text-xs">
                          <ShieldAlert size={13} strokeWidth={1.75} />
                          auth
                        </span>
                      ) : (
                        <span className="inline-flex items-center gap-1.5 text-text-mut text-xs">
                          <Box size={13} strokeWidth={1.75} className="text-text-faint" />
                          <span className="mono-data">{l.container || 'container'}</span>
                        </span>
                      )}
                    </td>
                    <td className="mono-data text-text leading-relaxed break-all selectable">{l.line}</td>
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

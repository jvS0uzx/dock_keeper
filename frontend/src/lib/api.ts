import { API_TOKEN, apiBase } from '../config';
import {
  SESSION_EXPIRED_EVENT,
  clearSession,
  loadSession,
  type Role,
  type SessionInfo,
  type SiteAccess,
} from './session';

// Espelha api.ServerLiveStat do backend (/api/metrics/live).
export interface ServerLiveStat {
  id: string;
  host_ip: string;
  name: string;
  uptime: number;
  disk_used: number;
  disk_total: number;
  cpu: number;
  mem_used: number;
  mem_total: number;
  load1: number;
  online: boolean;
  // Nulo quando a fonte de coleta desta máquina não mede o valor, diferente de
  // zero, que é leitura real. Ver achados 4 e 5 do QA do fluxo de métricas.
  //
  // O campo já se chamou latency_ms. O número nunca foi latência de rede: é o
  // handshake SSH inteiro (TCP + troca de chaves), uma ordem de grandeza acima
  // do RTT. Só existe em máquina coletada por SSH.
  ssh_handshake_ms: number | null;
  kind: string;
  site_id: number | null;
  os: string;
  platform: string;
  arch: string;
  last_user: string;
  agent_version: string;
  // Nulo quando a máquina não tem sensor legível pela fonte que a coleta.
  temperature_c: number | null;
  collect_nginx: boolean;
}

// Espelha api.ContainerLiveStat do backend.
export interface ContainerLiveStat {
  server_id: string;
  docker_id: string;
  name: string;
  project: string;
  state: string;
  status: string;
  cpu: number;
  mem_used: number;
  mem_limit: number;
}

// Espelha api.LbStat do backend.
export interface LbStat {
  upstream_addr: string;
  server_name: string;
  status: string;
  requests_count: number;
}

export interface LiveMetrics {
  servers: ServerLiveStat[];
  containers: ContainerLiveStat[];
  load_balancing: LbStat[];
}

export interface HistoryPoint {
  ts: string;
  value: number;
}

export interface ServerRecord {
  id: string;
  name: string;
  host_ip: string;
  user: string;
  port: number;
  created_at: string;
}

export interface DomainRecord {
  id: number;
  domain: string;
  server_id: string;
  valid: boolean;
  issuer: string;
  days_left: number;
  error_msg: string;
  /**
   * Motivo mais grave da invalidez, legível por máquina: expirado,
   * ainda_nao_valido, hostname_divergente, autoassinado, cadeia_nao_confiavel,
   * sem_certificado, handshake. Vazio quando o certificado está válido.
   * error_msg continua listando TODOS os problemas em texto.
   */
  invalid_reason: string;
  last_check: string | null;
}

export interface AlertRuleRecord {
  id: number;
  name: string;
  /** "*" para todos, ou o id de um servidor específico. */
  target: string;
  /** Quando preenchido, a regra vale para todas as máquinas da unidade. */
  target_site_id: number | null;
  metric: string;
  operator: string;
  threshold: number;
  enabled: boolean;
  severity: string;
  /** Segundos que a condição precisa se manter antes de disparar. 0 = imediato. */
  for_duration_sec: number;
  last_fired: string | null;
}

/** Domínio observado no access log do Nginx. */
export interface DiscoveredDomain {
  domain: string;
  monitored: boolean;
  sample_reqs: number;
}

/**
 * Corpo do cadastro de regra. Só `target_site_id` OU `target` faz sentido: o
 * backend recusa os dois preenchidos e normaliza `target` para "*" quando a
 * regra é por unidade.
 */
export interface AlertRuleInput {
  name: string;
  target: string;
  target_site_id?: number | null;
  metric: string;
  operator: string;
  threshold: number;
  enabled: boolean;
  severity?: string;
  for_duration_sec?: number;
}

export interface LogEntryRecord {
  id: number;
  server_id: string;
  source: string;
  container: string;
  line: string;
  timestamp: string;
}

export interface NetworkHostView {
  ip: string;
  hostname: string;
  mac: string;
  open_ports: string[];
  first_seen: string;
  last_seen: string;
  online: boolean;
  monitored: boolean;
  kind: string;
  device_type: string;
  /** true = tipo fixado pelo operador; a varredura não sobrescreve. */
  device_type_locked: boolean;
  site_id: number | null;
  /** true = unidade fixada pelo operador; o coletor não reverte. */
  site_locked: boolean;
  floor: string;
  sector: string;
  room: string;
  rack: string;
  asset_tag: string;
  owner: string;
  notes: string;
}

/** Campos cadastrais editáveis; o resto vem da varredura. */
export type HostInventoryPatch = Partial<
  Pick<NetworkHostView, 'floor' | 'sector' | 'room' | 'rack' | 'asset_tag' | 'owner' | 'notes' | 'device_type'>
> & { site_id?: number | null };

export interface Site {
  id: number;
  name: string;
  code: string;
  address: string;
  latitude: number;
  longitude: number;
  created_at: string;
}

export interface FloorPlanPin {
  id: number;
  host_ip: string;
  label: string;
  x: number;
  y: number;
  target_plan_id: number | null;
  hostname: string;
  device_type: string;
  online: boolean;
  monitored: boolean;
  known: boolean;
  /** Vazio quando a máquina ainda não reporta métricas. */
  server_id: string;
}

export interface FloorPlan {
  id: number;
  site_id: number | null;
  name: string;
  content_type: string;
  width: number;
  height: number;
  created_at: string;
  pins: FloorPlanPin[];
}

/** Pin sem o estado resolvido — é o que o painel envia ao gravar. */
export interface FloorPlanPinInput {
  host_ip: string;
  label: string;
  x: number;
  y: number;
  target_plan_id: number | null;
}

export interface NetworkInventory {
  hosts: NetworkHostView[];
  total: number;
  online: number;
  monitored: number;
  last_scan: string | null;
  scan_active: boolean;
}

export interface PortInfo {
  protocol: string;
  state: string;
  port: string;
  process: string;
}

export type HistoryMetric = 'cpu' | 'mem' | 'disk' | 'load' | 'temperature' | 'latency';
export type HistoryRange = '1h' | '6h' | '24h' | '7d';
export type ContainerAction = 'start' | 'stop' | 'restart';

/** Usuário do painel, como devolvido por GET /api/users (admin). */
export interface UserRecord {
  id: number;
  username: string;
  role: Role;
  active: boolean;
  last_login: string | null;
  created_at: string;
  accesses: SiteAccess[];
}

/** Resposta de GET /api/auth/me: quem está autenticado e com qual credencial. */
export interface MeInfo {
  username: string;
  role: Role;
  kind: 'user' | 'token';
  accesses: SiteAccess[];
}

/**
 * Credencial da requisição: a sessão da pessoa vence o token de máquina.
 * Devolve também se a credencial veio de sessão, para o tratamento de 401
 * saber se deve derrubá-la.
 *
 * Em produção o API_TOKEN é sempre vazio (ver config.ts), então sem sessão a
 * requisição sai sem credencial e volta 401 — de propósito.
 */
const authHeader = (headers: Headers): { usedSession: boolean } => {
  const session = loadSession();
  if (session) {
    headers.set('Authorization', `Bearer ${session.token}`);
    return { usedSession: true };
  }
  if (API_TOKEN) headers.set('Authorization', `Bearer ${API_TOKEN}`);
  return { usedSession: false };
};

/**
 * 401 numa requisição feita com sessão significa sessão vencida ou revogada:
 * limpa e avisa o App para voltar ao login. Com token de máquina não há o que
 * derrubar — o erro sobe normalmente.
 */
const handleUnauthorized = (usedSession: boolean) => {
  if (!usedSession) return;
  clearSession();
  window.dispatchEvent(new CustomEvent(SESSION_EXPIRED_EVENT));
};

/**
 * Faz a requisição e normaliza a resposta. Concentra aqui a validação de
 * fronteira: token, status HTTP e formato do corpo só são tratados neste ponto.
 */
const request = async <T>(path: string, init: RequestInit = {}): Promise<T> => {
  const headers = new Headers(init.headers);
  const { usedSession } = authHeader(headers);
  if (init.body) headers.set('Content-Type', 'application/json');

  const res = await fetch(apiBase() + path, { ...init, headers });
  if (!res.ok) {
    if (res.status === 401) handleUnauthorized(usedSession);
    throw new Error((await res.text()) || `HTTP ${res.status}`);
  }
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
};

const send = (method: string, body?: unknown): RequestInit => ({
  method,
  ...(body === undefined ? {} : { body: JSON.stringify(body) }),
});

/**
 * Abre um EventSource autenticado.
 *
 * EventSource não permite definir cabeçalhos, então o segredo precisa ir na
 * URL — e URL vai parar no access log do Nginx e no histórico do navegador.
 * Por isso não mandamos o API_TOKEN: pedimos antes um ticket de uso único,
 * válido por 30s, que só serve para abrir este stream.
 */
export const openStream = async (
  path: string,
  params: Record<string, string>,
): Promise<EventSource> => {
  const { ticket } = await request<{ ticket: string }>('/api/stream-ticket', send('POST'));
  const query = new URLSearchParams({ ...params, ticket });
  return new EventSource(`${apiBase()}${path}?${query}`);
};

const asArray = <T>(data: unknown): T[] => (Array.isArray(data) ? (data as T[]) : []);

// Espelha database.AuditLog do backend (/api/audit).
export interface AuditEntry {
  id: number;
  at: string;
  actor_user_id: number | null;
  actor_username: string;
  actor_role: string;
  source_ip: string;
  user_agent: string;
  action: string;
  target_type: string;
  target_id: string;
  target_label: string;
  site_id: number | null;
  /** ok, denied ou error. */
  result: string;
  /** JSON serializado montado por allowlist no backend. */
  detail: string;
}

export interface AuditPage {
  items: AuditEntry[];
  total: number;
  limit: number;
  offset: number;
}

export interface AuditQuery {
  actor?: string;
  action?: string;
  result?: string;
  site_id?: string;
  /** RFC3339; o backend recusa qualquer outro formato com 400. */
  from?: string;
  to?: string;
  limit?: number;
  offset?: number;
}

export const api = {
  async audit(query: AuditQuery, signal?: AbortSignal): Promise<AuditPage> {
    const params = new URLSearchParams();
    for (const [key, value] of Object.entries(query)) {
      if (value !== undefined && value !== '') params.set(key, String(value));
    }
    const qs = params.toString();
    const data = await request<Partial<AuditPage>>(`/api/audit${qs ? `?${qs}` : ''}`, { signal });
    return {
      items: data.items ?? [],
      total: data.total ?? 0,
      limit: data.limit ?? 0,
      offset: data.offset ?? 0,
    };
  },

  async liveMetrics(signal?: AbortSignal): Promise<LiveMetrics> {
    const data = await request<Partial<LiveMetrics>>('/api/metrics/live', { signal });
    return {
      servers: data.servers ?? [],
      containers: data.containers ?? [],
      load_balancing: data.load_balancing ?? [],
    };
  },

  async history(
    serverId: string,
    metric: HistoryMetric,
    range: HistoryRange,
    signal?: AbortSignal,
  ): Promise<HistoryPoint[]> {
    const params = new URLSearchParams({ server_id: serverId, metric, range });
    return asArray<HistoryPoint>(await request(`/api/metrics/history?${params}`, { signal }));
  },

  containerAction(serverId: string, containerName: string, action: ContainerAction) {
    return request<unknown>(
      '/api/containers/action',
      send('POST', { server_id: serverId, container_name: containerName, action }),
    );
  },

  async servers(): Promise<ServerRecord[]> {
    return asArray<ServerRecord>(await request('/api/servers'));
  },

  createServer(body: { name: string; host_ip: string; user: string }) {
    return request<ServerRecord>('/api/servers', send('POST', body));
  },

  deleteServer(id: string) {
    return request<unknown>(`/api/servers?id=${encodeURIComponent(id)}`, send('DELETE'));
  },

  async domains(): Promise<DomainRecord[]> {
    return asArray<DomainRecord>(await request('/api/ssl/domains'));
  },

  createDomain(domain: string) {
    return request<DomainRecord>('/api/ssl/domains', send('POST', { domain, server_id: '' }));
  },

  deleteDomain(id: number) {
    return request<unknown>(`/api/ssl/domains?id=${id}`, send('DELETE'));
  },

  recheckDomain(id: number) {
    return request<DomainRecord>(`/api/ssl/recheck?id=${id}`, send('POST'));
  },

  /** Domínios que o Nginx atendeu, com marcação de quais já são monitorados. */
  async discoverDomains(signal?: AbortSignal): Promise<DiscoveredDomain[]> {
    return asArray<DiscoveredDomain>(await request('/api/ssl/discover', { signal }));
  },

  importDomains(domains: string[]) {
    return request<{ imported: number }>('/api/ssl/import', send('POST', { domains }));
  },

  recheckAllDomains() {
    return request<unknown>('/api/ssl/recheck-all', send('POST'));
  },

  async alertRules(): Promise<AlertRuleRecord[]> {
    return asArray<AlertRuleRecord>(await request('/api/alerts/rules'));
  },

  createAlertRule(body: AlertRuleInput) {
    return request<AlertRuleRecord>('/api/alerts/rules', send('POST', body));
  },

  deleteAlertRule(id: number) {
    return request<unknown>(`/api/alerts/rules?id=${id}`, send('DELETE'));
  },

  toggleAlertRule(id: number, enabled: boolean) {
    return request<unknown>(`/api/alerts/rules?id=${id}`, send('PATCH', { enabled }));
  },

  async searchLogs(params: Record<string, string>): Promise<LogEntryRecord[]> {
    const query = new URLSearchParams(params);
    return asArray<LogEntryRecord>(await request(`/api/logs/search?${query}`));
  },

  /**
   * siteId recorta o inventário na origem. A planta baixa precisa disso: a
   * paleta de máquinas arrastáveis oferecia hosts de outras unidades, que nunca
   * resolvem contra uma planta desta. Filtrar no cliente traria o parque inteiro
   * para descartar no navegador.
   */
  async networkHosts(signal?: AbortSignal, siteId?: number): Promise<NetworkInventory> {
    const query = siteId ? `?site_id=${siteId}` : '';
    const data = await request<Partial<NetworkInventory>>(`/api/network/hosts${query}`, { signal });
    return {
      hosts: data.hosts ?? [],
      total: data.total ?? 0,
      online: data.online ?? 0,
      monitored: data.monitored ?? 0,
      last_scan: data.last_scan ?? null,
      scan_active: data.scan_active ?? false,
    };
  },

  scanNetwork() {
    return request<unknown>('/api/network/scan', send('POST'));
  },

  updateHost(ip: string, patch: HostInventoryPatch) {
    return request<NetworkHostView>(`/api/network/host?ip=${encodeURIComponent(ip)}`, send('PATCH', patch));
  },

  async sites(): Promise<Site[]> {
    return asArray<Site>(await request('/api/sites'));
  },

  createSite(body: { name: string; code: string; address?: string }) {
    return request<Site>('/api/sites', send('POST', body));
  },

  deleteSite(id: number) {
    return request<unknown>(`/api/sites?id=${id}`, send('DELETE'));
  },

  async floorPlans(): Promise<FloorPlan[]> {
    return asArray<FloorPlan>(await request('/api/floorplans'));
  },

  floorPlan(id: number, signal?: AbortSignal) {
    return request<FloorPlan>(`/api/floorplans/${id}`, { signal });
  },

  deleteFloorPlan(id: number) {
    return request<unknown>(`/api/floorplans/${id}`, send('DELETE'));
  },

  savePins(planId: number, pins: FloorPlanPinInput[]) {
    return request<{ pins: FloorPlanPin[] }>(`/api/floorplans/${planId}/pins`, send('PUT', { pins }));
  },

  /** Upload multipart: o Content-Type é definido pelo browser, com o boundary. */
  async uploadFloorPlan(name: string, image: File, siteId?: number | null): Promise<FloorPlan> {
    const form = new FormData();
    form.append('name', name);
    form.append('image', image);
    if (siteId) form.append('site_id', String(siteId));

    const headers = new Headers();
    const { usedSession } = authHeader(headers);

    const res = await fetch(`${apiBase()}/api/floorplans`, { method: 'POST', body: form, headers });
    if (!res.ok) {
      if (res.status === 401) handleUnauthorized(usedSession);
      throw new Error((await res.text()) || `HTTP ${res.status}`);
    }
    return (await res.json()) as FloorPlan;
  },

  /**
   * Baixa a imagem da planta como object URL.
   *
   * <img src> não envia cabeçalho, então buscar com fetch autenticado é o que
   * evita o token na URL. Quem chama precisa revogar o URL ao descartar.
   */
  async floorPlanImageUrl(id: number, signal?: AbortSignal): Promise<string> {
    const headers = new Headers();
    const { usedSession } = authHeader(headers);

    const res = await fetch(`${apiBase()}/api/floorplans/${id}/image`, { headers, signal });
    if (!res.ok) {
      if (res.status === 401) handleUnauthorized(usedSession);
      throw new Error(`HTTP ${res.status}`);
    }
    return URL.createObjectURL(await res.blob());
  },

  async securityRadar(serverId: string, signal?: AbortSignal): Promise<PortInfo[]> {
    const params = new URLSearchParams({ server_id: serverId });
    return asArray<PortInfo>(await request(`/api/security/radar?${params}`, { signal }));
  },

  login(username: string, password: string) {
    return request<SessionInfo>('/api/auth/login', send('POST', { username, password }));
  },

  logout() {
    return request<unknown>('/api/auth/logout', send('POST'));
  },

  me(signal?: AbortSignal) {
    return request<MeInfo>('/api/auth/me', { signal });
  },

  async users(): Promise<UserRecord[]> {
    const list = asArray<UserRecord>(await request('/api/users'));
    // O backend pode omitir accesses vazio; a UI itera sem checar.
    return list.map((u) => ({ ...u, accesses: u.accesses ?? [] }));
  },

  createUser(body: { username: string; password: string; role: Role; accesses?: SiteAccess[] }) {
    return request<UserRecord>('/api/users', send('POST', body));
  },

  updateUser(
    id: number,
    patch: { password?: string; role?: Role; active?: boolean; accesses?: SiteAccess[] },
  ) {
    return request<unknown>(`/api/users?id=${id}`, send('PATCH', patch));
  },

  deleteUser(id: number) {
    return request<unknown>(`/api/users?id=${id}`, send('DELETE'));
  },
};

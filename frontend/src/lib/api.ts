import { API_TOKEN, apiBase } from '../config';
import {
  SESSION_EXPIRED_EVENT,
  clearSession,
  loadSession,
  type Role,
  type SessionInfo,
  type SiteAccess,
} from './session';

export type NginxEstado = 'desconhecido' | 'ausente' | 'inativo' | 'sem_upstream' | 'candidato';

export type NginxPapel = 'nenhum' | 'principal' | 'reserva';

export interface NginxUpstreamLink {
  server_id: string;
  bloco: string;
  destino: string;
  observado_em: string;
}

export interface ServerLiveStat {
  id: string;
  host_ip: string;
  name: string;
  uptime: number;
  disk_used: number;
  disk_total: number;
  cpu: number | null;
  mem_used: number;
  mem_total: number;
  load1: number | null;
  online: boolean;
  ssh_handshake_ms: number | null;
  kind: string;
  site_id: number | null;
  os: string;
  platform: string;
  arch: string;
  last_user: string;
  agent_version: string;
  temperature_c: number | null;
  collect_nginx: boolean;
  addresses: string[];
  absence_alert?: boolean;
  behind_lb?: boolean;
  behind_lb_origem?: 'manual' | 'trafego' | 'configuracao' | 'nenhum';
  nginx_estado?: NginxEstado;
  nginx_motivo?: string;
  nginx_papel?: NginxPapel;
  nginx_checado_em?: string | null;
  net_rx_bps: number | null;
  net_tx_bps: number | null;
  rtt_ms: number | null;
}

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
  health?: string | null;
  restart_count?: number | null;
  oom_killed?: boolean | null;
}

export interface LbStat {
  upstream_addr: string;
  server_name: string;
  status: string;
  requests_count: number;
  server_id?: string;
}

export interface LiveMetrics {
  servers: ServerLiveStat[];
  containers: ContainerLiveStat[];
  load_balancing: LbStat[];
  lb_window_sec: number | null;
  nginx_topologia?: NginxUpstreamLink[];
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
  aliases?: string[] | null;
  addresses?: string[] | null;
  absence_alert?: boolean;
}

export interface DomainRecord {
  id: number;
  domain: string;
  server_id: string;
  valid: boolean;
  issuer: string;
  days_left: number;
  error_msg: string;
  invalid_reason: string;
  last_check: string | null;
}

export interface AlertRuleRecord {
  id: number;
  name: string;
  target: string;
  target_site_id: number | null;
  metric: string;
  operator: string;
  threshold: number;
  enabled: boolean;
  severity: string;
  for_duration_sec: number;
  last_fired: string | null;
}

export interface DiscoveredDomain {
  domain: string;
  monitored: boolean;
  sample_reqs: number;
}

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
  device_type_locked: boolean;
  site_id: number | null;
  site_locked: boolean;
  floor: string;
  sector: string;
  room: string;
  rack: string;
  asset_tag: string;
  owner: string;
  notes: string;
}

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

export interface MetricaDoCatalogo {
  nome: string;
  rotulo: string;
  unidade: string;
  tem_tendencia: boolean;
  escopo: string;
  em_regra: boolean;
}

export type HistoryRange = '1h' | '6h' | '24h' | '7d' | '30d' | '90d';

export interface CustomWindow {
  from: string;
  to?: string;
}

export type HistoryWindow = HistoryRange | CustomWindow;

export interface DashboardPanelInput {
  title: string;
  server_id: string;
  metric: string;
  range: HistoryRange;
  width: 1 | 2;
}

export interface DashboardPanel extends DashboardPanelInput {
  id: number;
  position: number;
}

export interface Dashboard {
  id: number;
  name: string;
  panels: DashboardPanel[];
  updated_at: string;
}

export interface DashboardInput {
  name: string;
  panels: DashboardPanelInput[];
}

export interface Annotation {
  id: number;
  server_id: string | null;
  at: string;
  text: string;
  author: string;
  created_at: string;
}

export interface AnnotationInput {
  server_id: string | null;
  at?: string;
  text: string;
}
export type ContainerAction = 'start' | 'stop' | 'restart';

export interface UserRecord {
  id: number;
  username: string;
  nome: string | null;
  email: string | null;
  role: Role;
  active: boolean;
  last_login: string | null;
  created_at: string;
  accesses: SiteAccess[];
}

export type DeviceKind = 'agent' | 'collector';

export interface DeviceRecord {
  device_id: string;
  site_id: number;
  kind: DeviceKind;
  machine_id: string;
  hostname: string;
  created_at: string;
  last_seen_at: string | null;
  revoked_at: string | null;
}

export interface EnrollToken {
  enrollment_token: string;
  site_id: number;
  kind: DeviceKind;
  expires_at: string;
}

export interface MeInfo {
  username: string;
  nome?: string | null;
  email?: string | null;
  role: Role;
  kind: 'user' | 'token';
  accesses: SiteAccess[];
}

const authHeader = (headers: Headers): { usedSession: boolean } => {
  const session = loadSession();
  if (session) {
    headers.set('Authorization', `Bearer ${session.token}`);
    return { usedSession: true };
  }
  if (API_TOKEN) headers.set('Authorization', `Bearer ${API_TOKEN}`);
  return { usedSession: false };
};

const handleUnauthorized = (usedSession: boolean) => {
  if (!usedSession) return;
  clearSession();
  window.dispatchEvent(new CustomEvent(SESSION_EXPIRED_EVENT));
};

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

export const openStream = async (
  path: string,
  params: Record<string, string>,
): Promise<EventSource> => {
  const { ticket } = await request<{ ticket: string }>('/api/stream-ticket', send('POST'));
  const query = new URLSearchParams({ ...params, ticket });
  return new EventSource(`${apiBase()}${path}?${query}`);
};

const asArray = <T>(data: unknown): T[] => (Array.isArray(data) ? (data as T[]) : []);

export const apiErrorMessage = (err: unknown, fallback: string): string => {
  try {
    const message = (JSON.parse((err as Error).message) as { error?: unknown }).error;
    return typeof message === 'string' && message ? message : fallback;
  } catch {
    return fallback;
  }
};

export type AlertStatus = 'open' | 'acked' | 'resolved';
export type AlertDelivery = 'pendente' | 'enviado' | 'falhou' | 'sem_canal' | 'dispensado';

export interface AlertItem {
  id: number;
  key: string;
  severity: string;
  text: string;
  status: AlertStatus;
  server_id: string | null;
  site_id: number | null;
  rule_id: number | null;
  created_at: string;
  acked_at: string | null;
  acked_by: number | null;
  resolved_at: string | null;
  delivery: AlertDelivery;
  attempts: number;
  next_attempt_at: string | null;
  last_attempt_at: string | null;
  last_error: string;
  renotify_count: number;
  last_notified_at: string | null;
  last_seen_at: string | null;
  server_name: string | null;
  site_name: string | null;
  alvo_tipo: string | null;
  alvo_id: string | null;
  alvo_nome: string | null;
  metrica: string | null;
  valor: number | null;
  limiar: number | null;
  unidade: string | null;
}

export interface Readiness {
  status: string;
  db?: string;
  degradado?: string[];
  alertas?: string;
  alertas_detalhe?: string;
  alertas_falhos?: number;
  alertas_sem_canal?: number;
  logs_descartados?: number;
}

export interface AlertSummary {
  open: number;
  acked: number;
  falhou: number;
}

export interface AlertQuery {
  status?: AlertStatus | 'all';
  site_id?: number;
  limit?: number;
  from?: string;
  to?: string;
}

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
  result: string;
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

  readiness(signal?: AbortSignal) {
    return request<Readiness>('/api/readyz', { signal });
  },

  async alerts(query: AlertQuery = {}, signal?: AbortSignal): Promise<AlertItem[]> {
    const params = new URLSearchParams();
    for (const [chave, valor] of Object.entries(query)) {
      if (valor !== undefined && valor !== '') params.set(chave, String(valor));
    }
    const qs = params.toString();
    return asArray<AlertItem>(await request<unknown>(`/api/alerts${qs ? `?${qs}` : ''}`, { signal }));
  },

  async alertsSummary(siteId: number | null = null, signal?: AbortSignal): Promise<AlertSummary> {
    const qs = siteId === null ? '' : `?site_id=${siteId}`;
    const data = await request<Partial<AlertSummary>>(`/api/alerts/summary${qs}`, { signal });
    return { open: data.open ?? 0, acked: data.acked ?? 0, falhou: data.falhou ?? 0 };
  },

  ackAlert(id: number) {
    return request<AlertItem>(`/api/alerts/ack?id=${id}`, send('POST'));
  },

  resolveAlert(id: number) {
    return request<AlertItem>(`/api/alerts/resolve?id=${id}`, send('POST'));
  },

  async liveMetrics(signal?: AbortSignal): Promise<LiveMetrics> {
    const data = await request<Partial<LiveMetrics>>('/api/metrics/live', { signal });
    return {
      servers: (data.servers ?? []).map((s) => ({
        ...s,
        cpu: s.cpu ?? null,
        load1: s.load1 ?? null,
        addresses: s.addresses ?? [],
        net_rx_bps: s.net_rx_bps ?? null,
        net_tx_bps: s.net_tx_bps ?? null,
        rtt_ms: s.rtt_ms ?? null,
      })),
      containers: data.containers ?? [],
      load_balancing: data.load_balancing ?? [],
      lb_window_sec: typeof data.lb_window_sec === 'number' && data.lb_window_sec > 0 ? data.lb_window_sec : null,
    };
  },

  async metricsCatalog(signal?: AbortSignal): Promise<MetricaDoCatalogo[]> {
    return asArray<MetricaDoCatalogo>(await request('/api/metrics/catalogo', { signal }));
  },

  async history(
    serverId: string,
    metric: string,
    janela: HistoryWindow,
    signal?: AbortSignal,
  ): Promise<HistoryPoint[]> {
    const params = new URLSearchParams({ server_id: serverId, metric });
    if (typeof janela === 'string') {
      params.set('range', janela);
    } else {
      params.set('from', janela.from);
      if (janela.to) params.set('to', janela.to);
    }
    return asArray<HistoryPoint>(await request(`/api/metrics/history?${params}`, { signal }));
  },

  async dashboards(signal?: AbortSignal): Promise<Dashboard[]> {
    const list = asArray<Dashboard>(await request('/api/dashboards', { signal }));
    return list.map((d) => ({ ...d, panels: [...(d.panels ?? [])].sort((a, b) => a.position - b.position) }));
  },

  createDashboard(body: DashboardInput) {
    return request<Dashboard>('/api/dashboards', send('POST', body));
  },

  updateDashboard(id: number, body: DashboardInput) {
    return request<Dashboard>(`/api/dashboards?id=${id}`, send('PUT', body));
  },

  deleteDashboard(id: number) {
    return request<unknown>(`/api/dashboards?id=${id}`, send('DELETE'));
  },

  async annotations(
    query: { server_id?: string; from: string; to: string },
    signal?: AbortSignal,
  ): Promise<Annotation[]> {
    const params = new URLSearchParams({ from: query.from, to: query.to });
    if (query.server_id) params.set('server_id', query.server_id);
    return asArray<Annotation>(await request(`/api/annotations?${params}`, { signal }));
  },

  createAnnotation(body: AnnotationInput) {
    return request<Annotation>('/api/annotations', send('POST', body));
  },

  deleteAnnotation(id: number) {
    return request<unknown>(`/api/annotations?id=${id}`, send('DELETE'));
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

  updateServerAliases(id: string, aliases: string[]) {
    return request<ServerRecord>(`/api/servers?id=${encodeURIComponent(id)}`, send('PATCH', { aliases }));
  },

  setServerAbsenceAlert(id: string, absence_alert: boolean) {
    return request<ServerRecord>(`/api/servers?id=${encodeURIComponent(id)}`, send('PATCH', { absence_alert }));
  },

  setServerBehindLb(id: string, behind_lb: boolean | null) {
    return request<ServerRecord>(`/api/servers?id=${encodeURIComponent(id)}`, send('PATCH', { behind_lb }));
  },

  setServerCollectNginx(id: string, collect_nginx: boolean) {
    return request<ServerRecord>(`/api/servers?id=${encodeURIComponent(id)}`, send('PATCH', { collect_nginx }));
  },

  renameServer(id: string, name: string) {
    return request<ServerRecord>(`/api/servers?id=${encodeURIComponent(id)}`, send('PATCH', { name }));
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

  async devices(): Promise<DeviceRecord[]> {
    return asArray<DeviceRecord>(await request('/api/devices'));
  },

  createEnrollToken(body: { kind: DeviceKind; site_id: number }) {
    return request<EnrollToken>('/api/enroll/tokens', send('POST', body));
  },

  revokeDevice(deviceId: string) {
    return request<unknown>(`/api/devices?device_id=${encodeURIComponent(deviceId)}`, send('DELETE'));
  },

  async users(): Promise<UserRecord[]> {
    const list = asArray<UserRecord>(await request('/api/users'));
    return list.map((u) => ({
      ...u,
      nome: u.nome ?? null,
      email: u.email ?? null,
      accesses: u.accesses ?? [],
    }));
  },

  createUser(body: {
    username: string;
    password: string;
    role: Role;
    nome?: string;
    email?: string;
    accesses?: SiteAccess[];
  }) {
    return request<UserRecord>('/api/users', send('POST', body));
  },

  updateUser(
    id: number,
    patch: {
      password?: string;
      role?: Role;
      active?: boolean;
      nome?: string;
      email?: string;
      accesses?: SiteAccess[];
    },
  ) {
    return request<unknown>(`/api/users?id=${id}`, send('PATCH', patch));
  },

  deleteUser(id: number) {
    return request<unknown>(`/api/users?id=${id}`, send('DELETE'));
  },
};

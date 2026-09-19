import type { ContainerLiveStat } from './api';
import { compararNumeros } from './agregado';

export type ContainerSortField = 'name' | 'cpu' | 'mem';
export type SortDirection = 'asc' | 'desc';

export interface ContainerPrefs {
  hideStopped: boolean;
  sortBy: ContainerSortField;
  direction: SortDirection;
}

export interface ContainerGroup {
  project: string;
  containers: ContainerLiveStat[];
  cpuTotal: number;
  memTotal: number;
}

export const DEFAULT_CONTAINER_PREFS: ContainerPrefs = { hideStopped: false, sortBy: 'name', direction: 'asc' };

export const NO_PROJECT = 'Sem Projeto (Avulsos)';

const STORAGE_KEY = 'dockkeeper.containers';

const SORT_FIELDS: ContainerSortField[] = ['name', 'cpu', 'mem'];
const DIRECTIONS: SortDirection[] = ['asc', 'desc'];

export const loadContainerPrefs = (): ContainerPrefs => {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return DEFAULT_CONTAINER_PREFS;
    const parsed = JSON.parse(raw) as Partial<ContainerPrefs>;
    if (!SORT_FIELDS.includes(parsed.sortBy as ContainerSortField) || !DIRECTIONS.includes(parsed.direction as SortDirection)) {
      return DEFAULT_CONTAINER_PREFS;
    }
    return {
      hideStopped: parsed.hideStopped === true,
      sortBy: parsed.sortBy as ContainerSortField,
      direction: parsed.direction as SortDirection,
    };
  } catch {
    return DEFAULT_CONTAINER_PREFS;
  }
};

export const saveContainerPrefs = (prefs: ContainerPrefs) => {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(prefs));
  } catch {
    return;
  }
};

const compare = (field: ContainerSortField, sinal: 1 | -1) =>
  (a: ContainerLiveStat, b: ContainerLiveStat): number => {
    if (field === 'name') return sinal * a.name.localeCompare(b.name, 'pt-BR');
    const valor = (c: ContainerLiveStat) => (field === 'cpu' ? c.cpu : c.mem_used) ?? null;
    return compararNumeros(valor(a), valor(b), sinal) || a.name.localeCompare(b.name, 'pt-BR');
  };

export const groupContainers = (
  containers: ContainerLiveStat[],
  options: ContainerPrefs & { search: string },
): ContainerGroup[] => {
  const term = options.search.trim().toLowerCase();
  const visible = containers.filter(
    (c) => c.name.toLowerCase().includes(term) && (!options.hideStopped || c.state === 'running'),
  );

  const order = compare(options.sortBy, options.direction === 'asc' ? 1 : -1);
  const groups = new Map<string, ContainerLiveStat[]>();
  for (const c of visible) {
    const project = c.project || NO_PROJECT;
    groups.set(project, [...(groups.get(project) ?? []), c]);
  }

  return [...groups.entries()]
    .sort(([a], [b]) => a.localeCompare(b, 'pt-BR'))
    .map(([project, list]) => ({
      project,
      containers: [...list].sort(order),
      cpuTotal: list.reduce((sum, c) => sum + (c.cpu ?? 0), 0),
      memTotal: list.reduce((sum, c) => sum + c.mem_used, 0),
    }));
};

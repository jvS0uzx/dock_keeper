import { afterEach, describe, expect, it } from 'vitest';

import type { ContainerLiveStat } from './api';
import { groupContainers, loadContainerPrefs, saveContainerPrefs, DEFAULT_CONTAINER_PREFS } from './containers';

const c = (name: string, project: string, state: string, cpu: number, mem: number): ContainerLiveStat => ({
  server_id: 's1',
  docker_id: name,
  name,
  project,
  state,
  status: '',
  cpu,
  mem_used: mem,
  mem_limit: 4096,
});

const LISTA = [
  c('web', 'loja', 'running', 12.5, 300),
  c('db', 'loja', 'running', 30, 1000),
  c('worker', 'loja', 'exited', 0, 0),
  c('cache', '', 'running', 2, 50),
];

afterEach(() => localStorage.clear());

describe('ordenação com container sem medida de CPU', () => {
  it('põe o nulo no fim nos dois sentidos', () => {
    const semMedida = { ...c('sem-medida', 'loja', 'running', 0, 10), cpu: null as unknown as number };
    const lista = [c('web', 'loja', 'running', 12.5, 300), semMedida, c('db', 'loja', 'running', 30, 1000)];

    const asc = groupContainers(lista, { ...DEFAULT_CONTAINER_PREFS, sortBy: 'cpu', direction: 'asc', search: '' });
    expect(asc[0].containers.map((x) => x.name)).toEqual(['web', 'db', 'sem-medida']);

    const desc = groupContainers(lista, { ...DEFAULT_CONTAINER_PREFS, sortBy: 'cpu', direction: 'desc', search: '' });
    expect(desc[0].containers.map((x) => x.name)).toEqual(['db', 'web', 'sem-medida']);
  });

  it('não conta o nulo como zero na soma do projeto', () => {
    const semMedida = { ...c('sem-medida', 'loja', 'running', 0, 10), cpu: null as unknown as number };
    const grupos = groupContainers([c('web', 'loja', 'running', 12.5, 300), semMedida], {
      ...DEFAULT_CONTAINER_PREFS,
      search: '',
    });
    expect(grupos[0].cpuTotal).toBeCloseTo(12.5);
  });
});

describe('groupContainers', () => {
  it('agrupa por projeto e soma CPU e memória dos visíveis', () => {
    const grupos = groupContainers(LISTA, { ...DEFAULT_CONTAINER_PREFS, search: '' });
    const loja = grupos.find((g) => g.project === 'loja');
    expect(loja?.containers.map((x) => x.name)).toEqual(['db', 'web', 'worker']);
    expect(loja?.cpuTotal).toBeCloseTo(42.5);
    expect(loja?.memTotal).toBe(1300);
  });

  it('oculta parados e recalcula a soma', () => {
    const grupos = groupContainers(LISTA, { ...DEFAULT_CONTAINER_PREFS, hideStopped: true, search: '' });
    const loja = grupos.find((g) => g.project === 'loja');
    expect(loja?.containers.map((x) => x.name)).toEqual(['db', 'web']);
  });

  it('ordena por nome, CPU e memória nos dois sentidos', () => {
    const nomes = (sortBy: 'name' | 'cpu' | 'mem', direction: 'asc' | 'desc') =>
      groupContainers(LISTA, { hideStopped: true, sortBy, direction, search: '' })
        .find((g) => g.project === 'loja')?.containers.map((x) => x.name);

    expect(nomes('name', 'asc')).toEqual(['db', 'web']);
    expect(nomes('name', 'desc')).toEqual(['web', 'db']);
    expect(nomes('cpu', 'desc')).toEqual(['db', 'web']);
    expect(nomes('cpu', 'asc')).toEqual(['web', 'db']);
    expect(nomes('mem', 'desc')).toEqual(['db', 'web']);
  });

  it('busca por nome e some com o projeto vazio', () => {
    const grupos = groupContainers(LISTA, { ...DEFAULT_CONTAINER_PREFS, search: 'cac' });
    expect(grupos.map((g) => g.project)).toEqual(['Sem Projeto (Avulsos)']);
  });
});

describe('preferências de containers', () => {
  it('guarda e recupera do localStorage', () => {
    saveContainerPrefs({ hideStopped: true, sortBy: 'cpu', direction: 'desc' });
    expect(loadContainerPrefs()).toEqual({ hideStopped: true, sortBy: 'cpu', direction: 'desc' });
  });

  it('valor corrompido cai no padrão', () => {
    localStorage.setItem('dockkeeper.containers', '{isto não é json');
    expect(loadContainerPrefs()).toEqual(DEFAULT_CONTAINER_PREFS);
    localStorage.setItem('dockkeeper.containers', JSON.stringify({ sortBy: 'hackeado', direction: 'x' }));
    expect(loadContainerPrefs()).toEqual(DEFAULT_CONTAINER_PREFS);
  });
});

import { describe, expect, it } from 'vitest';

type LeitorDeArquivo = { readFileSync(caminho: URL, codificacao: 'utf8'): string };

const fs = (
  globalThis as unknown as { process: { getBuiltinModule(id: 'node:fs'): LeitorDeArquivo } }
).process.getBuiltinModule('node:fs');

const ler = (caminho: string) => fs.readFileSync(new URL(caminho, import.meta.url), 'utf8');

const existe = (caminho: string) => {
  try {
    ler(caminho);
    return true;
  } catch {
    return false;
  }
};

describe('index.html só aponta para o ícone do DockKeeper', () => {
  const html = ler('../index.html');
  const icones = [...html.matchAll(/<link[^>]+rel="(?:icon|apple-touch-icon)"[^>]*>/g)].map(
    (m) => m[0],
  );

  it('declara ao menos o favicon e o apple-touch-icon', () => {
    expect(icones.length).toBeGreaterThanOrEqual(2);
  });

  it('não referencia ícone que não seja o do DockKeeper', () => {
    const permitidos = ['/favicon-32.png', '/apple-touch-icon.png'];
    const destinos = icones.map((tag) => /href="([^"]+)"/.exec(tag)?.[1] ?? '');
    expect(destinos.filter((href) => !permitidos.includes(href))).toEqual([]);
  });

  it('o logo padrão do Vite não fica no public', () => {
    expect(existe('../public/favicon.svg')).toBe(false);
  });
});

describe('nginx do painel', () => {
  const conf = ler('../nginx.conf');
  const api = /location \/api\/ \{([\s\S]*?)\n {4}\}/.exec(conf)?.[1] ?? '';

  it('aceita corpo de 10 MB na API, acima dos 8 MB de planta do backend', () => {
    expect(api).toMatch(/client_max_body_size\s+10m;/);
  });

  it('continua incluindo os cabeçalhos de segurança em todo location', () => {
    const blocos = conf.match(/location [^{]+\{/g) ?? [];
    const inclusoes = conf.match(/include \/etc\/nginx\/seguranca\.conf;/g) ?? [];
    expect(inclusoes.length).toBe(blocos.length + 1);
  });
});

describe('versão exibida vem de um lugar só', () => {
  it('a barra lateral não crava a versão', () => {
    expect(ler('./components/Sidebar.tsx')).not.toMatch(/v\d+\.\d+\.\d+/);
  });

  it('o package.json tem versão de verdade', () => {
    const pacote = JSON.parse(ler('../package.json')) as { version: string };
    expect(pacote.version).not.toBe('0.0.0');
  });
});

describe('dependências sem uso não ficam no package.json', () => {
  it('clsx e tailwind-merge saíram', () => {
    const pacote = JSON.parse(ler('../package.json')) as {
      dependencies: Record<string, string>;
    };
    expect(Object.keys(pacote.dependencies)).not.toContain('clsx');
    expect(Object.keys(pacote.dependencies)).not.toContain('tailwind-merge');
  });

  it('o tailwind.config.js que o Tailwind v4 não carrega saiu', () => {
    expect(existe('../tailwind.config.js')).toBe(false);
  });
});

describe('CI do painel', () => {
  const ci = ler('../../.github/workflows/ci.yml');

  it('roda os testes Go com -p 1, como o portão local, porque os pacotes dividem um banco', () => {
    const linha = ci.split('\n').find((l) => /run: go test/.test(l)) ?? '';
    expect(linha).toMatch(/-p 1\b/);
  });

  it('confere que o compose entrega ao backend as variáveis do .env.example', () => {
    expect(ci).toMatch(/confere-env-compose\.sh/);
  });
});

describe('playbook do agente não imprime convite nem credencial', () => {
  const playbook = ler('../../backend/deploy/agent/ansible/dockkeeper-agent.yml');
  const tarefas = playbook.split(/\n(?= {4}- name: )/).slice(1);
  const sensiveis = tarefas.filter((t) => /agente_enroll_token|agente_token|credential\.json/.test(t));

  it('acha as tarefas que tocam o segredo', () => {
    expect(sensiveis.length).toBeGreaterThanOrEqual(3);
  });

  it.each(sensiveis.map((t) => [t.split('\n')[0].trim(), t]))('%s', (_nome, tarefa) => {
    expect(tarefa).toMatch(/\n {6}no_log: true\n/);
  });
});

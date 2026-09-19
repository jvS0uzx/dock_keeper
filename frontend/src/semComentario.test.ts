import ts from 'typescript';
import { describe, expect, it } from 'vitest';

type LeitorDeArquivo = { readFileSync(caminho: URL, codificacao: 'utf8'): string };

const fs = (
  globalThis as unknown as { process: { getBuiltinModule(id: 'node:fs'): LeitorDeArquivo } }
).process.getBuiltinModule('node:fs');

const arquivos = Object.keys(
  import.meta.glob([
    './**/*.{ts,tsx,css}',
    '!./**/node_modules/**',
    '../*.{ts,js}',
    '../tsconfig*.json',
    '../.oxlintrc.json',
    '../Dockerfile',
    '../nginx.conf',
    '../.gitignore',
    '../.dockerignore',
    '../index.html',
  ]),
);

const fontes = Object.fromEntries(
  arquivos.map((nome) => [nome, fs.readFileSync(new URL(nome, import.meta.url), 'utf8')]),
);

const DIRETIVA = /^\/\/\/\s*<(reference|amd)|^\/\/\s*@ts-|^\/[/*]\s*(eslint|oxlint)-/;

function linhasTs(nome: string, texto: string): number[] {
  const tipo = nome.endsWith('.tsx')
    ? ts.ScriptKind.TSX
    : nome.endsWith('.json')
      ? ts.ScriptKind.JSON
      : nome.endsWith('.ts')
        ? ts.ScriptKind.TS
        : ts.ScriptKind.JS;
  const sf = ts.createSourceFile(nome, texto, ts.ScriptTarget.Latest, true, tipo);
  const inicios = new Set<number>();
  const anotar = (pos: number, fim: number, sempre = false) => {
    if (sempre || !DIRETIVA.test(texto.slice(pos, fim))) inicios.add(pos);
  };
  const varrer = (pos: number) => {
    for (const r of ts.getLeadingCommentRanges(texto, pos) ?? []) anotar(r.pos, r.end);
    for (const r of ts.getTrailingCommentRanges(texto, pos) ?? []) anotar(r.pos, r.end);
  };
  const visitar = (no: ts.Node) => {
    if (no.kind === ts.SyntaxKind.JsxText) return;
    if (ts.isJsxExpression(no) && !no.expression) {
      anotar(no.getStart(sf), no.end, true);
      return;
    }
    const filhos = no.getChildren(sf);
    if (filhos.length === 0) {
      varrer(no.pos);
      return;
    }
    filhos.forEach(visitar);
  };
  visitar(sf);
  varrer(sf.endOfFileToken.pos);
  return [...new Set([...inicios].map((p) => sf.getLineAndCharacterOfPosition(p).line + 1))].sort((a, b) => a - b);
}

function linhasCss(texto: string): number[] {
  const semTexto = texto.replace(/"(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*'/g, (s) => s.replace(/[^\n]/g, ' '));
  return linhasDe(semTexto, /\/\*/g);
}

function linhasHtml(texto: string): number[] {
  return linhasDe(texto, /<!--/g);
}

function linhasDe(texto: string, padrao: RegExp): number[] {
  return [...texto.matchAll(padrao)].map((m) => texto.slice(0, m.index).split('\n').length);
}

function linhasHash(texto: string): number[] {
  const linhas: number[] = [];
  texto.split('\n').forEach((bruta, i) => {
    const limpa = bruta.trim();
    if (!limpa.startsWith('#')) return;
    if (i === 0 && (limpa.startsWith('#!') || limpa.startsWith('# syntax='))) return;
    linhas.push(i + 1);
  });
  return linhas;
}

function comentarios(nome: string, texto: string): number[] {
  if (/\.(tsx?|jsx?|json)$/.test(nome)) return linhasTs(nome, texto);
  if (nome.endsWith('.css')) return linhasCss(texto);
  if (nome.endsWith('.html')) return linhasHtml(texto);
  return linhasHash(texto);
}

describe('zero comentário em código', () => {
  it('varre o frontend inteiro, com conteúdo', () => {
    expect(arquivos.length).toBeGreaterThan(40);
    expect(arquivos).toEqual(expect.arrayContaining(['./index.css', '../index.html', '../.gitignore', '../Dockerfile']));
    expect(fontes['./index.css'].length).toBeGreaterThan(0);
  });

  it('nenhum arquivo do frontend tem comentário', () => {
    const achados = Object.entries(fontes).flatMap(([nome, texto]) =>
      comentarios(nome, texto).map((linha) => `${nome}:${linha}`),
    );
    expect(achados).toEqual([]);
  });

  it('acusa comentário de cada linguagem', () => {
    expect(comentarios('a.ts', 'const a = 1; // ao lado\n/* bloco */\nconst b = 2;\n')).toEqual([1, 2]);
    expect(comentarios('b.tsx', 'const v = (\n  <div>\n    {/* jsx */}\n    <p>x</p>\n  </div>\n);\n')).toEqual([3]);
    expect(comentarios('c.json', '{\n  // nota\n  "a": 1\n}\n')).toEqual([2]);
    expect(comentarios('d.css', 'a {\n  color: red; /* cor */\n}\n')).toEqual([2]);
    expect(comentarios('e.html', '<p>x</p>\n<!-- nota -->\n')).toEqual([2]);
    expect(comentarios('Dockerfile', '# syntax=docker/dockerfile:1\nFROM x\n# nota\n')).toEqual([3]);
  });

  it('não confunde texto com comentário nem acusa diretiva', () => {
    expect(comentarios('a.ts', "const u = 'http://x/*y*/';\nconst r = /\\/\\/ z/;\nconst t = `// ${u}`;\n")).toEqual([]);
    expect(comentarios('b.tsx', 'const v = <p>texto // não é comentário</p>;\n')).toEqual([]);
    expect(comentarios('c.ts', '/// <reference types="vite/client" />\n// @ts-expect-error\nconst a: number = "x";\n')).toEqual([]);
    expect(comentarios('d.css', 'a::before {\n  content: "/* não */";\n}\n')).toEqual([]);
    expect(comentarios('run.sh', '#!/bin/bash\necho "# texto"\n')).toEqual([]);
  });
});

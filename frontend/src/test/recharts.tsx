import { cloneElement, type ReactElement } from 'react';

type Recharts = typeof import('recharts');

export const comLarguraFixa = (real: Recharts): Recharts => ({
  ...real,
  ResponsiveContainer: (({ children }: { children: ReactElement<{ width?: number; height?: number }> }) => (
    <div style={{ width: 800, height: 300 }}>{cloneElement(children, { width: 800, height: 300 })}</div>
  )) as unknown as Recharts['ResponsiveContainer'],
});

export const responder = (status: number, corpo: unknown) =>
  new Response(JSON.stringify(corpo), { status, headers: { 'Content-Type': 'application/json' } });

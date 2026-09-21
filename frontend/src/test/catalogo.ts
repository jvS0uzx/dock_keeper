import type { MetricaDoCatalogo } from '../lib/api';

const arquivos = import.meta.glob('../__fixtures__/metrics-catalogo.saida-de-backend-*.json', {
  eager: true,
  import: 'default',
}) as Record<string, MetricaDoCatalogo[]>;

export const CATALOGO: MetricaDoCatalogo[] = Object.values(arquivos)[0];

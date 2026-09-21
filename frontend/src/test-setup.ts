import { afterEach } from 'vitest';
import { cleanup } from '@testing-library/react';

if (!Element.prototype.scrollIntoView) {
  Element.prototype.scrollIntoView = () => {};
}

if (!globalThis.ResizeObserver) {
  globalThis.ResizeObserver = class {
    observe() {}
    unobserve() {}
    disconnect() {}
  };
}

afterEach(async () => {
  cleanup();
  localStorage.clear();
  (await import('./lib/catalogo')).esquecerCatalogo();
});

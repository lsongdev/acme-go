// ACME certificate manager entry point (panel module contract, version 1).
// The host is a panel shell, or standalone.js on its own.
import { createMount } from './lib/mount.js';

const page = load => ({ load });

const routes = [
  { path: '/certificates', ...page(() => import('./pages/certificates.js')) },
  { path: '/certificates/new', ...page(() => import('./pages/certificate-new.js')) },
  { path: '/certificates/:id', ...page(() => import('./pages/certificate.js')) },
  { path: '/issuances', ...page(() => import('./pages/issuances.js')) },
  { path: '/issuances/:id', ...page(() => import('./pages/issuance.js')) },
  { path: '/accounts', ...page(() => import('./pages/accounts.js')) },
  { path: '/providers', ...page(() => import('./pages/providers.js')) },
  { path: '/orders', ...page(() => import('./pages/orders.js')) },
  // The module root opens the certificate list.
  { path: '/', ...page(() => import('./pages/certificates.js')) },
];

export const mount = createMount({ routes });

// ACME-specific UI helpers: API access, statuses, issuance steps and
// certificate formatting. Generic components come from lib/.
import { html } from './lib/html.js';
import { useResource } from './lib/hooks.js';
import { usePanel } from './lib/panel.js';
import { Status as ShellStatus } from './lib/ui.js';

// useApi loads a GET endpoint of this module, polling every interval(data)
// milliseconds while that returns a positive number.
export function useApi(path, interval) {
  const { api } = usePanel();
  return useResource(signal => api.get(path, { signal }), [path], { interval });
}

// Link points at a page of this module.
export function Link({ to, children, ...props }) {
  const { href } = usePanel();
  return html`<a href=${href(to)} ...${props}>${children}</a>`;
}

const STATUS = {
  // certificates
  pending: ['Issuing', 'info'],
  active: ['Active', 'success'],
  failed: ['Failed', 'danger'],
  revoked: ['Revoked', undefined],
  // issuances
  running: ['Running', 'info'],
  retrying: ['Retry scheduled', 'warning'],
  waiting: ['Needs action', 'warning'],
  succeeded: ['Succeeded', 'success'],
  // accounts and orders
  valid: ['Valid', 'success'],
  deactivated: ['Deactivated', undefined],
  ready: ['Ready', 'info'],
  processing: ['Processing', 'info'],
  invalid: ['Invalid', 'danger'],
};

export function Status({ status }) {
  const [label, variant] = STATUS[status] || [status];
  return html`<${ShellStatus} state=${status} label=${label} variant=${variant} />`;
}

export const STEPS = [
  ['queued', 'Queued'],
  ['creating-order', 'Creating the order'],
  ['authorizing', 'Authorizing'],
  ['publishing-dns', 'Publishing DNS records'],
  ['awaiting-dns', 'Waiting for DNS records'],
  ['checking-dns', 'Checking DNS records'],
  ['validating', 'CA validating'],
  ['finalizing', 'Submitting the CSR'],
  ['downloading', 'Downloading'],
  ['done', 'Done'],
];

export const stepLabel = step => STEPS.find(([key]) => key === step)?.[1] || step;

export const CHALLENGES = { 'http-01': 'HTTP-01', 'dns-01': 'DNS-01' };

export const unfinished = issuance => issuance && (issuance.status === 'running' || issuance.status === 'waiting' || issuance.status === 'retrying');

export function formatDate(value, withTime = false) {
  if (!value) return '—';
  const date = new Date(value);
  if (Number.isNaN(date.getTime()) || date.getFullYear() < 2000) return '—';
  return date.toLocaleString('zh-CN', withTime
    ? { year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }
    : { year: 'numeric', month: '2-digit', day: '2-digit' });
}

export function daysUntil(value) {
  if (!value) return null;
  return Math.floor((new Date(value).getTime() - Date.now()) / 86400000);
}

export function Expiry({ value }) {
  const days = daysUntil(value);
  if (days === null) return html`<span class="muted">—</span>`;
  const tone = days < 0 ? 'error' : days <= 14 ? 'warn' : days <= 30 ? 'info' : 'ok';
  const text = days < 0 ? `expired ${-days} days ago` : `${days} days left`;
  return html`<span>${formatDate(value)} <span class=${`days days-${tone}`}>${text}</span></span>`;
}

export function Domains({ domains }) {
  if (!domains?.length) return '—';
  const [first, ...rest] = domains;
  return html`<span class="mono">${first}</span>${rest.length ? html` <span class="muted" title=${rest.join('\n')}>+${rest.length}</span>` : null}`;
}

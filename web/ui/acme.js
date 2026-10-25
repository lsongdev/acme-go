// ACME-specific UI helpers: API access, statuses, issuance steps and
// certificate formatting. Generic components are in lib/.
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
  pending: ['签发中', 'blue'],
  active: ['有效', 'green'],
  failed: ['失败', 'red'],
  revoked: ['已吊销', 'gray'],
  // issuances
  running: ['进行中', 'blue'],
  waiting: ['等待操作', 'amber'],
  succeeded: ['成功', 'green'],
  // accounts and orders
  valid: ['有效', 'green'],
  deactivated: ['已停用', 'gray'],
  ready: ['待签发', 'blue'],
  processing: ['处理中', 'blue'],
  invalid: ['无效', 'red'],
};

export function Status({ status }) {
  const [label, tone] = STATUS[status] || [status, 'gray'];
  return html`<${ShellStatus} state=${status} label=${label} tone=${tone} />`;
}

export const STEPS = [
  ['queued', '排队'],
  ['creating-order', '创建订单'],
  ['authorizing', '获取授权'],
  ['publishing-dns', '发布 DNS 记录'],
  ['awaiting-dns', '等待添加 DNS 记录'],
  ['checking-dns', '检查 DNS 记录'],
  ['validating', 'CA 验证'],
  ['finalizing', '提交 CSR'],
  ['downloading', '下载证书'],
  ['done', '完成'],
];

export const stepLabel = step => STEPS.find(([key]) => key === step)?.[1] || step;

export const CHALLENGES = { 'http-01': 'HTTP-01', 'dns-01': 'DNS-01' };

export const unfinished = issuance => issuance && (issuance.status === 'running' || issuance.status === 'waiting');

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
  const text = days < 0 ? `已过期 ${-days} 天` : `剩余 ${days} 天`;
  return html`<span>${formatDate(value)} <span class=${`days days-${tone}`}>${text}</span></span>`;
}

export function Domains({ domains }) {
  if (!domains?.length) return '—';
  const [first, ...rest] = domains;
  return html`<span class="mono">${first}</span>${rest.length ? html` <span class="muted" title=${rest.join('\n')}>+${rest.length}</span>` : null}`;
}

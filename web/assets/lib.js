import { h } from 'preact';
import { useCallback, useEffect, useRef, useState } from 'preact/hooks';
import htm from 'htm';

export const html = htm.bind(h);

export class APIError extends Error {
  constructor(status, code, message) {
    super(message);
    this.status = status;
    this.code = code;
  }
}

// request calls the JSON API and returns its "data" member.
export async function request(method, path, body) {
  const options = { method, headers: {} };
  if (body !== undefined) {
    options.headers['Content-Type'] = 'application/json';
    options.body = JSON.stringify(body);
  }
  const response = await fetch(`api${path}`, options);
  if (response.status === 204) return null;
  let payload = null;
  try { payload = await response.json(); } catch { /* empty or non-JSON body */ }
  if (!response.ok) {
    throw new APIError(response.status, payload?.error?.code, payload?.error?.message || response.statusText);
  }
  return payload?.data ?? null;
}

export const api = {
  get: path => request('GET', path),
  post: (path, body) => request('POST', path, body),
  patch: (path, body) => request('PATCH', path, body),
  del: path => request('DELETE', path),
};

// useResource loads path and reloads it every interval ms while
// interval(data) returns a positive number.
export function useResource(path, interval) {
  const [state, setState] = useState({ data: null, error: null, loading: true });
  const intervalRef = useRef(interval);
  intervalRef.current = interval;
  const load = useCallback(async () => {
    try {
      const data = await api.get(path);
      setState({ data, error: null, loading: false });
      return data;
    } catch (error) {
      setState(prev => ({ ...prev, error, loading: false }));
      return null;
    }
  }, [path]);
  useEffect(() => {
    let timer;
    let alive = true;
    const tick = async () => {
      const data = await load();
      const delay = alive && intervalRef.current ? intervalRef.current(data) : 0;
      if (alive && delay > 0) timer = setTimeout(tick, delay);
    };
    setState(prev => ({ ...prev, loading: true }));
    tick();
    return () => { alive = false; clearTimeout(timer); };
  }, [load]);
  return { ...state, reload: load };
}

// useAction wraps an async operation with busy and error state.
export function useAction() {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(null);
  const run = useCallback(async fn => {
    setBusy(true);
    setError(null);
    try {
      return await fn();
    } catch (err) {
      setError(err);
      return undefined;
    } finally {
      setBusy(false);
    }
  }, []);
  return { busy, error, run, setError };
}

export const navigate = path => { location.hash = `#${path}`; };

export const Link = ({ to, children, ...props }) => html`<a href=${`#${to}`} ...${props}>${children}</a>`;

export function ErrorNotice({ error }) {
  if (!error) return null;
  return html`<div class="notice notice-error" role="alert">${error.message || String(error)}</div>`;
}

export function Loading() {
  return html`<p class="muted">加载中…</p>`;
}

export function Empty({ children }) {
  return html`<div class="empty">${children}</div>`;
}

const STATUS = {
  // certificates
  pending: ['签发中', 'info'],
  active: ['有效', 'ok'],
  failed: ['失败', 'error'],
  revoked: ['已吊销', 'muted'],
  // issuances
  running: ['进行中', 'info'],
  waiting: ['等待操作', 'warn'],
  succeeded: ['成功', 'ok'],
  // accounts and orders
  valid: ['有效', 'ok'],
  deactivated: ['已停用', 'muted'],
  ready: ['待签发', 'info'],
  processing: ['处理中', 'info'],
  invalid: ['无效', 'error'],
};

export function Status({ status }) {
  const [label, tone] = STATUS[status] || [status, 'muted'];
  return html`<span class=${`badge badge-${tone}`}>${label}</span>`;
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

export function CopyButton({ text, label = '复制' }) {
  const [copied, setCopied] = useState(false);
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(text);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch { /* clipboard unavailable, e.g. plain HTTP on a remote host */ }
  };
  return html`<button type="button" class="button button-small" onClick=${copy}>${copied ? '已复制' : label}</button>`;
}

export const unfinished = issuance => issuance && (issuance.status === 'running' || issuance.status === 'waiting');

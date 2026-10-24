// Shared UI building blocks for every module.
import { html, useEffect, useRef, useState } from './html.js';
import { t, isChinese } from './i18n.js';

export function Page({ title, subtitle, actions, children }) {
  return html`<section class="page">
    <header class="page-header">
      <div class="page-title">
        <h1>${title}</h1>
        ${subtitle ? html`<div class="page-subtitle">${subtitle}</div>` : null}
      </div>
      ${actions ? html`<div class="page-actions">${actions}</div>` : null}
    </header>
    ${children}
  </section>`;
}

export function Notice({ error, tone = 'error', children }) {
  if (!error && !children) return null;
  return html`<div class=${`notice notice-${tone}`} role=${tone === 'error' ? 'alert' : 'status'}>
    ${children || error.message || String(error)}
  </div>`;
}

export function Loading() {
  return html`<div class="loading" aria-busy="true"><span class="spinner"></span>${t('loading')}</div>`;
}

export function Empty({ children }) {
  return html`<div class="empty">${children}</div>`;
}

// Resource renders the loading / error / empty states of a useResource result.
export function Resource({ resource, empty, children }) {
  const { data, error } = resource;
  if (data === undefined) return error ? html`<${Notice} error=${error} />` : html`<${Loading} />`;
  return html`${error ? html`<${Notice} error=${error} />` : null}
    ${Array.isArray(data) && data.length === 0 && empty ? html`<${Empty}>${empty}<//>` : children(data)}`;
}

const tones = {
  running: 'green', healthy: 'green', succeeded: 'green', active: 'green', valid: 'green', ok: 'green',
  partial: 'amber', paused: 'amber', restarting: 'amber', starting: 'amber', waiting: 'amber', pending: 'blue',
  processing: 'blue', ready: 'blue', removing: 'amber',
  exited: 'gray', created: 'gray', stopped: 'gray', inactive: 'gray', missing: 'gray', canceled: 'gray',
  deactivated: 'gray', revoked: 'gray',
  dead: 'red', failed: 'red', invalid: 'red', unhealthy: 'red', error: 'red',
};

// Default Chinese labels; other languages show the state as is.
const labels = {
  running: '运行中', partial: '部分运行', paused: '已暂停', restarting: '重启中', exited: '已退出',
  created: '已创建', stopped: '已停止', inactive: '未启动', missing: '无容器', dead: '异常',
  removing: '删除中', succeeded: '成功', failed: '失败', canceled: '已取消', healthy: '健康', unhealthy: '不健康', starting: '启动中',
};

// Status renders a state as a coloured badge. Modules pass label and tone
// for states the shell does not know.
export function Status({ state, label, tone, title }) {
  return html`<span class=${`badge badge-${tone || tones[state] || 'gray'}`} title=${title}>
    <span class="dot"></span>${label || (isChinese() && labels[state]) || state}
  </span>`;
}

export function Badge({ tone = 'gray', children, title }) {
  return html`<span class=${`badge badge-${tone}`} title=${title}>${children}</span>`;
}

export function Field({ label, hint, error, required, children }) {
  return html`<label class=${`field ${error ? 'has-error' : ''}`}>
    <span class="field-label">${label}${required ? html`<span class="required">*</span>` : null}</span>
    ${children}
    ${error ? html`<span class="field-error">${error}</span>` : hint ? html`<span class="field-hint">${hint}</span>` : null}
  </label>`;
}

// KV renders a definition list from [label, value] pairs, skipping empty values.
export function KV({ items }) {
  return html`<dl class="kv">
    ${items.filter(([, v]) => v !== undefined && v !== null && v !== '' && !(Array.isArray(v) && !v.length)).map(([k, v]) => html`
      <dt>${k}</dt><dd>${v}</dd>`)}
  </dl>`;
}

export function Tabs({ tabs, active, onChange }) {
  return html`<nav class="tabs" role="tablist">
    ${tabs.map(([id, label]) => html`<button type="button" role="tab" class=${`tab ${active === id ? 'active' : ''}`}
      aria-selected=${active === id} onClick=${() => onChange(id)}>${label}</button>`)}
  </nav>`;
}

export function Modal({ title, onClose, children, footer, wide }) {
  useEffect(() => {
    const onKey = e => e.key === 'Escape' && onClose?.();
    addEventListener('keydown', onKey);
    return () => removeEventListener('keydown', onKey);
  }, [onClose]);
  return html`<div class="modal-backdrop" onClick=${e => e.target === e.currentTarget && onClose?.()}>
    <div class=${`modal ${wide ? 'modal-wide' : ''}`} role="dialog" aria-modal="true" aria-label=${title}>
      <header class="modal-header"><h2>${title}</h2>
        ${onClose ? html`<button type="button" class="btn btn-ghost btn-icon" aria-label=${t('close')} onClick=${onClose}>✕</button>` : null}
      </header>
      <div class="modal-body">${children}</div>
      ${footer ? html`<footer class="modal-footer">${footer}</footer>` : null}
    </div>
  </div>`;
}

export function LogView({ lines, follow = true, empty = t('noOutput') }) {
  const ref = useRef();
  const pinned = useRef(true);
  useEffect(() => {
    const el = ref.current;
    if (el && follow && pinned.current) el.scrollTop = el.scrollHeight;
  }, [lines.length, follow]);
  const onScroll = e => {
    const el = e.currentTarget;
    pinned.current = el.scrollHeight - el.scrollTop - el.clientHeight < 24;
  };
  return html`<pre class="log" ref=${ref} onScroll=${onScroll}>${lines.length
    ? lines.map(l => typeof l === 'string' ? l + '\n' : html`<span class=${l.stream === 'stderr' ? 'log-err' : ''}>${l.text}\n</span>`)
    : html`<span class="muted">${empty}</span>`}</pre>`;
}

// ---- Toasts ---------------------------------------------------------------

let toastListener = () => {};
let toastSeq = 0;

export function toast(message, tone = 'info') {
  toastListener({ id: ++toastSeq, message, tone });
}

export function toastError(err) {
  toast(err?.message || String(err), 'error');
}

export function Toaster() {
  const [items, setItems] = useState([]);
  useEffect(() => {
    toastListener = item => {
      setItems(list => [...list, item]);
      setTimeout(() => setItems(list => list.filter(i => i.id !== item.id)), item.tone === 'error' ? 8000 : 3500);
    };
  }, []);
  return html`<div class="toasts" aria-live="polite">
    ${items.map(i => html`<div key=${i.id} class=${`toast toast-${i.tone}`}>${i.message}</div>`)}
  </div>`;
}

// ---- Dialogs --------------------------------------------------------------

let dialogListener = () => {};
let dialogSeq = 0;

// openDialog shows render(close) above the page until close is called, so
// modules can open dialogs from anywhere (e.g. after starting a job).
export function openDialog(render) {
  const id = ++dialogSeq;
  const close = () => dialogListener({ id, close: true });
  dialogListener({ id, render: () => render(close) });
  return close;
}

export function DialogHost() {
  const [dialogs, setDialogs] = useState([]);
  useEffect(() => {
    dialogListener = d => setDialogs(list => d.close ? list.filter(x => x.id !== d.id) : [...list, d]);
  }, []);
  return dialogs.map(d => html`<div key=${d.id}>${d.render()}</div>`);
}

// CopyButton copies text to the clipboard (unavailable on plain HTTP to a
// remote host, where it silently does nothing).
export function CopyButton({ text, label = t('copy'), class: cls = 'btn btn-sm' }) {
  const [copied, setCopied] = useState(false);
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(text);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch { /* clipboard unavailable */ }
  };
  return html`<button type="button" class=${cls} onClick=${copy}>${copied ? t('copied') : label}</button>`;
}

// ListInput edits a list of strings, always keeping one empty row to type in.
export function ListInput({ values, onChange, placeholder }) {
  const rows = values.length && values[values.length - 1] === '' ? values : [...values, ''];
  const set = (i, v) => onChange(rows.map((r, j) => (j === i ? v : r)).filter((r, j, all) => r !== '' || j === all.length - 1));
  return html`<div class="list-editor">
    ${rows.map((value, i) => html`<div class="list-editor-row" key=${i}>
      <input class="input mono" value=${value} placeholder=${placeholder} onInput=${e => set(i, e.target.value)} />
      ${value ? html`<button type="button" class="btn btn-ghost btn-icon" aria-label=${t('remove')} onClick=${() => onChange(rows.filter((_, j) => j !== i))}>✕</button>` : null}
    </div>`)}
  </div>`;
}

// ConfirmButton asks for confirmation in a dialog before running
// onConfirm(checked). An optional checkbox offers a variant of the action
// (e.g. "also delete volumes"); onConfirm may return a promise.
export function ConfirmButton({ label, title, description, checkbox, confirmLabel = t('confirm'), danger = true, class: cls = 'btn', onConfirm }) {
  const [open, setOpen] = useState(false);
  const [checked, setChecked] = useState(false);
  const [running, setRunning] = useState(false);
  const submit = async () => {
    setRunning(true);
    try {
      await onConfirm(checked);
      setOpen(false);
    } catch (err) {
      toastError(err);
    } finally {
      setRunning(false);
    }
  };
  return html`<button class=${`${cls} ${danger ? 'btn-danger' : ''}`} onClick=${() => { setChecked(false); setOpen(true); }}>${label}</button>
    ${open ? html`<${Modal} title=${title} onClose=${() => setOpen(false)} footer=${html`
      <span class="spacer"></span>
      <button class="btn" onClick=${() => setOpen(false)}>${t('cancel')}</button>
      <button class=${`btn ${danger ? 'btn-danger' : 'btn-primary'}`} disabled=${running} onClick=${submit}>${confirmLabel}</button>`}>
      <p style="margin-top:0">${description}</p>
      ${checkbox ? html`<label class="check"><input type="checkbox" checked=${checked} onChange=${e => setChecked(e.target.checked)} />${checkbox}</label>` : null}
    <//>` : null}`;
}

// confirmDanger asks before destructive actions.
export function confirmDanger(message) {
  return window.confirm(message);
}

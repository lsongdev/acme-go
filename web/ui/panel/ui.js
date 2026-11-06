// Copied from github.com/lsongdev/ohmyserver/panel/ui; change it there and copy it back.
// The few UI helpers the panel modules share beyond the library's components
// (import those from @lsongdev/scripts/react/components/index.js): state
// badges, loading states, dialogs that restore focus across shadow roots,
// and error toasts.
import { html, useLayoutEffect } from './html.js';
import { Alert, Badge, Dialog, Loading, Empty, toast } from '@lsongdev/scripts/react/components/index.js';

// Resource renders the loading, error and empty states of a useResource result.
export function Resource({ resource, empty, children }) {
  const { data, error } = resource;
  if (data === undefined) return error ? html`<${Alert} error=${error} />` : html`<${Loading} />`;
  return html`${error ? html`<${Alert} error=${error} />` : null}
    ${Array.isArray(data) && data.length === 0 && empty ? html`<${Empty}>${empty}<//>` : children(data)}`;
}

// The badge variant of states the modules share; others show plain.
const variants = {
  running: 'success', healthy: 'success', succeeded: 'success', active: 'success', valid: 'success', ok: 'success',
  partial: 'warning', paused: 'warning', restarting: 'warning', starting: 'warning', waiting: 'warning', removing: 'warning',
  pending: 'info', processing: 'info', ready: 'info',
  dead: 'danger', failed: 'danger', invalid: 'danger', unhealthy: 'danger', error: 'danger',
};

const capitalize = s => (s ? s[0].toUpperCase() + s.slice(1) : s);

// Status renders a state as a badge with a dot, labelled with the state
// unless the page passes a label. Pages pass a variant for states not
// listed above.
export function Status({ state, label, variant, title }) {
  return html`<${Badge} variant=${variant || variants[state]} dot title=${title}>${label || capitalize(state)}<//>`;
}

// Modal is a dialog shown for as long as it is rendered; onClose asks the
// owner to stop rendering it.
export function Modal({ title, onClose, children, footer, wide, closeOnBackdrop = false }) {
  useLayoutEffect(() => {
    // document.activeElement is the shadow host for mounted modules.
    let previous = document.activeElement;
    while (previous?.shadowRoot?.activeElement) previous = previous.shadowRoot.activeElement;
    return () => queueMicrotask(() => { if (previous?.isConnected) previous.focus(); });
  }, []);
  // Native close controls fire after closing. Reopen if the owner vetoes
  // dismissal (unsaved input or a pending request). Escape can be stopped
  // before the native dialog closes at all.
  const close = event => {
    if (onClose?.() === false && event.currentTarget.isConnected) event.currentTarget.showModal();
  };
  const cancel = event => { event.preventDefault(); onClose?.(); };
  return html`<${Dialog} open title=${title} footer=${footer} onClose=${close} onCancel=${cancel}
    closeOnBackdrop=${closeOnBackdrop} className=${wide ? 'dialog-wide' : ''}>${children}<//>`;
}

// toastError shows a failed action's error.
export function toastError(err) {
  toast(err?.message || String(err), 'danger');
}

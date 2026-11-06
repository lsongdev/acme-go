// Copied from github.com/lsongdev/ohmyserver/panel/ui; change it there and copy it back.
import { useLayoutEffect, useRef } from './html.js';
import { usePanel } from './panel.js';

export function routeURL(path, query = {}) {
  const search = new URLSearchParams(Object.entries(query).filter(([, v]) => v !== '' && v != null)).toString();
  return path + (search ? '?' + search : '');
}

export function returnPath(query, fallback = '/') {
  return typeof query.return === 'string' && /^\/(?!\/)/.test(query.return) ? query.return : fallback;
}

// The host guards links, programmatic navigation, and browser history.
// beforeunload covers refresh/close/external navigation. Release the guard
// synchronously after a successful save, before navigating away.
export function useUnsavedChanges(dirty, { pending = false } = {}) {
  const { beforeLeave } = usePanel();
  const changed = useRef({ dirty, pending });
  changed.current = { dirty, pending };
  useLayoutEffect(() => {
    const stop = beforeLeave?.(() => !changed.current.pending && (!changed.current.dirty || window.confirm('Discard your unsaved changes?')));
    const unload = event => {
      if (!changed.current.dirty && !changed.current.pending) return;
      event.preventDefault();
      event.returnValue = '';
    };
    window.addEventListener('beforeunload', unload);
    return () => { stop?.(); window.removeEventListener('beforeunload', unload); };
  }, [beforeLeave]);
  return () => { changed.current = { dirty: false, pending: false }; };
}

export { FormSection } from '@lsongdev/scripts/react/components/index.js';

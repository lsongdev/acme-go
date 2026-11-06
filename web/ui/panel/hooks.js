// Copied from github.com/lsongdev/ohmyserver/panel/ui; change it there and copy it back.
// Hooks for module pages: the library's useResource and useAction, plus
// useBusy for lists whose rows run actions independently.
import { useCallback, useState } from '@lsongdev/scripts/react/react.js';

export { useResource, useAction } from '@lsongdev/scripts/react/hooks/resource.js';

// useBusy tracks in-flight actions by key and reports their errors.
export function useBusy(onError) {
  const [busy, setBusy] = useState({});
  const run = useCallback(async (key, fn) => {
    setBusy(b => ({ ...b, [key]: true }));
    try {
      return await fn();
    } catch (err) {
      onError?.(err);
    } finally {
      setBusy(b => { const next = { ...b }; delete next[key]; return next; });
    }
  }, [onError]);
  return [busy, run];
}

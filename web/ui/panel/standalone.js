// Copied from github.com/lsongdev/ohmyserver/panel/ui; change it there and copy it back.
// Runs a module on its own: a minimal host that reads manifest.json for the
// top navigation and drives mount() from the URL hash, exactly as the panel
// shell would. A module's standalone entry imports standalone() from here and
// mount() from its module.js, and calls standalone(mount).
import { html, render } from '@lsongdev/scripts/react/react.js';
import { Navbar } from '@lsongdev/scripts/react/components/index.js';
import { createHashNavigation } from '@lsongdev/scripts/navigation/hash.js';

export async function standalone(mount, {
  topbar = document.getElementById('topbar'),
  root = document.getElementById('module'),
} = {}) {
  const navigation = createHashNavigation({ event: 'panel:routechange', stateKey: 'panelNavigation' });
  const manifest = await (await fetch('manifest.json')).json();
  document.title = manifest.title;

  const location = () => {
    const raw = window.location.hash.replace(/^#/, '') || '/';
    const [path, search = ''] = raw.split('?');
    return { path: path || '/', query: Object.fromEntries(new URLSearchParams(search)) };
  };
  const href = (path, query) => {
    const s = new URLSearchParams(Object.entries(query || {}).filter(([, v]) => v !== '' && v != null)).toString();
    return '#' + path + (s ? '?' + s : '');
  };
  const navigate = navigation.navigate;
  const context = () => ({
    id: manifest.id,
    apiBase: new URL('api/', document.baseURI).href,
    assetsBase: new URL('./', document.baseURI).href,
    ...location(),
    navigate, href,
    beforeLeave: navigation.beforeLeave,
    link: () => null, // no other modules standalone
    lang: document.documentElement.lang,
  });

  const nav = manifest.nav || [];
  const TopBar = ({ path }) => html`<${Navbar} brand=${manifest.title} mark="◆"
    items=${nav.map(i => ({ href: href(i.path), title: i.title }))} current=${href(path)} />`;

  const instance = mount(root, context());
  const update = () => {
    const ctx = context();
    // The module root opens its home page.
    if (ctx.path === '/' && manifest.home && manifest.home !== '/') return navigate(manifest.home, { replace: true });
    render(html`<${TopBar} path=${ctx.path} />`, topbar);
    instance.update(ctx);
  };
  addEventListener('panel:routechange', update);
  update();
}

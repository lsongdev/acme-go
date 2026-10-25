// Runs the module on its own: a minimal host that reads manifest.json for
// the top navigation and drives mount() from the URL hash, exactly as a
// panel shell would.
import { html, render } from './lib/html.js';
import { mount } from './module.js';

const manifest = await (await fetch('manifest.json')).json();
document.title = manifest.title;

function location() {
  const raw = window.location.hash.replace(/^#/, '') || '/';
  const [path, search = ''] = raw.split('?');
  return { path: path || '/', query: Object.fromEntries(new URLSearchParams(search)) };
}

const href = (path, query) => {
  const s = new URLSearchParams(Object.entries(query || {}).filter(([, v]) => v !== '' && v != null)).toString();
  return '#' + path + (s ? '?' + s : '');
};

function navigate(to, { replace = false } = {}) {
  if (replace) history.replaceState(null, '', '#' + to);
  else history.pushState(null, '', '#' + to);
  dispatchEvent(new HashChangeEvent('hashchange'));
}

function context() {
  return {
    id: manifest.id,
    apiBase: new URL('api/', document.baseURI).href,
    assetsBase: new URL('./', document.baseURI).href,
    ...location(),
    navigate, href,
    link: () => null, // no other modules standalone
    lang: document.documentElement.lang,
  };
}

function TopBar({ path }) {
  // The longest matching entry is active; "/" only matches exactly.
  const active = (manifest.nav || [])
    .filter(i => path === i.path || (i.path !== '/' && path.startsWith(i.path + '/')))
    .sort((a, b) => b.path.length - a.path.length)[0];
  return html`
    <a class="brand" href="#/"><span class="brand-mark">◆</span>${manifest.title}</a>
    <nav>${(manifest.nav || []).map(i => html`<a class=${`nav-link ${i === active ? 'active' : ''}`} href=${href(i.path)}>${i.title}</a>`)}</nav>`;
}

const topbar = document.getElementById('topbar');
const instance = mount(document.getElementById('module'), context());
const update = () => {
  const ctx = context();
  // The module root opens its home page.
  if (ctx.path === '/' && manifest.home && manifest.home !== '/') return navigate(manifest.home, { replace: true });
  render(html`<${TopBar} path=${ctx.path} />`, topbar);
  instance.update(ctx);
};
addEventListener('hashchange', update);
addEventListener('popstate', update);
update();

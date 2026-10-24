// Runs the module on its own: a minimal host that reads manifest.json for
// the sidebar and drives mount() from the URL hash, exactly as a panel
// shell would.
import { html, render, useState } from './lib/html.js';
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

function Sidebar({ path }) {
  const [open, setOpen] = useState(false);
  const groups = [];
  for (const item of manifest.nav || []) {
    let group = groups.find(g => g.name === (item.group || ''));
    if (!group) groups.push(group = { name: item.group || '', items: [] });
    group.items.push(item);
  }
  // The longest matching entry is active; "/" only matches exactly.
  const active = (manifest.nav || [])
    .filter(i => path === i.path || (i.path !== '/' && path.startsWith(i.path + '/')))
    .sort((a, b) => b.path.length - a.path.length)[0];
  document.getElementById('layout').classList.toggle('menu-open', open);
  return html`
    <a class="brand" href="#/"><span class="brand-mark">◆</span>${manifest.title}</a>
    <button class="btn btn-ghost menu-toggle" aria-label="Menu" onClick=${() => setOpen(o => !o)}>☰</button>
    <nav class="nav" onClick=${() => setOpen(false)}>${groups.map(g => html`<div class="nav-group">
      ${g.name ? html`<div class="nav-heading">${g.name}</div>` : null}
      ${g.items.map(i => html`<a class=${`nav-link ${i === active ? 'active' : ''}`} href=${href(i.path)}>
        <span class="nav-icon">${i.icon || '•'}</span>${i.title}</a>`)}
    </div>`)}</nav>`;
}

const sidebar = document.getElementById('sidebar');
const instance = mount(document.getElementById('module'), context());
const update = () => {
  const ctx = context();
  render(html`<${Sidebar} path=${ctx.path} />`, sidebar);
  instance.update(ctx);
};
addEventListener('hashchange', update);
addEventListener('popstate', update);
update();

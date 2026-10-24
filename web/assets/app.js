import { render } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import { html, Link } from './lib.js';
import Certificates from './pages/certificates.js';
import Certificate from './pages/certificate.js';
import NewCertificate from './pages/certificate-new.js';
import Issuances from './pages/issuances.js';
import Issuance from './pages/issuance.js';
import Accounts from './pages/accounts.js';
import Providers from './pages/providers.js';
import Orders from './pages/orders.js';

const routes = [
  ['/certificates', Certificates],
  ['/certificates/new', NewCertificate],
  ['/certificates/:id', Certificate],
  ['/issuances', Issuances],
  ['/issuances/:id', Issuance],
  ['/accounts', Accounts],
  ['/providers', Providers],
  ['/orders', Orders],
];

const nav = [
  ['/certificates', '证书'],
  ['/issuances', '签发记录'],
  ['/accounts', '账户'],
  ['/providers', '服务商'],
  ['/orders', '订单'],
];

function match(path) {
  const parts = path.split('/').filter(Boolean);
  for (const [pattern, component] of routes) {
    const keys = pattern.split('/').filter(Boolean);
    if (keys.length !== parts.length) continue;
    const params = {};
    if (keys.every((key, i) => key.startsWith(':') ? (params[key.slice(1)] = decodeURIComponent(parts[i])) : key === parts[i])) {
      return { component, params };
    }
  }
  return null;
}

const currentPath = () => location.hash.replace(/^#/, '') || '/certificates';

function App() {
  const [path, setPath] = useState(currentPath);
  useEffect(() => {
    const onChange = () => { setPath(currentPath()); window.scrollTo(0, 0); };
    addEventListener('hashchange', onChange);
    return () => removeEventListener('hashchange', onChange);
  }, []);
  const route = match(path);
  const section = '/' + (path.split('/')[1] || '');
  return html`
    <div class="layout">
      <header class="sidebar">
        <div class="brand"><span class="brand-mark" aria-hidden="true"></span>ACME</div>
        <nav>
          ${nav.map(([to, label]) => html`
            <${Link} to=${to} class="nav-link" aria-current=${section === to ? 'page' : undefined}>${label}<//>`)}
        </nav>
      </header>
      <main class="content">
        ${route
          ? html`<${route.component} key=${path} ...${route.params} />`
          : html`<div class="page"><h1>页面不存在</h1><p><${Link} to="/certificates">返回证书列表<//></p></div>`}
      </main>
    </div>`;
}

render(html`<${App} />`, document.getElementById('app'));

import { html } from '../lib/html.js';
import { Page, Notice, Loading, Empty } from '../lib/ui.js';
import { usePanel } from '../lib/panel.js';
import { useApi, Link, Status, Domains, Expiry, CHALLENGES } from '../acme.js';

export default function Certificates() {
  const { href } = usePanel();
  // Refresh while any certificate is still being issued for the first time.
  const { data, error } = useApi('/certificates', items => items?.some(c => c.status === 'pending') ? 3000 : 0);
  return html`<${Page} title="证书" actions=${html`<a class="btn btn-primary" href=${href('/certificates/new')}>申请证书</a>`}>
    <${Notice} error=${error} />
    ${data === undefined ? (error ? null : html`<${Loading} />`) : data.length === 0 ? html`
      <${Empty}>
        <p>还没有证书。</p>
        <p class="muted">先在「账户」中注册一个 ACME 账户，然后申请证书。</p>
      <//>` : html`
      <div class="table-wrap"><table class="table">
        <thead><tr><th>域名</th><th>状态</th><th>颁发者</th><th>到期时间</th><th>验证方式</th><th>自动续期</th></tr></thead>
        <tbody>${data.map(c => html`<tr key=${c.id}>
          <td><${Link} to=${`/certificates/${c.id}`}><${Domains} domains=${c.domains} /><//></td>
          <td><${Status} status=${c.status} /></td>
          <td>${c.issuer || html`<span class="muted">—</span>`}</td>
          <td><${Expiry} value=${c.not_after} /></td>
          <td>${CHALLENGES[c.challenge_type] || c.challenge_type}</td>
          <td>${c.auto_renew ? '开启' : html`<span class="muted">关闭</span>`}</td>
        </tr>`)}</tbody>
      </table></div>`}
  <//>`;
}

import { html, Link, useResource, ErrorNotice, Loading, Empty, Status, Domains, Expiry, CHALLENGES } from '../lib.js';

export default function Certificates() {
  // Refresh while any certificate is still being issued for the first time.
  const { data, error, loading } = useResource('/certificates', items => items?.some(c => c.status === 'pending') ? 3000 : 0);
  return html`
    <div class="page">
      <div class="page-header">
        <h1>证书</h1>
        <${Link} to="/certificates/new" class="button button-primary">申请证书<//>
      </div>
      <${ErrorNotice} error=${error} />
      ${loading && !data ? html`<${Loading} />` : data?.length === 0 ? html`
        <${Empty}>
          <p>还没有证书。</p>
          <p class="muted">先在「账户」中注册一个 ACME 账户，然后申请证书。</p>
        <//>` : data && html`
        <div class="table-wrap">
          <table>
            <thead><tr><th>域名</th><th>状态</th><th>颁发者</th><th>到期时间</th><th>验证方式</th><th>自动续期</th></tr></thead>
            <tbody>
              ${data.map(c => html`
                <tr key=${c.id}>
                  <td><${Link} to=${`/certificates/${c.id}`}><${Domains} domains=${c.domains} /><//></td>
                  <td><${Status} status=${c.status} /></td>
                  <td>${c.issuer || html`<span class="muted">—</span>`}</td>
                  <td><${Expiry} value=${c.not_after} /></td>
                  <td>${CHALLENGES[c.challenge_type] || c.challenge_type}</td>
                  <td>${c.auto_renew ? '开启' : html`<span class="muted">关闭</span>`}</td>
                </tr>`)}
            </tbody>
          </table>
        </div>`}
    </div>`;
}

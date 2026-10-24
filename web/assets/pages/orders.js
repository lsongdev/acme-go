import { html, Link, useResource, ErrorNotice, Loading, Empty, Status, Domains, formatDate } from '../lib.js';

export default function Orders() {
  const { data, error, loading } = useResource('/orders');
  return html`
    <div class="page">
      <div class="page-header"><h1>订单</h1></div>
      <p class="muted">每次签发会在 CA 创建一个 ACME 订单。</p>
      <${ErrorNotice} error=${error} />
      ${loading && !data ? html`<${Loading} />` : data?.length === 0 ? html`<${Empty}><p>暂无订单。</p><//>` : data && html`
        <div class="table-wrap">
          <table>
            <thead><tr><th>#</th><th>域名</th><th>状态</th><th>签发</th><th>订单 URL</th><th>创建时间</th></tr></thead>
            <tbody>
              ${data.map(o => html`
                <tr key=${o.id}>
                  <td>${o.id}</td>
                  <td><${Domains} domains=${o.domains} /></td>
                  <td><${Status} status=${o.status} /></td>
                  <td>${o.issuance_id ? html`<${Link} to=${`/issuances/${o.issuance_id}`}>#${o.issuance_id}<//>` : '—'}</td>
                  <td class="mono truncate" title=${o.url}>${o.url}</td>
                  <td>${formatDate(o.created_at, true)}</td>
                </tr>`)}
            </tbody>
          </table>
        </div>`}
    </div>`;
}

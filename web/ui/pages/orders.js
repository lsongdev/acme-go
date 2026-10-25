import { html } from '../lib/html.js';
import { Page, Notice, Loading, Empty } from '../lib/ui.js';
import { useApi, Link, Status, Domains, formatDate } from '../acme.js';

export default function Orders() {
  const { data, error } = useApi('/orders');
  return html`<${Page} title="订单" subtitle="每次签发会在 CA 创建一个 ACME 订单。">
    <${Notice} error=${error} />
    ${data === undefined ? (error ? null : html`<${Loading} />`) : data.length === 0 ? html`<${Empty}>暂无订单。<//>` : html`
      <div class="table-wrap"><table class="table">
        <thead><tr><th>#</th><th>域名</th><th>状态</th><th>签发</th><th>订单 URL</th><th>创建时间</th></tr></thead>
        <tbody>${data.map(o => html`<tr key=${o.id}>
          <td>${o.id}</td>
          <td><${Domains} domains=${o.domains} /></td>
          <td><${Status} status=${o.status} /></td>
          <td>${o.issuance_id ? html`<${Link} to=${`/issuances/${o.issuance_id}`}>#${o.issuance_id}<//>` : '—'}</td>
          <td class="mono truncate" style="max-width:320px" title=${o.url}>${o.url}</td>
          <td class="nowrap">${formatDate(o.created_at, true)}</td>
        </tr>`)}</tbody>
      </table></div>`}
  <//>`;
}

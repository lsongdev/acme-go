import { html } from '../lib/html.js';
import { Page, Notice, Loading, Empty, Table } from '../lib/ui.js';
import { useApi, Link, Status, Domains, formatDate } from '../acme.js';

export default function Orders() {
  const { data, error } = useApi('/orders');
  return html`<${Page} title="Orders" subtitle="Each issuance creates an ACME order at the CA.">
    <${Notice} error=${error} />
    ${data === undefined ? (error ? null : html`<${Loading} />`) : data.length === 0 ? html`<${Empty}>No orders yet.<//>` : html`
      <${Table}>
        <thead><tr><th>#</th><th>Domains</th><th>Status</th><th>Issuance</th><th>Order URL</th><th>Created</th></tr></thead>
        <tbody>${data.map(o => html`<tr key=${o.id}>
          <td>${o.id}</td>
          <td><${Domains} domains=${o.domains} /></td>
          <td><${Status} status=${o.status} /></td>
          <td>${o.issuance_id ? html`<${Link} to=${`/issuances/${o.issuance_id}`}>#${o.issuance_id}<//>` : '—'}</td>
          <td class="mono truncate" style="max-width:320px" title=${o.url}>${o.url}</td>
          <td class="nowrap">${formatDate(o.created_at, true)}</td>
        </tr>`)}</tbody>
      <//>`}
  <//>`;
}

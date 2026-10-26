import { html } from '../lib/html.js';
import { Page, Notice, Loading, Empty, Table } from '../lib/ui.js';
import { usePanel } from '../lib/panel.js';
import { useApi, Link, Status, Domains, Expiry, CHALLENGES } from '../acme.js';

export default function Certificates() {
  const { href } = usePanel();
  // Refresh while any certificate is still being issued for the first time.
  const { data, error } = useApi('/certificates', items => items?.some(c => c.status === 'pending') ? 3000 : 0);
  return html`<${Page} title="Certificates" actions=${html`<a class="button button-primary" href=${href('/certificates/new')}>Request certificate</a>`}>
    <${Notice} error=${error} />
    ${data === undefined ? (error ? null : html`<${Loading} />`) : data.length === 0 ? html`
      <${Empty}>
        <p>No certificates yet.</p>
        <p class="muted">Register an ACME account under Accounts first, then request a certificate.</p>
      <//>` : html`
      <${Table}>
        <thead><tr><th>Domains</th><th>Status</th><th>Issuer</th><th>Expires</th><th>Challenge</th><th>Auto-renew</th></tr></thead>
        <tbody>${data.map(c => html`<tr key=${c.id}>
          <td><${Link} to=${`/certificates/${c.id}`}><${Domains} domains=${c.domains} /><//></td>
          <td><${Status} status=${c.status} /></td>
          <td>${c.issuer || html`<span class="muted">—</span>`}</td>
          <td><${Expiry} value=${c.not_after} /></td>
          <td>${CHALLENGES[c.challenge_type] || c.challenge_type}</td>
          <td>${c.auto_renew ? 'On' : html`<span class="muted">Off</span>`}</td>
        </tr>`)}</tbody>
      <//>`}
  <//>`;
}

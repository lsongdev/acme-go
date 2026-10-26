import { html } from '../lib/html.js';
import { Page, Notice, Loading, Empty, Table } from '../lib/ui.js';
import { useApi, Link, Status, Domains, formatDate, stepLabel, unfinished, CHALLENGES } from '../acme.js';

export default function Issuances() {
  const { data, error } = useApi('/issuances', items => items?.some(unfinished) ? 3000 : 0);
  return html`<${Page} title="Issuances">
    <${Notice} error=${error} />
    ${data === undefined ? (error ? null : html`<${Loading} />`) : data.length === 0 ? html`<${Empty}>No issuances yet.<//>` : html`
      <${Table}>
        <thead><tr><th>#</th><th>Domains</th><th>Status</th><th>Step</th><th>Challenge</th><th>Started</th><th>Error</th></tr></thead>
        <tbody>${data.map(i => html`<tr key=${i.id}>
          <td><${Link} to=${`/issuances/${i.id}`}>#${i.id}<//></td>
          <td><${Link} to=${`/certificates/${i.certificate_id}`}><${Domains} domains=${i.domains} /><//></td>
          <td><${Status} status=${i.status} /></td>
          <td>${stepLabel(i.step)}</td>
          <td>${CHALLENGES[i.challenge_type] || i.challenge_type}</td>
          <td>${formatDate(i.created_at, true)}</td>
          <td class="error-cell" title=${i.error}>${i.error || ''}</td>
        </tr>`)}</tbody>
      <//>`}
  <//>`;
}

import { html, useState } from '../lib/html.js';
import { useAction } from '../lib/hooks.js';
import { Page, Notice, Loading, Field, KV, CopyButton, confirmDanger, Table } from '../lib/ui.js';
import { usePanel } from '../lib/panel.js';
import { useApi, Link, Status, Expiry, formatDate, stepLabel, unfinished, CHALLENGES } from '../acme.js';

// RFC 5280 revocation reasons accepted by ACME CAs.
const REASONS = [
  [0, 'Unspecified'],
  [1, 'Key compromise'],
  [3, 'Affiliation changed'],
  [4, 'Superseded'],
  [5, 'Cessation of operation'],
];

export default function Certificate({ params: { id } }) {
  const { api, client, href, navigate } = usePanel();
  const issuances = useApi(`/issuances?certificate_id=${id}`, items => items?.some(unfinished) ? 3000 : 0);
  const busyIssuing = issuances.data?.some(unfinished);
  const certificate = useApi(`/certificates/${id}`, c => c?.status === 'pending' || busyIssuing ? 3000 : 0);
  const accounts = useApi('/accounts');
  const action = useAction();
  const [revoking, setRevoking] = useState(false);
  const [reason, setReason] = useState(0);

  const c = certificate.data;
  const back = html`<a class="button" href=${href('/certificates')}>Back</a>`;
  if (!c) {
    return html`<${Page} title="Certificate" actions=${back}>
      ${certificate.error ? html`<${Notice} error=${certificate.error} />` : html`<${Loading} />`}
    <//>`;
  }

  const reload = () => { certificate.reload(); issuances.reload(); };
  const account = accounts.data?.find(a => a.id === c.account_id);
  const download = format => client.url(`certificates/${c.id}/download`, { format }).href;

  const renew = () => action.run(async () => {
    const issuance = await api.post(`/certificates/${c.id}/renew`);
    navigate(`/issuances/${issuance.id}`);
  });
  // Reload even on failure so the controls snap back to the saved values.
  const update = patch => action.run(() => api.patch(`/certificates/${c.id}`, patch)).then(reload);
  const revoke = () => action.run(async () => {
    await api.post(`/certificates/${c.id}/revoke`, { reason: Number(reason) });
    setRevoking(false);
    reload();
  });
  const remove = () => {
    if (!confirmDanger(`Delete the certificate ${c.domains[0]}? The certificate and its private key are deleted for good.`)) return;
    action.run(async () => {
      await api.del(`/certificates/${c.id}`);
      navigate('/certificates');
    });
  };

  return html`<${Page} title=${html`<span class="mono">${c.domains[0]}</span>`} actions=${html`
      ${c.status !== 'revoked' ? html`
        <button class="button button-primary" onClick=${renew} disabled=${action.busy || busyIssuing}>
          ${c.status === 'active' ? 'Renew now' : 'Issue again'}
        </button>` : null}
      ${c.cert_pem ? html`<a class="button" href=${download('zip')} download>Download</a>` : null}
      ${back}`}>
    <${Notice} error=${action.error || certificate.error} />
    ${busyIssuing ? html`<${Notice} tone="info">
      Issuing: <${Link} to=${`/issuances/${issuances.data.find(unfinished).id}`}>view progress<//>
    <//>` : null}

    <div class="card">
      <${KV} items=${[
        ['Status', html`<${Status} status=${c.status} />`],
        ['Domains', c.domains.map(d => html`<div class="mono">${d}</div>`)],
        ['Expires', html`<${Expiry} value=${c.not_after} />`],
        ['Valid from', formatDate(c.not_before, true)],
        ['Issuer', c.issuer || '—'],
        ['Serial number', html`<span class="mono">${c.serial_number || '—'}</span>`],
        ['Created', formatDate(c.created_at, true)],
      ]} />
    </div>

    <div class="card"><h2>Renewal</h2>
      <div class="form-grid">
        <${Field} label="ACME account">
          <select class="input" value=${c.account_id} disabled=${action.busy}
            onChange=${e => update({ account_id: Number(e.currentTarget.value) })}>
            ${!account ? html`<option value=${c.account_id}>Account #${c.account_id}</option>` : null}
            ${(accounts.data || []).filter(a => a.status === 'valid' || a.id === c.account_id).map(a => html`
              <option value=${a.id}>${a.provider_name} · ${a.email || `Account #${a.id}`}</option>`)}
          </select>
        <//>
        <${Field} label="Challenge">
          <select class="input" value=${c.challenge_type} disabled=${action.busy}
            onChange=${e => update({ challenge_type: e.currentTarget.value })}>
            ${Object.entries(CHALLENGES).map(([value, label]) => html`<option value=${value}>${label}</option>`)}
          </select>
        <//>
      </div>
      <label class="field-check" style="margin-top:12px">
        <input type="checkbox" checked=${c.auto_renew} disabled=${action.busy || c.status === 'revoked'}
          onChange=${e => update({ auto_renew: e.currentTarget.checked })} />Auto-renew
      </label>
    </div>

    ${c.cert_pem ? html`<div class="card">
      <div class="toolbar"><h2 style="margin:0">Certificate chain (PEM)</h2><span class="spacer"></span>
        <${CopyButton} text=${c.cert_pem} />
        <a class="button button-small" href=${download('fullchain')} download>fullchain.pem</a>
        <a class="button button-small" href=${download('key')} download>privkey.pem</a>
      </div>
      <pre class="pem">${c.cert_pem}</pre>
    </div>` : null}

    <div class="card"><h2>Issuances</h2>
      ${issuances.data?.length ? html`<${Table} framed=${false}>
        <thead><tr><th>#</th><th>Status</th><th>Step</th><th>Started</th><th>Error</th></tr></thead>
        <tbody>${issuances.data.map(i => html`<tr key=${i.id}>
          <td><${Link} to=${`/issuances/${i.id}`}>#${i.id}<//></td>
          <td><${Status} status=${i.status} /></td>
          <td>${stepLabel(i.step)}</td>
          <td>${formatDate(i.created_at, true)}</td>
          <td class="error-cell" title=${i.error}>${i.error || ''}</td>
        </tr>`)}</tbody>
      <//>` : html`<p class="muted">None yet</p>`}
    </div>

    <div class="card card-danger"><h2>Danger zone</h2>
      ${c.status === 'active' ? (revoking ? html`<div class="toolbar">
        <${Field} label="Reason">
          <select class="input" value=${reason} onChange=${e => setReason(e.currentTarget.value)}>
            ${REASONS.map(([value, label]) => html`<option value=${value}>${label}</option>`)}
          </select>
        <//>
        <button class="button button-danger" onClick=${revoke} disabled=${action.busy}>Revoke</button>
        <button class="button" onClick=${() => setRevoking(false)}>Cancel</button>
      </div>` : html`<div class="danger-row">
        <div><strong>Revoke the certificate</strong><p class="muted">Ask the CA to revoke it and turn auto-renew off. This cannot be undone.</p></div>
        <button class="button button-danger" onClick=${() => setRevoking(true)}>Revoke…</button>
      </div>`) : null}
      <div class="danger-row">
        <div><strong>Delete the certificate</strong><p class="muted">Delete the certificate, its private key and issuances. This does not revoke it.</p></div>
        <button class="button button-danger" onClick=${remove} disabled=${action.busy || busyIssuing}>Delete</button>
      </div>
    </div>
  <//>`;
}

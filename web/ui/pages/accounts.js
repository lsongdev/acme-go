import { html, useEffect, useState } from '../lib/html.js';
import { useAction } from '../lib/hooks.js';
import { Page, Notice, Loading, Empty, Field, confirmDanger, Table } from '../lib/ui.js';
import { usePanel } from '../lib/panel.js';
import { useApi, Status, formatDate } from '../acme.js';

function RegisterForm({ providers, onDone }) {
  const { api } = usePanel();
  const [providerID, setProviderID] = useState(providers[0]?.id ?? '');
  const [email, setEmail] = useState('');
  const [agree, setAgree] = useState(false);
  const [directory, setDirectory] = useState(null);
  const lookup = useAction();
  const submit = useAction();

  // Show the provider's terms of service before registering.
  useEffect(() => {
    setDirectory(null);
    setAgree(false);
    if (!providerID) return;
    lookup.run(async () => setDirectory(await api.get(`/providers/${providerID}/directory`)));
  }, [providerID]);

  const register = event => {
    event.preventDefault();
    submit.run(async () => {
      await api.post('/accounts', { provider_id: Number(providerID), email, agree_terms: agree });
      onDone();
    });
  };
  const needsTerms = !!directory?.terms_of_service;

  return html`<form class="card form" onSubmit=${register}>
    <h2>Register an account</h2>
    <${Notice} error=${lookup.error || submit.error} />
    <div class="form-grid">
      <${Field} label="Provider" required>
        <select class="input" value=${providerID} onChange=${e => setProviderID(e.currentTarget.value)} required>
          ${providers.map(p => html`<option value=${p.id}>${p.name}</option>`)}
        </select>
      <//>
      <${Field} label="Contact email" hint="Optional. The CA sends notices about certificates to it.">
        <input class="input" type="email" value=${email} placeholder="admin@example.com" onInput=${e => setEmail(e.currentTarget.value)} />
      <//>
    </div>
    ${lookup.busy ? html`<p class="muted">Reading the provider…</p>` : null}
    ${directory?.external_account_required ? html`<${Notice} tone="warn">This provider requires external account binding (EAB), which is not supported yet.<//>` : null}
    ${needsTerms ? html`<label class="field-check">
      <input type="checkbox" checked=${agree} onChange=${e => setAgree(e.currentTarget.checked)} required />
      <span>I agree to the <a href=${directory.terms_of_service} target="_blank" rel="noopener noreferrer">terms of service</a></span>
    </label>` : null}
    <div class="form-actions">
      <button class="button button-primary" type="submit"
        disabled=${submit.busy || lookup.busy || !directory || directory.external_account_required || (needsTerms && !agree)}>
        ${submit.busy ? 'Registering…' : 'Register'}
      </button>
      <button class="button" type="button" onClick=${onDone}>Cancel</button>
    </div>
  </form>`;
}

export default function Accounts() {
  const { api } = usePanel();
  const accounts = useApi('/accounts');
  const providers = useApi('/providers');
  const action = useAction();
  const [registering, setRegistering] = useState(false);

  const deactivate = account => {
    if (!confirmDanger(`Deactivate the account ${account.email || '#' + account.id} at ${account.provider_name}? This cannot be undone, and the account can no longer issue certificates.`)) return;
    action.run(async () => { await api.post(`/accounts/${account.id}/deactivate`); accounts.reload(); });
  };
  const remove = account => {
    if (!confirmDanger(`Delete the account ${account.email || '#' + account.id} here? Its private key is deleted; the account at the CA is unaffected.`)) return;
    action.run(async () => { await api.del(`/accounts/${account.id}`); accounts.reload(); });
  };

  const data = accounts.data;
  return html`<${Page} title="Accounts" actions=${registering ? null : html`
      <button class="button button-primary" onClick=${() => setRegistering(true)} disabled=${!providers.data?.length}>Register account</button>`}>
    <${Notice} error=${accounts.error || providers.error || action.error} />
    ${registering && providers.data ? html`<${RegisterForm} providers=${providers.data} onDone=${() => { setRegistering(false); accounts.reload(); }} />` : null}
    ${data === undefined ? (accounts.error ? null : html`<${Loading} />`) : data.length === 0 ? html`
      <${Empty}><p>No ACME accounts yet.</p><p class="muted">Register an account with a provider before issuing certificates.</p><//>` : html`
      <div class="section"><${Table}>
        <thead><tr><th>Provider</th><th>Email</th><th>Status</th><th>Account URL</th><th>Created</th><th class="actions"></th></tr></thead>
        <tbody>${data.map(a => html`<tr key=${a.id}>
          <td>${a.provider_name}</td>
          <td>${a.email || html`<span class="muted">—</span>`}</td>
          <td><${Status} status=${a.status} /></td>
          <td class="mono truncate" style="max-width:280px" title=${a.url}>${a.url}</td>
          <td class="nowrap">${formatDate(a.created_at, true)}</td>
          <td class="actions"><span class="button-row">
            ${a.status === 'valid' ? html`<button class="button button-small" onClick=${() => deactivate(a)} disabled=${action.busy}>Deactivate</button>` : null}
            <button class="button button-small button-danger" onClick=${() => remove(a)} disabled=${action.busy}>Delete</button>
          </span></td>
        </tr>`)}</tbody>
      <//></div>`}
  <//>`;
}

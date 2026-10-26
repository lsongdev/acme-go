import { html, useState } from '../lib/html.js';
import { useAction } from '../lib/hooks.js';
import { Page, Notice, Loading, Field, confirmDanger, Table } from '../lib/ui.js';
import { usePanel } from '../lib/panel.js';
import { useApi, formatDate } from '../acme.js';

function AddForm({ onDone }) {
  const { api } = usePanel();
  const [name, setName] = useState('');
  const [directoryURL, setDirectoryURL] = useState('');
  const { busy, error, run } = useAction();
  const submit = event => {
    event.preventDefault();
    run(async () => {
      await api.post('/providers', { name, directory_url: directoryURL });
      onDone();
    });
  };
  return html`<form class="card form" onSubmit=${submit}>
    <h2>Add a provider</h2>
    <${Notice} error=${error} />
    <div class="form-grid">
      <${Field} label="Name" required>
        <input class="input" value=${name} required onInput=${e => setName(e.currentTarget.value)} placeholder="My CA" />
      <//>
      <${Field} label="ACME directory URL" required>
        <input class="input" type="url" value=${directoryURL} required onInput=${e => setDirectoryURL(e.currentTarget.value)}
          placeholder="https://acme.example.com/directory" />
      <//>
    </div>
    <div class="form-actions">
      <button class="button button-primary" type="submit" disabled=${busy}>Add</button>
      <button class="button" type="button" onClick=${onDone}>Cancel</button>
    </div>
  </form>`;
}

export default function Providers() {
  const { api } = usePanel();
  const { data, error, reload } = useApi('/providers');
  const action = useAction();
  const [adding, setAdding] = useState(false);
  const remove = provider => {
    if (!confirmDanger(`Delete the provider ${provider.name}?`)) return;
    action.run(async () => { await api.del(`/providers/${provider.id}`); reload(); });
  };
  return html`<${Page} title="Providers" actions=${adding ? null : html`<button class="button button-primary" onClick=${() => setAdding(true)}>Add provider</button>`}>
    <${Notice} error=${error || action.error} />
    ${adding ? html`<${AddForm} onDone=${() => { setAdding(false); reload(); }} />` : null}
    ${data === undefined ? (error ? null : html`<${Loading} />`) : html`
      <div class="section"><${Table}>
        <thead><tr><th>Name</th><th>Directory URL</th><th>Added</th><th class="actions"></th></tr></thead>
        <tbody>${data.map(p => html`<tr key=${p.id}>
          <td>${p.name}</td>
          <td class="mono truncate" style="max-width:360px" title=${p.directory_url}>${p.directory_url}</td>
          <td class="nowrap">${formatDate(p.created_at)}</td>
          <td class="actions"><button class="button button-small button-danger" onClick=${() => remove(p)} disabled=${action.busy}>Delete</button></td>
        </tr>`)}</tbody>
      <//></div>`}
  <//>`;
}

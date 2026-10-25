import { html, useState } from '../lib/html.js';
import { useAction } from '../lib/hooks.js';
import { Page, Notice, Loading, Field, confirmDanger } from '../lib/ui.js';
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
    <h2>添加服务商</h2>
    <${Notice} error=${error} />
    <div class="form-grid">
      <${Field} label="名称" required>
        <input class="input" value=${name} required onInput=${e => setName(e.currentTarget.value)} placeholder="My CA" />
      <//>
      <${Field} label="ACME 目录 URL" required>
        <input class="input" type="url" value=${directoryURL} required onInput=${e => setDirectoryURL(e.currentTarget.value)}
          placeholder="https://acme.example.com/directory" />
      <//>
    </div>
    <div class="form-actions">
      <button class="btn btn-primary" type="submit" disabled=${busy}>添加</button>
      <button class="btn" type="button" onClick=${onDone}>取消</button>
    </div>
  </form>`;
}

export default function Providers() {
  const { api } = usePanel();
  const { data, error, reload } = useApi('/providers');
  const action = useAction();
  const [adding, setAdding] = useState(false);
  const remove = provider => {
    if (!confirmDanger(`删除服务商 ${provider.name}？`)) return;
    action.run(async () => { await api.del(`/providers/${provider.id}`); reload(); });
  };
  return html`<${Page} title="服务商" actions=${adding ? null : html`<button class="btn btn-primary" onClick=${() => setAdding(true)}>添加服务商</button>`}>
    <${Notice} error=${error || action.error} />
    ${adding ? html`<${AddForm} onDone=${() => { setAdding(false); reload(); }} />` : null}
    ${data === undefined ? (error ? null : html`<${Loading} />`) : html`
      <div class="table-wrap section"><table class="table">
        <thead><tr><th>名称</th><th>目录 URL</th><th>添加时间</th><th class="actions"></th></tr></thead>
        <tbody>${data.map(p => html`<tr key=${p.id}>
          <td>${p.name}</td>
          <td class="mono truncate" style="max-width:360px" title=${p.directory_url}>${p.directory_url}</td>
          <td class="nowrap">${formatDate(p.created_at)}</td>
          <td class="actions"><button class="btn btn-sm btn-danger" onClick=${() => remove(p)} disabled=${action.busy}>删除</button></td>
        </tr>`)}</tbody>
      </table></div>`}
  <//>`;
}

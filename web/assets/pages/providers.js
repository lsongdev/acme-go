import { useState } from 'preact/hooks';
import { html, api, useResource, useAction, ErrorNotice, Loading, formatDate } from '../lib.js';

function AddForm({ onDone }) {
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
  return html`
    <form class="card form" onSubmit=${submit}>
      <h2>添加服务商</h2>
      <${ErrorNotice} error=${error} />
      <label class="field">
        <span class="label">名称</span>
        <input value=${name} required onInput=${e => setName(e.currentTarget.value)} placeholder="My CA" />
      </label>
      <label class="field">
        <span class="label">ACME 目录 URL</span>
        <input type="url" value=${directoryURL} required onInput=${e => setDirectoryURL(e.currentTarget.value)}
          placeholder="https://acme.example.com/directory" />
      </label>
      <div class="actions">
        <button class="button button-primary" type="submit" disabled=${busy}>添加</button>
        <button class="button" type="button" onClick=${onDone}>取消</button>
      </div>
    </form>`;
}

export default function Providers() {
  const { data, error, loading, reload } = useResource('/providers');
  const action = useAction();
  const [adding, setAdding] = useState(false);
  const remove = provider => {
    if (!confirm(`删除服务商 ${provider.name}？`)) return;
    action.run(async () => { await api.del(`/providers/${provider.id}`); reload(); });
  };
  return html`
    <div class="page">
      <div class="page-header">
        <h1>服务商</h1>
        ${!adding && html`<button class="button button-primary" onClick=${() => setAdding(true)}>添加服务商</button>`}
      </div>
      <${ErrorNotice} error=${error || action.error} />
      ${adding && html`<${AddForm} onDone=${() => { setAdding(false); reload(); }} />`}
      ${loading && !data ? html`<${Loading} />` : data && html`
        <div class="table-wrap">
          <table>
            <thead><tr><th>名称</th><th>目录 URL</th><th>添加时间</th><th></th></tr></thead>
            <tbody>
              ${data.map(p => html`
                <tr key=${p.id}>
                  <td>${p.name}</td>
                  <td class="mono truncate" title=${p.directory_url}>${p.directory_url}</td>
                  <td>${formatDate(p.created_at)}</td>
                  <td class="row-actions">
                    <button class="button button-small button-danger" onClick=${() => remove(p)} disabled=${action.busy}>删除</button>
                  </td>
                </tr>`)}
            </tbody>
          </table>
        </div>`}
    </div>`;
}

import { useEffect, useState } from 'preact/hooks';
import { html, api, useResource, useAction, ErrorNotice, Loading, Empty, Status, formatDate } from '../lib.js';

function RegisterForm({ providers, onDone }) {
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

  return html`
    <form class="card form" onSubmit=${register}>
      <h2>注册账户</h2>
      <${ErrorNotice} error=${lookup.error || submit.error} />
      <label class="field">
        <span class="label">服务商</span>
        <select value=${providerID} onChange=${e => setProviderID(e.currentTarget.value)} required>
          ${providers.map(p => html`<option value=${p.id}>${p.name}</option>`)}
        </select>
      </label>
      <label class="field">
        <span class="label">联系邮箱</span>
        <input type="email" value=${email} placeholder="admin@example.com" onInput=${e => setEmail(e.currentTarget.value)} />
        <span class="hint">可选。CA 会向此邮箱发送证书相关通知。</span>
      </label>
      ${lookup.busy && html`<p class="muted">正在读取服务商信息…</p>`}
      ${directory?.external_account_required && html`
        <div class="notice notice-warn">该服务商要求外部账户绑定（EAB），目前不支持。</div>`}
      ${needsTerms && html`
        <label class="choice">
          <input type="checkbox" checked=${agree} onChange=${e => setAgree(e.currentTarget.checked)} required />
          <span>我已阅读并同意 <a href=${directory.terms_of_service} target="_blank" rel="noopener noreferrer">服务条款</a></span>
        </label>`}
      <div class="actions">
        <button class="button button-primary" type="submit"
          disabled=${submit.busy || lookup.busy || !directory || directory.external_account_required || (needsTerms && !agree)}>
          ${submit.busy ? '正在注册…' : '注册'}
        </button>
        <button class="button" type="button" onClick=${onDone}>取消</button>
      </div>
    </form>`;
}

export default function Accounts() {
  const accounts = useResource('/accounts');
  const providers = useResource('/providers');
  const action = useAction();
  const [registering, setRegistering] = useState(false);

  const deactivate = account => {
    if (!confirm(`在 ${account.provider_name} 停用账户 ${account.email || '#' + account.id}？停用后无法恢复，也不能再用它签发证书。`)) return;
    action.run(async () => { await api.post(`/accounts/${account.id}/deactivate`); accounts.reload(); });
  };
  const remove = account => {
    if (!confirm(`从本地删除账户 ${account.email || '#' + account.id}？账户私钥将被删除，CA 处的账户不受影响。`)) return;
    action.run(async () => { await api.del(`/accounts/${account.id}`); accounts.reload(); });
  };

  const data = accounts.data;
  return html`
    <div class="page">
      <div class="page-header">
        <h1>账户</h1>
        ${!registering && html`<button class="button button-primary" onClick=${() => setRegistering(true)} disabled=${!providers.data?.length}>注册账户</button>`}
      </div>
      <${ErrorNotice} error=${accounts.error || providers.error || action.error} />
      ${registering && providers.data && html`
        <${RegisterForm} providers=${providers.data} onDone=${() => { setRegistering(false); accounts.reload(); }} />`}
      ${accounts.loading && !data ? html`<${Loading} />` : data?.length === 0 ? html`
        <${Empty}><p>还没有 ACME 账户。</p><p class="muted">签发证书前需要先在服务商处注册账户。</p><//>` : data && html`
        <div class="table-wrap">
          <table>
            <thead><tr><th>服务商</th><th>邮箱</th><th>状态</th><th>账户 URL</th><th>创建时间</th><th></th></tr></thead>
            <tbody>
              ${data.map(a => html`
                <tr key=${a.id}>
                  <td>${a.provider_name}</td>
                  <td>${a.email || html`<span class="muted">—</span>`}</td>
                  <td><${Status} status=${a.status} /></td>
                  <td class="mono truncate" title=${a.url}>${a.url}</td>
                  <td>${formatDate(a.created_at, true)}</td>
                  <td class="row-actions">
                    ${a.status === 'valid' && html`<button class="button button-small" onClick=${() => deactivate(a)} disabled=${action.busy}>停用</button>`}
                    <button class="button button-small button-danger" onClick=${() => remove(a)} disabled=${action.busy}>删除</button>
                  </td>
                </tr>`)}
            </tbody>
          </table>
        </div>`}
    </div>`;
}

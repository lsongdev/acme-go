import { html, useEffect, useState } from '../lib/html.js';
import { useAction } from '../lib/hooks.js';
import { Page, Notice, Loading, Empty, Field, confirmDanger } from '../lib/ui.js';
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
    <h2>注册账户</h2>
    <${Notice} error=${lookup.error || submit.error} />
    <div class="form-grid">
      <${Field} label="服务商" required>
        <select class="input" value=${providerID} onChange=${e => setProviderID(e.currentTarget.value)} required>
          ${providers.map(p => html`<option value=${p.id}>${p.name}</option>`)}
        </select>
      <//>
      <${Field} label="联系邮箱" hint="可选。CA 会向此邮箱发送证书相关通知。">
        <input class="input" type="email" value=${email} placeholder="admin@example.com" onInput=${e => setEmail(e.currentTarget.value)} />
      <//>
    </div>
    ${lookup.busy ? html`<p class="muted">正在读取服务商信息…</p>` : null}
    ${directory?.external_account_required ? html`<${Notice} tone="warn">该服务商要求外部账户绑定（EAB），目前不支持。<//>` : null}
    ${needsTerms ? html`<label class="check">
      <input type="checkbox" checked=${agree} onChange=${e => setAgree(e.currentTarget.checked)} required />
      <span>我已阅读并同意 <a href=${directory.terms_of_service} target="_blank" rel="noopener noreferrer">服务条款</a></span>
    </label>` : null}
    <div class="form-actions">
      <button class="btn btn-primary" type="submit"
        disabled=${submit.busy || lookup.busy || !directory || directory.external_account_required || (needsTerms && !agree)}>
        ${submit.busy ? '正在注册…' : '注册'}
      </button>
      <button class="btn" type="button" onClick=${onDone}>取消</button>
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
    if (!confirmDanger(`在 ${account.provider_name} 停用账户 ${account.email || '#' + account.id}？停用后无法恢复，也不能再用它签发证书。`)) return;
    action.run(async () => { await api.post(`/accounts/${account.id}/deactivate`); accounts.reload(); });
  };
  const remove = account => {
    if (!confirmDanger(`从本地删除账户 ${account.email || '#' + account.id}？账户私钥将被删除，CA 处的账户不受影响。`)) return;
    action.run(async () => { await api.del(`/accounts/${account.id}`); accounts.reload(); });
  };

  const data = accounts.data;
  return html`<${Page} title="账户" actions=${registering ? null : html`
      <button class="btn btn-primary" onClick=${() => setRegistering(true)} disabled=${!providers.data?.length}>注册账户</button>`}>
    <${Notice} error=${accounts.error || providers.error || action.error} />
    ${registering && providers.data ? html`<${RegisterForm} providers=${providers.data} onDone=${() => { setRegistering(false); accounts.reload(); }} />` : null}
    ${data === undefined ? (accounts.error ? null : html`<${Loading} />`) : data.length === 0 ? html`
      <${Empty}><p>还没有 ACME 账户。</p><p class="muted">签发证书前需要先在服务商处注册账户。</p><//>` : html`
      <div class="table-wrap section"><table class="table">
        <thead><tr><th>服务商</th><th>邮箱</th><th>状态</th><th>账户 URL</th><th>创建时间</th><th class="actions"></th></tr></thead>
        <tbody>${data.map(a => html`<tr key=${a.id}>
          <td>${a.provider_name}</td>
          <td>${a.email || html`<span class="muted">—</span>`}</td>
          <td><${Status} status=${a.status} /></td>
          <td class="mono truncate" style="max-width:280px" title=${a.url}>${a.url}</td>
          <td class="nowrap">${formatDate(a.created_at, true)}</td>
          <td class="actions"><span class="btn-group">
            ${a.status === 'valid' ? html`<button class="btn btn-sm" onClick=${() => deactivate(a)} disabled=${action.busy}>停用</button>` : null}
            <button class="btn btn-sm btn-danger" onClick=${() => remove(a)} disabled=${action.busy}>删除</button>
          </span></td>
        </tr>`)}</tbody>
      </table></div>`}
  <//>`;
}

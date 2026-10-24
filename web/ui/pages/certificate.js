import { html, useState } from '../lib/html.js';
import { useAction } from '../lib/hooks.js';
import { Page, Notice, Loading, Field, KV, CopyButton, confirmDanger } from '../lib/ui.js';
import { usePanel } from '../lib/panel.js';
import { useApi, Link, Status, Expiry, formatDate, stepLabel, unfinished, CHALLENGES } from '../acme.js';

// RFC 5280 revocation reasons accepted by ACME CAs.
const REASONS = [
  [0, '未指定'],
  [1, '私钥泄露'],
  [3, '信息变更'],
  [4, '已被替代'],
  [5, '停止运营'],
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
  const back = html`<a class="btn" href=${href('/certificates')}>返回</a>`;
  if (!c) {
    return html`<${Page} title="证书" actions=${back}>
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
    if (!confirmDanger(`删除证书 ${c.domains[0]}？证书和私钥将从数据库中永久删除。`)) return;
    action.run(async () => {
      await api.del(`/certificates/${c.id}`);
      navigate('/certificates');
    });
  };

  return html`<${Page} title=${html`<span class="mono">${c.domains[0]}</span>`} actions=${html`
      ${c.status !== 'revoked' ? html`
        <button class="btn btn-primary" onClick=${renew} disabled=${action.busy || busyIssuing}>
          ${c.status === 'active' ? '立即续期' : '重新签发'}
        </button>` : null}
      ${c.cert_pem ? html`<a class="btn" href=${download('zip')} download>下载</a>` : null}
      ${back}`}>
    <${Notice} error=${action.error || certificate.error} />
    ${busyIssuing ? html`<${Notice} tone="info">
      正在签发：<${Link} to=${`/issuances/${issuances.data.find(unfinished).id}`}>查看进度<//>
    <//>` : null}

    <div class="card">
      <${KV} items=${[
        ['状态', html`<${Status} status=${c.status} />`],
        ['域名', c.domains.map(d => html`<div class="mono">${d}</div>`)],
        ['到期时间', html`<${Expiry} value=${c.not_after} />`],
        ['生效时间', formatDate(c.not_before, true)],
        ['颁发者', c.issuer || '—'],
        ['序列号', html`<span class="mono">${c.serial_number || '—'}</span>`],
        ['创建时间', formatDate(c.created_at, true)],
      ]} />
    </div>

    <div class="card"><h2>续期设置</h2>
      <div class="form-grid">
        <${Field} label="ACME 账户">
          <select class="input" value=${c.account_id} disabled=${action.busy}
            onChange=${e => update({ account_id: Number(e.currentTarget.value) })}>
            ${!account ? html`<option value=${c.account_id}>账户 #${c.account_id}</option>` : null}
            ${(accounts.data || []).filter(a => a.status === 'valid' || a.id === c.account_id).map(a => html`
              <option value=${a.id}>${a.provider_name} · ${a.email || `账户 #${a.id}`}</option>`)}
          </select>
        <//>
        <${Field} label="验证方式">
          <select class="input" value=${c.challenge_type} disabled=${action.busy}
            onChange=${e => update({ challenge_type: e.currentTarget.value })}>
            ${Object.entries(CHALLENGES).map(([value, label]) => html`<option value=${value}>${label}</option>`)}
          </select>
        <//>
      </div>
      <label class="check" style="margin-top:12px">
        <input type="checkbox" checked=${c.auto_renew} disabled=${action.busy || c.status === 'revoked'}
          onChange=${e => update({ auto_renew: e.currentTarget.checked })} />自动续期
      </label>
    </div>

    ${c.cert_pem ? html`<div class="card">
      <div class="toolbar"><h2 style="margin:0">证书链（PEM）</h2><span class="spacer"></span>
        <${CopyButton} text=${c.cert_pem} />
        <a class="btn btn-sm" href=${download('fullchain')} download>fullchain.pem</a>
        <a class="btn btn-sm" href=${download('key')} download>privkey.pem</a>
      </div>
      <pre class="pem">${c.cert_pem}</pre>
    </div>` : null}

    <div class="card"><h2>签发记录</h2>
      ${issuances.data?.length ? html`<table class="table">
        <thead><tr><th>#</th><th>状态</th><th>步骤</th><th>开始时间</th><th>错误</th></tr></thead>
        <tbody>${issuances.data.map(i => html`<tr key=${i.id}>
          <td><${Link} to=${`/issuances/${i.id}`}>#${i.id}<//></td>
          <td><${Status} status=${i.status} /></td>
          <td>${stepLabel(i.step)}</td>
          <td>${formatDate(i.created_at, true)}</td>
          <td class="error-cell" title=${i.error}>${i.error || ''}</td>
        </tr>`)}</tbody>
      </table>` : html`<p class="muted">暂无记录</p>`}
    </div>

    <div class="card card-danger"><h2>危险操作</h2>
      ${c.status === 'active' ? (revoking ? html`<div class="toolbar">
        <${Field} label="吊销原因">
          <select class="input" value=${reason} onChange=${e => setReason(e.currentTarget.value)}>
            ${REASONS.map(([value, label]) => html`<option value=${value}>${label}</option>`)}
          </select>
        <//>
        <button class="btn btn-danger" onClick=${revoke} disabled=${action.busy}>确认吊销</button>
        <button class="btn" onClick=${() => setRevoking(false)}>取消</button>
      </div>` : html`<div class="danger-row">
        <div><strong>吊销证书</strong><p class="muted">通知 CA 吊销此证书，并关闭自动续期。此操作不可撤销。</p></div>
        <button class="btn btn-danger" onClick=${() => setRevoking(true)}>吊销</button>
      </div>`) : null}
      <div class="danger-row">
        <div><strong>删除证书</strong><p class="muted">从数据库删除证书、私钥及签发记录。不会吊销证书。</p></div>
        <button class="btn btn-danger" onClick=${remove} disabled=${action.busy || busyIssuing}>删除</button>
      </div>
    </div>
  <//>`;
}

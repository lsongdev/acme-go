import { useState } from 'preact/hooks';
import {
  html, Link, api, navigate, useResource, useAction, ErrorNotice, Loading, Status, Expiry,
  CopyButton, formatDate, stepLabel, unfinished, CHALLENGES,
} from '../lib.js';

// RFC 5280 revocation reasons accepted by ACME CAs.
const REASONS = [
  [0, '未指定'],
  [1, '私钥泄露'],
  [3, '信息变更'],
  [4, '已被替代'],
  [5, '停止运营'],
];

export default function Certificate({ id }) {
  const issuances = useResource(`/issuances?certificate_id=${id}`, items => items?.some(unfinished) ? 3000 : 0);
  const busyIssuing = issuances.data?.some(unfinished);
  const certificate = useResource(`/certificates/${id}`, c => c?.status === 'pending' || busyIssuing ? 3000 : 0);
  const accounts = useResource('/accounts');
  const action = useAction();
  const [revoking, setRevoking] = useState(false);
  const [reason, setReason] = useState(0);

  const c = certificate.data;
  if (certificate.loading && !c) return html`<div class="page"><${Loading} /></div>`;
  if (!c) return html`<div class="page"><${ErrorNotice} error=${certificate.error} /></div>`;

  const reload = () => { certificate.reload(); issuances.reload(); };
  const account = accounts.data?.find(a => a.id === c.account_id);
  const download = format => `api/certificates/${c.id}/download?format=${format}`;

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
    if (!confirm(`删除证书 ${c.domains[0]}？证书和私钥将从数据库中永久删除。`)) return;
    action.run(async () => {
      await api.del(`/certificates/${c.id}`);
      navigate('/certificates');
    });
  };

  return html`
    <div class="page">
      <div class="page-header">
        <div>
          <p class="breadcrumb"><${Link} to="/certificates">证书<//> /</p>
          <h1 class="mono">${c.domains[0]}</h1>
        </div>
        <div class="actions">
          ${c.status !== 'revoked' && html`
            <button class="button button-primary" onClick=${renew} disabled=${action.busy || busyIssuing}>
              ${c.status === 'active' ? '立即续期' : '重新签发'}
            </button>`}
          ${c.cert_pem && html`<a class="button" href=${download('zip')} download>下载</a>`}
        </div>
      </div>
      <${ErrorNotice} error=${action.error || certificate.error} />
      ${busyIssuing && html`
        <div class="notice notice-info">
          正在签发：<${Link} to=${`/issuances/${issuances.data.find(unfinished).id}`}>查看进度<//>
        </div>`}

      <section class="card">
        <dl class="details">
          <dt>状态</dt><dd><${Status} status=${c.status} /></dd>
          <dt>域名</dt><dd>${c.domains.map(d => html`<div class="mono">${d}</div>`)}</dd>
          <dt>到期时间</dt><dd><${Expiry} value=${c.not_after} /></dd>
          <dt>生效时间</dt><dd>${formatDate(c.not_before, true)}</dd>
          <dt>颁发者</dt><dd>${c.issuer || '—'}</dd>
          <dt>序列号</dt><dd class="mono break">${c.serial_number || '—'}</dd>
          <dt>创建时间</dt><dd>${formatDate(c.created_at, true)}</dd>
        </dl>
      </section>

      <section class="card">
        <h2>续期设置</h2>
        <div class="form form-inline">
          <label class="field">
            <span class="label">ACME 账户</span>
            <select value=${c.account_id} disabled=${action.busy}
              onChange=${e => update({ account_id: Number(e.currentTarget.value) })}>
              ${!account && html`<option value=${c.account_id}>账户 #${c.account_id}</option>`}
              ${(accounts.data || []).filter(a => a.status === 'valid' || a.id === c.account_id).map(a => html`
                <option value=${a.id}>${a.provider_name} · ${a.email || `账户 #${a.id}`}</option>`)}
            </select>
          </label>
          <label class="field">
            <span class="label">验证方式</span>
            <select value=${c.challenge_type} disabled=${action.busy}
              onChange=${e => update({ challenge_type: e.currentTarget.value })}>
              ${Object.entries(CHALLENGES).map(([value, label]) => html`<option value=${value}>${label}</option>`)}
            </select>
          </label>
          <label class="choice">
            <input type="checkbox" checked=${c.auto_renew} disabled=${action.busy || c.status === 'revoked'}
              onChange=${e => update({ auto_renew: e.currentTarget.checked })} />
            <span><strong>自动续期</strong></span>
          </label>
        </div>
      </section>

      ${c.cert_pem && html`
        <section class="card">
          <div class="card-header">
            <h2>证书链（PEM）</h2>
            <div class="actions">
              <${CopyButton} text=${c.cert_pem} />
              <a class="button button-small" href=${download('fullchain')} download>fullchain.pem</a>
              <a class="button button-small" href=${download('key')} download>privkey.pem</a>
            </div>
          </div>
          <pre class="pem">${c.cert_pem}</pre>
        </section>`}

      <section class="card">
        <h2>签发记录</h2>
        ${issuances.data?.length ? html`
          <div class="table-wrap">
            <table>
              <thead><tr><th>#</th><th>状态</th><th>步骤</th><th>开始时间</th><th>错误</th></tr></thead>
              <tbody>
                ${issuances.data.map(i => html`
                  <tr key=${i.id}>
                    <td><${Link} to=${`/issuances/${i.id}`}>#${i.id}<//></td>
                    <td><${Status} status=${i.status} /></td>
                    <td>${stepLabel(i.step)}</td>
                    <td>${formatDate(i.created_at, true)}</td>
                    <td class="error-cell" title=${i.error}>${i.error || ''}</td>
                  </tr>`)}
              </tbody>
            </table>
          </div>` : html`<p class="muted">暂无记录</p>`}
      </section>

      <section class="card card-danger">
        <h2>危险操作</h2>
        ${c.status === 'active' && (revoking ? html`
          <div class="form form-inline">
            <label class="field">
              <span class="label">吊销原因</span>
              <select value=${reason} onChange=${e => setReason(e.currentTarget.value)}>
                ${REASONS.map(([value, label]) => html`<option value=${value}>${label}</option>`)}
              </select>
            </label>
            <div class="actions">
              <button class="button button-danger" onClick=${revoke} disabled=${action.busy}>确认吊销</button>
              <button class="button" onClick=${() => setRevoking(false)}>取消</button>
            </div>
          </div>` : html`
          <div class="danger-row">
            <div><strong>吊销证书</strong><p class="muted">通知 CA 吊销此证书，并关闭自动续期。此操作不可撤销。</p></div>
            <button class="button button-danger" onClick=${() => setRevoking(true)}>吊销</button>
          </div>`)}
        <div class="danger-row">
          <div><strong>删除证书</strong><p class="muted">从数据库删除证书、私钥及签发记录。不会吊销证书。</p></div>
          <button class="button button-danger" onClick=${remove} disabled=${action.busy || busyIssuing}>删除</button>
        </div>
      </section>
    </div>`;
}

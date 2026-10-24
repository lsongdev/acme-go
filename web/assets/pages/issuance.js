import {
  html, Link, api, useResource, useAction, ErrorNotice, Loading, Status, CopyButton,
  formatDate, STEPS, unfinished, CHALLENGES,
} from '../lib.js';

// Steps that only apply to one challenge type, or only to automatic DNS.
const HIDDEN_STEPS = {
  'http-01': ['publishing-dns', 'awaiting-dns', 'checking-dns'],
  'dns-01': [],
};

function Steps({ issuance }) {
  // DNS records are either published by a configured DNS provider or added
  // by the operator; show one of the two steps, the manual one by default.
  const skipped = issuance.step === 'publishing-dns' ? 'awaiting-dns' : 'publishing-dns';
  const steps = STEPS.filter(([key]) => key !== skipped && !HIDDEN_STEPS[issuance.challenge_type]?.includes(key));
  const current = steps.findIndex(([key]) => key === issuance.step);
  return html`
    <ol class="steps">
      ${steps.map(([key, label], index) => {
        let state = index < current ? 'done' : index === current ? 'current' : 'todo';
        if (issuance.status === 'succeeded') state = 'done';
        if (state === 'current' && issuance.status === 'failed') state = 'failed';
        return html`<li key=${key} class=${`step step-${state}`}>${label}</li>`;
      })}
    </ol>`;
}

function DNSRecords({ issuance, onContinue, busy }) {
  const waiting = issuance.status === 'waiting';
  return html`
    <section class="card">
      <h2>DNS TXT 记录</h2>
      ${waiting && html`
        <p>请在域名的 DNS 服务商处添加以下 TXT 记录，生效后点击「继续验证」。同名记录有多个值时，需全部添加。</p>`}
      <div class="table-wrap">
        <table>
          <thead><tr><th>域名</th><th>记录名</th><th>记录值</th></tr></thead>
          <tbody>
            ${issuance.dns_records.map(r => html`
              <tr key=${r.domain}>
                <td class="mono">${r.domain}</td>
                <td><span class="mono">${r.name}</span> <${CopyButton} text=${r.name} /></td>
                <td><span class="mono break">${r.value}</span> <${CopyButton} text=${r.value} /></td>
              </tr>`)}
          </tbody>
        </table>
      </div>
      ${waiting && html`
        <div class="actions">
          <button class="button button-primary" disabled=${busy} onClick=${() => onContinue(false)}>继续验证</button>
          <button class="button" disabled=${busy} onClick=${() => onContinue(true)}
            title="不通过公共 DNS 检查记录，直接请求 CA 验证">跳过检查</button>
        </div>
        <p class="hint">「继续验证」会先通过公共 DNS（1.1.1.1、8.8.8.8）确认记录可见，避免 CA 验证失败后需要重新下单。</p>`}
    </section>`;
}

export default function Issuance({ id }) {
  const { data: issuance, error, loading, reload } = useResource(`/issuances/${id}`, i => unfinished(i) ? 2000 : 0);
  const action = useAction();
  if (loading && !issuance) return html`<div class="page"><${Loading} /></div>`;
  if (!issuance) return html`<div class="page"><${ErrorNotice} error=${error} /></div>`;

  const resume = force => action.run(async () => {
    await api.post(`/issuances/${id}/continue`, { force });
    reload();
  });
  const cancel = () => {
    if (!confirm('取消此次签发？')) return;
    action.run(async () => {
      await api.post(`/issuances/${id}/cancel`);
      reload();
    });
  };

  return html`
    <div class="page">
      <div class="page-header">
        <div>
          <p class="breadcrumb"><${Link} to="/issuances">签发记录<//> /</p>
          <h1>签发 #${issuance.id}</h1>
        </div>
        <div class="actions">
          <${Link} to=${`/certificates/${issuance.certificate_id}`} class="button">查看证书<//>
          ${unfinished(issuance) && html`<button class="button button-danger" onClick=${cancel} disabled=${action.busy}>取消签发</button>`}
        </div>
      </div>
      <${ErrorNotice} error=${action.error} />

      <section class="card">
        <dl class="details">
          <dt>状态</dt><dd><${Status} status=${issuance.status} /></dd>
          <dt>域名</dt><dd>${issuance.domains.map(d => html`<div class="mono">${d}</div>`)}</dd>
          <dt>验证方式</dt><dd>${CHALLENGES[issuance.challenge_type] || issuance.challenge_type}</dd>
          <dt>开始时间</dt><dd>${formatDate(issuance.created_at, true)}</dd>
          <dt>更新时间</dt><dd>${formatDate(issuance.updated_at, true)}</dd>
        </dl>
        <${Steps} issuance=${issuance} />
        ${issuance.error && html`
          <div class=${`notice ${issuance.status === 'waiting' ? 'notice-warn' : 'notice-error'}`}>${issuance.error}</div>`}
        ${issuance.status === 'succeeded' && html`
          <div class="notice notice-ok">证书已签发。<${Link} to=${`/certificates/${issuance.certificate_id}`}>查看并下载<//></div>`}
      </section>

      ${issuance.dns_records?.length > 0 && html`
        <${DNSRecords} issuance=${issuance} onContinue=${resume} busy=${action.busy} />`}
    </div>`;
}

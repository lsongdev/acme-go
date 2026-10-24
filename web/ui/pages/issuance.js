import { html } from '../lib/html.js';
import { useAction } from '../lib/hooks.js';
import { Page, Notice, Loading, KV, CopyButton, confirmDanger } from '../lib/ui.js';
import { usePanel } from '../lib/panel.js';
import { useApi, Link, Status, formatDate, STEPS, unfinished, CHALLENGES } from '../acme.js';

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
  return html`<ol class="steps">
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
  return html`<div class="card"><h2>DNS TXT 记录</h2>
    ${waiting ? html`<p>请在域名的 DNS 服务商处添加以下 TXT 记录，生效后点击「继续验证」。同名记录有多个值时，需全部添加。</p>` : null}
    <div class="table-wrap"><table class="table">
      <thead><tr><th>域名</th><th>记录名</th><th>记录值</th></tr></thead>
      <tbody>${issuance.dns_records.map(r => html`<tr key=${r.domain}>
        <td class="mono">${r.domain}</td>
        <td><span class="mono">${r.name}</span> <${CopyButton} text=${r.name} /></td>
        <td><span class="mono" style="overflow-wrap:anywhere">${r.value}</span> <${CopyButton} text=${r.value} /></td>
      </tr>`)}</tbody>
    </table></div>
    ${waiting ? html`
      <div class="form-actions" style="margin-top:12px">
        <button class="btn btn-primary" disabled=${busy} onClick=${() => onContinue(false)}>继续验证</button>
        <button class="btn" disabled=${busy} onClick=${() => onContinue(true)}
          title="不通过公共 DNS 检查记录，直接请求 CA 验证">跳过检查</button>
      </div>
      <p class="field-hint">「继续验证」会先通过公共 DNS（1.1.1.1、8.8.8.8）确认记录可见，避免 CA 验证失败后需要重新下单。</p>` : null}
  </div>`;
}

export default function Issuance({ params: { id } }) {
  const { api, href } = usePanel();
  const { data: issuance, error, reload } = useApi(`/issuances/${id}`, i => unfinished(i) ? 2000 : 0);
  const action = useAction();
  const back = html`<a class="btn" href=${href('/issuances')}>返回</a>`;
  if (!issuance) {
    return html`<${Page} title=${`签发 #${id}`} actions=${back}>
      ${error ? html`<${Notice} error=${error} />` : html`<${Loading} />`}
    <//>`;
  }

  const resume = force => action.run(async () => {
    await api.post(`/issuances/${id}/continue`, { force });
    reload();
  });
  const cancel = () => {
    if (!confirmDanger('取消此次签发？')) return;
    action.run(async () => {
      await api.post(`/issuances/${id}/cancel`);
      reload();
    });
  };

  return html`<${Page} title=${`签发 #${issuance.id}`} actions=${html`
      <a class="btn" href=${href(`/certificates/${issuance.certificate_id}`)}>查看证书</a>
      ${unfinished(issuance) ? html`<button class="btn btn-danger" onClick=${cancel} disabled=${action.busy}>取消签发</button>` : null}
      ${back}`}>
    <${Notice} error=${action.error} />
    <div class="card">
      <${KV} items=${[
        ['状态', html`<${Status} status=${issuance.status} />`],
        ['域名', issuance.domains.map(d => html`<div class="mono">${d}</div>`)],
        ['验证方式', CHALLENGES[issuance.challenge_type] || issuance.challenge_type],
        ['开始时间', formatDate(issuance.created_at, true)],
        ['更新时间', formatDate(issuance.updated_at, true)],
      ]} />
      <${Steps} issuance=${issuance} />
      ${issuance.error ? html`<div style="margin-top:14px"><${Notice} tone=${issuance.status === 'waiting' ? 'warn' : 'error'}>${issuance.error}<//></div>` : null}
      ${issuance.status === 'succeeded' ? html`<div style="margin-top:14px"><${Notice} tone="info">证书已签发。<${Link} to=${`/certificates/${issuance.certificate_id}`}>查看并下载<//><//></div>` : null}
    </div>
    ${issuance.dns_records?.length > 0 ? html`<${DNSRecords} issuance=${issuance} onContinue=${resume} busy=${action.busy} />` : null}
  <//>`;
}

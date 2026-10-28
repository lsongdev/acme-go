import { html } from '../lib/html.js';
import { useAction } from '../lib/hooks.js';
import { Page, Notice, Loading, KV, CopyButton, confirmDanger, Table } from '../lib/ui.js';
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
  return html`<div class="card"><h2>DNS TXT records</h2>
    ${waiting ? html`<p>Add these TXT records at the DNS provider of the domains, then click Continue once they are live. Where a name has several values, add all of them.</p>` : null}
    <${Table}>
      <thead><tr><th>Domain</th><th>Name</th><th>Value</th></tr></thead>
      <tbody>${issuance.dns_records.map(r => html`<tr key=${r.domain}>
        <td class="mono">${r.domain}</td>
        <td><span class="mono">${r.name}</span> <${CopyButton} text=${r.name} /></td>
        <td><span class="mono" style="overflow-wrap:anywhere">${r.value}</span> <${CopyButton} text=${r.value} /></td>
      </tr>`)}</tbody>
    <//>
    ${waiting ? html`
      <div class="form-actions" style="margin-top:12px">
        <button class="button button-primary" disabled=${busy} onClick=${() => onContinue(false)}>Continue</button>
        <button class="button" disabled=${busy} onClick=${() => onContinue(true)}
          title="Ask the CA to validate without first checking the records through public DNS">Skip the check</button>
      </div>
      <p class="field-hint">Continue first checks through public DNS (1.1.1.1, 8.8.8.8) that the records are visible, so a failed CA validation does not force a new order.</p>` : null}
  </div>`;
}

export default function Issuance({ params: { id } }) {
  const { api, href } = usePanel();
  const { data: issuance, error, reload } = useApi(`/issuances/${id}`, i => unfinished(i) ? 2000 : 0);
  const action = useAction();
  const back = { href: href('/issuances'), label: 'Issuances' };
  if (!issuance) {
    return html`<${Page} title=${`Issuance #${id}`} back=${back}>
      ${error ? html`<${Notice} error=${error} />` : html`<${Loading} />`}
    <//>`;
  }

  const resume = force => action.run(async () => {
    await api.post(`/issuances/${id}/continue`, { force });
    reload();
  });
  const cancel = () => {
    if (!confirmDanger('Cancel this issuance?')) return;
    action.run(async () => {
      await api.post(`/issuances/${id}/cancel`);
      reload();
    });
  };

  return html`<${Page} title=${`Issuance #${issuance.id}`} back=${back} actions=${html`
      <a class="button" href=${href(`/certificates/${issuance.certificate_id}`)}>View certificate</a>
      ${unfinished(issuance) ? html`<button class="button button-danger" onClick=${cancel} disabled=${action.busy}>Cancel issuance</button>` : null}
      `}>
    <${Notice} error=${action.error} />
    <div class="card">
      <${KV} items=${[
        ['Status', html`<${Status} status=${issuance.status} />`],
        ['Domains', issuance.domains.map(d => html`<div class="mono">${d}</div>`)],
        ['Challenge', CHALLENGES[issuance.challenge_type] || issuance.challenge_type],
        ['Started', formatDate(issuance.created_at, true)],
        ['Updated', formatDate(issuance.updated_at, true)],
        ['Attempts', issuance.attempts || 0],
        ...(issuance.next_attempt_at ? [['Next retry', formatDate(issuance.next_attempt_at, true)]] : []),
        ...(issuance.order_url ? [['Order', html`<span class="mono" style="overflow-wrap:anywhere">${issuance.order_url}</span>`]] : []),
      ]} />
      <${Steps} issuance=${issuance} />
      ${issuance.error ? html`<div style="margin-top:14px"><${Notice} tone=${['waiting', 'retrying'].includes(issuance.status) ? 'warn' : 'error'}>${issuance.error}<//></div>` : null}
      ${issuance.status === 'succeeded' ? html`<div style="margin-top:14px"><${Notice} tone="info">The certificate was issued. <${Link} to=${`/certificates/${issuance.certificate_id}`}>View and download<//><//></div>` : null}
    </div>
    ${issuance.dns_records?.length > 0 ? html`<${DNSRecords} issuance=${issuance} onContinue=${resume} busy=${action.busy} />` : null}
  <//>`;
}

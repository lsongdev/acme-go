import { html, useState } from '../lib/html.js';
import { useAction } from '../lib/hooks.js';
import { Page, Notice, Loading, Field } from '../lib/ui.js';
import { usePanel } from '../lib/panel.js';
import { useApi, Link } from '../acme.js';

export default function NewCertificate() {
  const { api, href, navigate } = usePanel();
  const accounts = useApi('/accounts');
  const valid = (accounts.data || []).filter(a => a.status === 'valid');
  const [domains, setDomains] = useState('');
  const [accountID, setAccountID] = useState('');
  const [challenge, setChallenge] = useState('http-01');
  const [autoRenew, setAutoRenew] = useState(true);
  const { busy, error, run } = useAction();

  const list = domains.split(/[\s,]+/).filter(Boolean);
  const wildcard = list.some(d => d.startsWith('*.'));
  const challengeType = wildcard ? 'dns-01' : challenge;
  const selectedAccount = accountID || (valid[0] && String(valid[0].id)) || '';

  const submit = event => {
    event.preventDefault();
    run(async () => {
      const result = await api.post('/certificates', {
        domains: list,
        account_id: Number(selectedAccount),
        challenge_type: challengeType,
        auto_renew: autoRenew,
      });
      navigate(`/issuances/${result.issuance.id}`);
    });
  };

  const back = html`<a class="button" href=${href('/certificates')}>Back</a>`;
  if (accounts.data === undefined && !accounts.error) return html`<${Page} title="Request certificate" actions=${back}><${Loading} /><//>`;
  return html`<${Page} title="Request certificate" actions=${back}>
    <${Notice} error=${accounts.error || error} />
    ${valid.length === 0 ? html`<${Notice} tone="warn">No usable ACME account. <${Link} to="/accounts">Register one<//> first.<//>` : null}
    <form class="card form" onSubmit=${submit}>
      <${Field} label="Domains" required hint="One per line, or separated by commas or spaces. The first is the certificate's common name (CN).">
        <textarea class="input" rows="4" required placeholder=${'example.com\nwww.example.com'} value=${domains}
          onInput=${e => setDomains(e.currentTarget.value)}></textarea>
      <//>
      <${Field} label="ACME account" required>
        <select class="input" value=${selectedAccount} onChange=${e => setAccountID(e.currentTarget.value)} required>
          ${valid.map(a => html`<option value=${a.id}>${a.provider_name} · ${a.email || `Account #${a.id}`}</option>`)}
        </select>
      <//>
      <fieldset class="field">
        <legend class="field-label">Challenge</legend>
        <label class="choice">
          <input type="radio" name="challenge" value="http-01" checked=${challengeType === 'http-01'} disabled=${wildcard}
            onChange=${() => setChallenge('http-01')} />
          <span><strong>HTTP-01</strong><span class="field-hint">The CA fetches <code>http://domain/.well-known/acme-challenge/</code>: the domains must resolve to this server, with port 80 reachable (see the <code>-http01</code> flag).</span></span>
        </label>
        <label class="choice">
          <input type="radio" name="challenge" value="dns-01" checked=${challengeType === 'dns-01'}
            onChange=${() => setChallenge('dns-01')} />
          <span><strong>DNS-01</strong><span class="field-hint">Add TXT records in DNS; issuing shows which ones. Wildcard domains need this challenge.</span></span>
        </label>
      </fieldset>
      <label class="choice">
        <input type="checkbox" checked=${autoRenew} onChange=${e => setAutoRenew(e.currentTarget.checked)} />
        <span><strong>Auto-renew</strong><span class="field-hint">Issue again before expiry. With manual DNS-01, renewing still needs the records added by hand.</span></span>
      </label>
      <div class="form-actions">
        <button class="button button-primary" type="submit" disabled=${busy || valid.length === 0 || list.length === 0}>
          ${busy ? 'Submitting…' : 'Issue'}
        </button>
        <a class="button" href=${href('/certificates')}>Cancel</a>
      </div>
    </form>
  <//>`;
}

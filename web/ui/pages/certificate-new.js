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

  const back = html`<a class="btn" href=${href('/certificates')}>返回</a>`;
  if (accounts.data === undefined && !accounts.error) return html`<${Page} title="申请证书" actions=${back}><${Loading} /><//>`;
  return html`<${Page} title="申请证书" actions=${back}>
    <${Notice} error=${accounts.error || error} />
    ${valid.length === 0 ? html`<${Notice} tone="warn">没有可用的 ACME 账户，请先 <${Link} to="/accounts">注册账户<//>。<//>` : null}
    <form class="card form" onSubmit=${submit}>
      <${Field} label="域名" required hint="每行一个，或用逗号、空格分隔。第一个域名作为证书的通用名称（CN）。">
        <textarea class="input" rows="4" required placeholder=${'example.com\nwww.example.com'} value=${domains}
          onInput=${e => setDomains(e.currentTarget.value)}></textarea>
      <//>
      <${Field} label="ACME 账户" required>
        <select class="input" value=${selectedAccount} onChange=${e => setAccountID(e.currentTarget.value)} required>
          ${valid.map(a => html`<option value=${a.id}>${a.provider_name} · ${a.email || `账户 #${a.id}`}</option>`)}
        </select>
      <//>
      <fieldset class="field">
        <legend class="field-label">验证方式</legend>
        <label class="choice">
          <input type="radio" name="challenge" value="http-01" checked=${challengeType === 'http-01'} disabled=${wildcard}
            onChange=${() => setChallenge('http-01')} />
          <span><strong>HTTP-01</strong><span class="field-hint">CA 访问 <code>http://域名/.well-known/acme-challenge/</code>，域名需解析到本服务，且 80 端口可达（参见 <code>-http01</code> 参数）。</span></span>
        </label>
        <label class="choice">
          <input type="radio" name="challenge" value="dns-01" checked=${challengeType === 'dns-01'}
            onChange=${() => setChallenge('dns-01')} />
          <span><strong>DNS-01</strong><span class="field-hint">在 DNS 中添加 TXT 记录。签发过程中会显示需要添加的记录。通配符域名必须使用此方式。</span></span>
        </label>
      </fieldset>
      <label class="choice">
        <input type="checkbox" checked=${autoRenew} onChange=${e => setAutoRenew(e.currentTarget.checked)} />
        <span><strong>自动续期</strong><span class="field-hint">到期前自动重新签发。DNS-01 手动模式下续期仍需要人工添加记录。</span></span>
      </label>
      <div class="form-actions">
        <button class="btn btn-primary" type="submit" disabled=${busy || valid.length === 0 || list.length === 0}>
          ${busy ? '正在提交…' : '开始签发'}
        </button>
        <a class="btn" href=${href('/certificates')}>取消</a>
      </div>
    </form>
  <//>`;
}

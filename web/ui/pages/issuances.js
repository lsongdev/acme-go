import { html } from '../lib/html.js';
import { Page, Notice, Loading, Empty } from '../lib/ui.js';
import { useApi, Link, Status, Domains, formatDate, stepLabel, unfinished, CHALLENGES } from '../acme.js';

export default function Issuances() {
  const { data, error } = useApi('/issuances', items => items?.some(unfinished) ? 3000 : 0);
  return html`<${Page} title="签发记录">
    <${Notice} error=${error} />
    ${data === undefined ? (error ? null : html`<${Loading} />`) : data.length === 0 ? html`<${Empty}>暂无签发记录。<//>` : html`
      <div class="table-wrap"><table class="table">
        <thead><tr><th>#</th><th>域名</th><th>状态</th><th>步骤</th><th>验证方式</th><th>开始时间</th><th>错误</th></tr></thead>
        <tbody>${data.map(i => html`<tr key=${i.id}>
          <td><${Link} to=${`/issuances/${i.id}`}>#${i.id}<//></td>
          <td><${Link} to=${`/certificates/${i.certificate_id}`}><${Domains} domains=${i.domains} /><//></td>
          <td><${Status} status=${i.status} /></td>
          <td>${stepLabel(i.step)}</td>
          <td>${CHALLENGES[i.challenge_type] || i.challenge_type}</td>
          <td>${formatDate(i.created_at, true)}</td>
          <td class="error-cell" title=${i.error}>${i.error || ''}</td>
        </tr>`)}</tbody>
      </table></div>`}
  <//>`;
}

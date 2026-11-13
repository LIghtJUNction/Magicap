'use strict';

const $ = id => document.getElementById(id);
const pageInfo = {
  sessions: ['连接的切片', '看见', '流动', '让经过的请求，留下清晰的痕迹。'],
  rules: ['途中，另一个可能', '改变', '去向', '在请求抵达之前，写下你的决定。'],
  replay: ['一次新的尝试', '再次', '出发', '从一条请求开始，探索不同的回应。'],
  setup: ['连接的起点', '从此', '接入', '让设备、证书与客户端，走到同一条路上。'],
};
let secret = '';
let flows = [];
let selected = null;
let generation = 0;
let feedbackTimer;
let filterTimer;
const isService = ['127.0.0.1', 'localhost'].includes(location.hostname) && ['http:', 'https:'].includes(location.protocol);
const requestHeaders = () => ({ 'Content-Type': 'application/json', ...(secret ? { Authorization: `Bearer ${secret}` } : {}) });

async function api(path, options = {}) {
  let response;
  try {
    response = await fetch(path, { ...options, headers: requestHeaders() });
  } catch {
    throw new Error('尚未连接本机服务。请先开启抓包，再重新连接。');
  }
  if (!response.ok) {
    const messages = {
      400: '内容格式不正确，请检查地址、规则或请求头。',
      401: '密钥未通过验证，请在“连接”中填写已配置的密钥。',
      403: '当前访问来源未获允许，请使用本机观察台地址。',
      502: '请求未能抵达上游，请检查地址、网络与抓包状态。',
    };
    throw new Error(messages[response.status] || `服务返回异常（${response.status}），请稍后重试。`);
  }
  return response.json();
}

function report(error) {
  $('error').hidden = false;
  $('errorText').textContent = error instanceof SyntaxError ? '填写的内容不是有效的 JSON，请检查引号和逗号。' : error.message;
}
function run(action) {
  return async () => {
    $('error').hidden = true;
    try { await action(); } catch (error) { report(error); }
  };
}
function notify(message) {
  clearTimeout(feedbackTimer);
  $('feedback').textContent = message;
  $('feedback').hidden = false;
  feedbackTimer = setTimeout(() => { $('feedback').hidden = true; }, 2600);
}
function text(tag, value, className) {
  const element = document.createElement(tag);
  element.textContent = value;
  if (className) element.className = className;
  return element;
}
function decode(body) {
  try { return new TextDecoder().decode(Uint8Array.from(atob(body || ''), character => character.charCodeAt(0))); }
  catch { return '无法解码此正文。'; }
}
function prettyBody(body, headers) {
  const value = decode(body);
  const type = Object.entries(headers || {}).find(([key]) => key.toLowerCase() === 'content-type')?.[1]?.join(' ') || '';
  if (type.includes('json')) {
    try { return JSON.stringify(JSON.parse(value), null, 2); } catch { /* Preserve incomplete or truncated response text. */ }
  }
  return value || '没有正文。';
}
function describeURL(value) {
  try { const url = new URL(value); return { host: url.host, path: url.pathname + url.search }; }
  catch { return { host: value, path: '' }; }
}
function detailPlaceholder() {
  const box = text('div', '', 'detail-empty');
  box.append(text('span', '细节在此展开', 'small-index'), text('span', '↙', 'detail-stroke'));
  const paragraph = text('p', '选择一条请求，');
  paragraph.append(document.createElement('br'), document.createTextNode('沿着连接往里看。'));
  box.append(paragraph, text('small', '请求头、正文与响应，逐一呈现。'));
  $('detail').replaceChildren(box);
}
function show(flow) {
  selected = flow.id;
  const url = describeURL(flow.url);
  const detail = $('detail');
  const top = text('div', '', 'detail-top');
  top.append(text('span', `第 ${String(flow.id).padStart(3, '0')} 条`, 'detail-number'));
  const replay = text('button', '载入重放 ↗', 'quiet');
  replay.onclick = () => {
    if (!$('method').querySelector(`option[value="${CSS.escape(flow.method)}"]`)) {
      const option = text('option', flow.method); option.value = flow.method; $('method').append(option);
    }
    $('method').value = flow.method;
    $('url').value = flow.url;
    $('body').value = decode(flow.request_body);
    $('headers').value = JSON.stringify(Object.fromEntries(Object.entries(flow.request_headers || {}).map(([key, values]) => [key, values.join(', ')])), null, 2);
    switchTab('replay');
    notify(flow.request_truncated ? '已载入；原始请求正文曾被截断。' : '请求已载入，可以调整后发送。');
  };
  top.append(replay);
  const timing = new Date(flow.started).toLocaleTimeString('zh-CN', { hour12: false });
  detail.replaceChildren(top, text('h2', url.host), text('p', flow.url, 'detail-url'), text('span', `${flow.method}  /  ${flow.status || '失败'}  /  ${flow.duration_ms} ms  /  ${timing}`, 'detail-meta'));
  for (const [label, value] of [
    ['请求头', JSON.stringify(flow.request_headers || {}, null, 2)],
    [`请求正文${flow.request_truncated ? ' · 已截断' : ''}`, prettyBody(flow.request_body, flow.request_headers)],
    ['响应头', JSON.stringify(flow.response_headers || {}, null, 2)],
    [`响应正文${flow.response_truncated ? ' · 已截断' : ''}`, prettyBody(flow.response_body, flow.response_headers)],
  ]) {
    const section = document.createElement('details');
    section.open = label.startsWith('响应正文');
    section.append(text('summary', label), text('pre', value));
    detail.append(section);
  }
  if (flow.error) detail.append(text('pre', flow.error, 'bad'));
  draw();
}
function draw() {
  $('count').textContent = flows.length;
  $('sessionTotal').textContent = String(flows.length).padStart(3, '0');
  const container = $('flows');
  const scroll = container.scrollTop;
  container.replaceChildren();
  if (!flows.length) {
    const empty = text('div', '', 'empty');
    const symbol = text('span', '', 'empty-symbol');
    symbol.setAttribute('aria-hidden', 'true');
    symbol.append(document.createElement('i'));
    const filtered = $('filter').value.trim();
    empty.append(symbol, text('h2', filtered ? '没有找到这条连接。' : '等待一条连接。'));
    empty.append(text('p', filtered ? '换一段地址试试，或清除筛选查看所有记录。' : '开启抓包，将客户端代理指向本机。第一条请求经过时，会出现在这里。'));
    container.append(empty);
    return;
  }
  for (const flow of [...flows].reverse()) {
    const url = describeURL(flow.url);
    const button = text('button', '', `flow${selected === flow.id ? ' selected' : ''}`);
    button.setAttribute('aria-pressed', String(selected === flow.id));
    const address = text('span', '', 'flow-address');
    address.append(text('span', url.host, 'flow-host'));
    const path = text('span', '', 'flow-path');
    path.append(text('span', flow.method, 'flow-method'), document.createTextNode(url.path));
    address.append(path);
    const stats = text('span', '', 'flow-stats');
    stats.append(text('span', flow.status || '失败', flow.status >= 400 || !flow.status ? 'bad' : ''), text('span', `${flow.duration_ms} ms`, 'flow-time'));
    button.append(address, stats);
    button.title = flow.url;
    button.onclick = () => show(flow);
    container.append(button);
  }
  container.scrollTop = scroll;
}
async function refresh() {
  const current = ++generation;
  try {
    const [health, entries] = await Promise.all([
      api('/health'), api('/api/sessions?q=' + encodeURIComponent($('filter').value)),
    ]);
    if (current !== generation) return;
    $('status').textContent = health.capture ? '正在观测' : health.mcp ? '自动化接口已开启' : '服务已连接';
    $('status').dataset.running = String(health.capture || health.mcp);
    $('proxyPort').textContent = health.proxy_port;
    $('setupProxy').textContent = `127.0.0.1:${health.proxy_port}`;
    $('setupWeb').textContent = location.origin;
    $('setupMcp').textContent = health.mcp_port ? `http://127.0.0.1:${health.mcp_port}/mcp` : '通过命令行查询当前端口';
    $('localEntry').href = location.origin;
    $('footerStatus').textContent = health.capture ? '正在收集经过代理的请求' : '抓包未开启';
    flows = entries;
    if (selected !== null && !flows.some(flow => flow.id === selected)) { selected = null; detailPlaceholder(); }
    draw();
  } catch (error) {
    if (current !== generation) return;
    $('status').textContent = '尚未连接';
    $('status').dataset.running = 'false';
    $('footerStatus').textContent = '请在连接页检查服务状态';
    throw error;
  }
}
function switchTab(id) {
  if (!pageInfo[id]) id = 'sessions';
  for (const section of document.querySelectorAll('.tab')) section.hidden = section.id !== id;
  for (const button of document.querySelectorAll('[data-tab]')) {
    const active = button.dataset.tab === id;
    button.classList.toggle('active', active);
    if (active) button.setAttribute('aria-current', 'page'); else button.removeAttribute('aria-current');
  }
  const [label, first, second, caption] = pageInfo[id];
  $('sectionLabel').textContent = label;
  const indented = text('span', second, 'title-indent');
  indented.append(text('span', '。', 'title-period'));
  $('pageTitle').replaceChildren(document.createTextNode(first), indented);
  $('pageIntro').textContent = caption;
  document.title = `Magicap · ${{ sessions: '会话', rules: '规则', replay: '重放', setup: '连接' }[id]}`;
  if (location.hash !== `#${id}`) history.replaceState(null, '', `#${id}`);
  if (id === 'rules' && isService && !$('rulesEditor').dataset.loaded) run(loadRules)();
}
async function loadRules() {
  $('rulesEditor').value = JSON.stringify(await api('/api/rules'), null, 2);
  $('rulesEditor').dataset.loaded = 'true';
}
async function download(path, name) {
  let response;
  try { response = await fetch(path, { headers: requestHeaders() }); }
  catch { throw new Error('未连接本机服务，暂时无法下载。'); }
  if (!response.ok) throw new Error(response.status === 401 ? '请先在连接页应用密钥。' : '下载失败，请检查服务状态。');
  const url = URL.createObjectURL(await response.blob());
  const anchor = document.createElement('a'); anchor.href = url; anchor.download = name; anchor.click();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}

for (const button of document.querySelectorAll('[data-tab]')) button.onclick = () => switchTab(button.dataset.tab);
$('dismissError').onclick = () => { $('error').hidden = true; };
$('refresh').onclick = run(async () => { await refresh(); notify('记录已更新。'); });
$('filter').oninput = () => { clearTimeout(filterTimer); filterTimer = setTimeout(run(refresh), 180); };
$('clear').onclick = run(async () => {
  await api('/api/sessions', { method: 'DELETE' }); selected = null; detailPlaceholder(); await refresh(); notify('会话记录已清空。');
});
$('har').onclick = run(async () => { await download('/api/har', 'magicap.har'); notify('记录已导出。'); });
$('ca').onclick = run(() => download('/api/ca', 'magicap-ca.pem'));
$('loadRules').onclick = run(async () => { await loadRules(); notify('已读取设备上的规则。'); });
$('saveRules').onclick = run(async () => {
  const rules = JSON.parse($('rulesEditor').value);
  if (!Array.isArray(rules)) throw new Error('规则集应是一组规则，请使用方括号包围。');
  const button = $('saveRules'); button.disabled = true;
  try { await api('/api/rules', { method: 'PUT', body: JSON.stringify(rules) }); notify('规则已保存，立即生效。'); }
  finally { button.disabled = false; }
});
$('exampleRule').onclick = () => {
  $('rulesEditor').value = JSON.stringify([
    { match: 'example.com/mock', mock_status: 200, mock_body: '{"message":"这是一条模拟响应"}', response_headers: { 'Content-Type': 'application/json' } },
    { match: 'example.com/blocked', block: true },
  ], null, 2);
  notify('示例已填入，保存后才会生效。');
};
$('send').onclick = run(async () => {
  const address = $('url').value.trim();
  let parsed;
  try { parsed = new URL(address); } catch { throw new Error('请填写完整地址，以 http:// 或 https:// 开头。'); }
  if (!['http:', 'https:'].includes(parsed.protocol)) throw new Error('仅支持 HTTP 和 HTTPS 请求。');
  const headers = JSON.parse($('headers').value);
  if (!headers || typeof headers !== 'object' || Array.isArray(headers) || Object.values(headers).some(value => typeof value !== 'string')) throw new Error('请求头需要填写为名称与文字值组成的 JSON 对象。');
  $('send').disabled = true; $('replayStatus').textContent = '正在等待回应';
  try {
    const started = performance.now();
    const output = await api('/api/replay', { method: 'POST', body: JSON.stringify({ method: $('method').value, url: address, headers, body: $('body').value }) });
    $('replayResult').textContent = JSON.stringify({ 状态码: output.status, 响应头: output.headers, 正文: decode(output.body), 已截断: output.truncated }, null, 2);
    $('replayStatus').textContent = `${output.status} · ${Math.round(performance.now() - started)} ms`;
    if (isService) await refresh();
  } catch (error) { $('replayStatus').textContent = '请求未完成'; throw error; }
  finally { $('send').disabled = false; }
});
$('setSecret').onclick = run(async () => { secret = $('secret').value; await refresh(); notify('密钥已应用到当前页面。'); });
window.addEventListener('hashchange', () => switchTab(location.hash.slice(1)));
draw();
switchTab(isService ? location.hash.slice(1) || 'sessions' : 'setup');
if (isService) {
  run(refresh)();
  setInterval(() => { if ($('live').checked && document.visibilityState === 'visible') run(refresh)(); }, 4000);
} else {
  $('status').textContent = '请打开本机观察台';
  $('footerStatus').textContent = '此页面提供连接指南';
}

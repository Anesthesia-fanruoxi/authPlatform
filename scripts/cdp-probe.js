// CDP 探针 v3：验证表格铺满 + keep-alive 切换后无溢出。用法: node scripts/cdp-probe.js
const CDP_HTTP = 'http://127.0.0.1:9222';

async function getWsUrl() {
  const resp = await fetch(`${CDP_HTTP}/json/new?about:blank`, { method: 'PUT' });
  const tab = await resp.json();
  return tab.webSocketDebuggerUrl;
}
function connect(wsUrl) {
  return new Promise((resolve, reject) => {
    const ws = new WebSocket(wsUrl);
    let id = 0; const pending = new Map();
    ws.onopen = () => resolve({
      send(method, params = {}) {
        return new Promise((res, rej) => {
          const msgId = ++id; pending.set(msgId, { res, rej });
          ws.send(JSON.stringify({ id: msgId, method, params }));
        });
      },
      close() { ws.close(); },
    });
    ws.onmessage = ev => {
      const msg = JSON.parse(ev.data);
      if (msg.id && pending.has(msg.id)) {
        const { res, rej } = pending.get(msg.id); pending.delete(msg.id);
        msg.error ? rej(new Error(JSON.stringify(msg.error))) : res(msg.result);
      }
    };
    ws.onerror = reject;
  });
}

async function evalv(cdp, expression, awaitPromise = false) {
  const out = await cdp.send('Runtime.evaluate', { expression, returnByValue: true, awaitPromise });
  return out.result ? out.result.value : undefined;
}

// 等待登录页或布局出现（页面 JS 已执行）
async function waitFor(cdp, expr, timeoutMs = 20000) {
  const start = Date.now();
  while (Date.now() - start < timeoutMs) {
    if (await evalv(cdp, expr)) return true;
    await new Promise(r => setTimeout(r, 400));
  }
  return false;
}

async function login(cdp) {
  // 页面加载完（出现 #app 内容）后执行登录
  await waitFor(cdp, `!!document.querySelector('.login-wrap') || !!document.querySelector('.layout')`);
  await evalv(cdp, `(async () => {
    const r = await fetch('/api/admin/login', { method: 'POST', headers: {'Content-Type':'application/json'}, body: JSON.stringify({username:'admin', password:'admin123'}) });
    const d = await r.json();
    localStorage.setItem('token', d.data.token);
    location.reload();
    return true;
  })()`, true);
  await waitFor(cdp, `!!document.querySelector('.layout')`);
}

const measureExpr = `(() => {
  const q = s => document.querySelector(s);
  const main = q('.main'), card = q('.page-card'), tbl = q('.el-table'), box = q('.table-box');
  const r = el => el ? el.getBoundingClientRect() : null;
  const n = v => Math.round(v || 0);
  const mr = r(main), cr = r(card), br = r(box), tr = r(tbl);
  const out = [];
  out.push('innerHeight=' + window.innerHeight);
  if (mr) out.push('main: top=' + n(mr.top) + ' bottom=' + n(mr.bottom) + ' scrollH=' + main.scrollHeight);
  if (cr) out.push('card: top=' + n(cr.top) + ' bottom=' + n(cr.bottom) + ' overflow=' + (cr.bottom > window.innerHeight));
  if (br) out.push('tableBox: h=' + n(br.height));
  if (tr) out.push('table: h=' + n(tr.height) + ' bottom=' + n(tr.bottom));
  if (main) out.push('mainScrollable=' + (main.scrollHeight > main.clientHeight));
  out.push('bodyScrollable=' + (document.body.scrollHeight > window.innerHeight));
  out.push('page=' + (location.hash || '#/users'));
  return out.join('\\n');
})()`;

async function check(cdp) {
  return await evalv(cdp, measureExpr) || 'EVAL ERROR';
}

async function main() {
  const cdp = await connect(await getWsUrl());
  await cdp.send('Page.enable'); await cdp.send('Runtime.enable');

  for (const [w, h, tag] of [[1400, 900, '大视口'], [1400, 620, '小视口'], [1100, 700, '窄视口']]) {
    await cdp.send('Emulation.setDeviceMetricsOverride', { width: w, height: h, deviceScaleFactor: 1, mobile: false });
    await cdp.send('Page.navigate', { url: 'http://127.0.0.1:8080/#/users' });
    await new Promise(r => setTimeout(r, 1500));
    await login(cdp);
    console.log(`\n===== ${tag} ${w}x${h} =====`);
    console.log(await check(cdp));
    for (const hash of ['#/platforms', '#/grants', '#/logs', '#/users']) {
      await evalv(cdp, `location.hash = '${hash}'; true`);
      await new Promise(r => setTimeout(r, 1500));
      const lines = (await check(cdp)).split('\\n').slice(0, 5).join(' | ');
      console.log('  switch ' + hash + ' => ' + lines);
    }
  }
  cdp.close();
  process.exit(0);
}
main().catch(e => { console.error('ERR', e.message); process.exit(1); });

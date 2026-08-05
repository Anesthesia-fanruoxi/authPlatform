// 黑名单演示：制造登录记录与黑名单条目，供 UI 查看。用法: node scripts/demo-login.js
const crypto = require('crypto');
const BASE = 'http://127.0.0.1:8080';
const PLATFORMS = {
  'ops-platform': '7020698c56ab7d58a9b0b8929be3bff2b19c0f25601f3f6bb215cfbd083ba8d4',
  'cmdb-platform': 'ec0fbf412d7e511eeb1d5834ada8ac3e5b68222f3c43b577be3e30d3d829c5c3',
  'monitor-platform': 'b8c0b49f89bb90e67187db8c7f391abc1106585c8e2882b50ab7c1c4d631d819',
};

let adminToken = '';
async function admin(path, { method = 'GET', body } = {}) {
  const resp = await fetch(BASE + path, {
    method, headers: { 'Content-Type': 'application/json', Authorization: 'Bearer ' + adminToken },
    body: body ? JSON.stringify(body) : undefined,
  });
  return resp.json();
}
async function verify(pid, username, password) {
  const secret = PLATFORMS[pid];
  const body = JSON.stringify({ username, password, platform_id: pid });
  const ts = Math.floor(Date.now() / 1000);
  const bodyHash = crypto.createHash('sha256').update(body).digest('hex');
  const sig = crypto.createHmac('sha256', secret).update(`POST|/api/auth/verify|${ts}|${bodyHash}`).digest('hex');
  const resp = await fetch(BASE + '/api/auth/verify', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'X-Platform-Id': pid, 'X-Timestamp': String(ts), 'X-Sign': sig },
    body,
  });
  return resp.json();
}
const tok = d => d && d.token ? ' token=' + d.token.slice(0, 12) + '...' : '';

async function main() {
  const login = await admin('/api/admin/login', { method: 'POST', body: { username: 'admin', password: 'admin123' } });
  adminToken = login.data.token;
  console.log('== 1. 正常登录（成功，签发 token） ==');
  for (const [u, p, pid] of [['alice', 'Alice@123', 'ops-platform'], ['bob', 'Bob@1234', 'cmdb-platform'], ['eve', 'Eve@1234', 'monitor-platform']]) {
    const r = await verify(pid, u, p);
    console.log(`  ${u.padEnd(6)} -> ${pid.padEnd(15)} code=${r.code}${tok(r.data)}`);
  }

  console.log('\n== 2. 错误密码（失败日志，不触发锁定） ==');
  const r1 = await verify('ops-platform', 'alice', 'wrong-pass-1');
  console.log(`  alice 错密码           code=${r1.code} msg=${r1.msg}`);

  console.log('\n== 3. 连续 5 次失败（触发自动锁定 -> 黑名单 auto 记录） ==');
  for (let i = 0; i < 5; i++) {
    const r = await verify('ops-platform', 'frank', 'bad-pass-' + i);
    if (i < 4) continue;
    console.log(`  frank 第${i + 1}次失败       code=${r.code}`);
  }
  const r2 = await verify('ops-platform', 'frank', 'Frank@123');
  console.log(`  frank 正确密码（应被锁）  code=${r2.code} msg=${r2.msg}`);

  console.log('\n== 4. 手动拉黑 dave（黑名单 manual 记录） ==');
  const ban = await admin('/api/admin/bans', { method: 'POST', body: { username: 'dave', reason: '测试演示：违规操作' } });
  console.log(`  拉黑 dave: code=${ban.code}`);
  const r3 = await verify('ops-platform', 'dave', 'Dave@123');
  console.log(`  dave 登录（应被拒）      code=${r3.code} msg=${r3.msg}`);

  console.log('\n== 5. 当前黑名单列表 ==');
  const bans = (await admin('/api/admin/bans')).data.bans;
  for (const b of bans) {
    console.log(`  ${b.username.padEnd(8)} [${b.source}] ${b.reason} 到期=${b.expires_at || '永久'} 状态=${b.status}`);
  }
  console.log('\n黑名单页可看到上述记录；frank 为自动锁定、dave 为手动拉黑，均可在 UI 点「解除」。');
}
main().catch(e => { console.error('脚本异常:', e); process.exit(1); });

// 平台级登录方式测试。用法: node scripts/test-platform-login.js
const crypto = require('crypto');
const BASE = 'http://127.0.0.1:8080';
const PLATFORMS = {
  'ops-platform': '7020698c56ab7d58a9b0b8929be3bff2b19c0f25601f3f6bb215cfbd083ba8d4',
  'cmdb-platform': 'ec0fbf412d7e511eeb1d5834ada8ac3e5b68222f3c43b577be3e30d3d829c5c3',
  'monitor-platform': 'b8c0b49f89bb90e67187db8c7f391abc1106585c8e2882b50ab7c1c4d631d819',
};

let adminToken = '';
async function admin(path, { method = 'GET', body, raw = false } = {}) {
  const resp = await fetch(BASE + path, {
    method, headers: { 'Content-Type': 'application/json', Authorization: 'Bearer ' + adminToken },
    body: body ? JSON.stringify(body) : undefined,
  });
  const json = await resp.json();
  if (json.code !== 0 && !raw) throw Object.assign(new Error(json.msg), { code: json.code });
  return json;
}
async function verify(pid, body) {
  const secret = PLATFORMS[pid];
  const payload = JSON.stringify({ ...body, platform_id: pid });
  const ts = Math.floor(Date.now() / 1000);
  const bodyHash = crypto.createHash('sha256').update(payload).digest('hex');
  const sig = crypto.createHmac('sha256', secret).update(`POST|/api/auth/verify|${ts}|${bodyHash}`).digest('hex');
  const resp = await fetch(BASE + '/api/auth/verify', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'X-Platform-Id': pid, 'X-Timestamp': String(ts), 'X-Sign': sig },
    body: payload,
  });
  return resp.json();
}
async function verifyStep(pid, ticket, credential) {
  const secret = PLATFORMS[pid];
  const payload = JSON.stringify({ ticket, credential, platform_id: pid });
  const ts = Math.floor(Date.now() / 1000);
  const bodyHash = crypto.createHash('sha256').update(payload).digest('hex');
  const sig = crypto.createHmac('sha256', secret).update(`POST|/api/auth/verify-step|${ts}|${bodyHash}`).digest('hex');
  const resp = await fetch(BASE + '/api/auth/verify-step', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'X-Platform-Id': pid, 'X-Timestamp': String(ts), 'X-Sign': sig },
    body: payload,
  });
  return resp.json();
}

// RFC6238
const B32 = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567';
function b32decode(s) {
  s = s.toUpperCase().replace(/=+$/, '');
  let bits = '';
  for (const ch of s) { const v = B32.indexOf(ch); bits += v.toString(2).padStart(5, '0'); }
  const out = [];
  for (let i = 0; i + 8 <= bits.length; i += 8) out.push(parseInt(bits.slice(i, i + 8), 2));
  return Buffer.from(out);
}
function totp(secret, t = Date.now()) {
  const key = b32decode(secret);
  const buf = Buffer.alloc(8);
  buf.writeBigUInt64BE(BigInt(Math.floor(t / 1000 / 30)));
  const h = crypto.createHmac('sha1', key).update(buf).digest();
  const off = h[h.length - 1] & 0x0f;
  return String(((h[off] & 0x7f) << 24 | h[off + 1] << 16 | h[off + 2] << 8 | h[off + 3]) % 1000000).padStart(6, '0');
}

let pass = 0, fail = 0;
function T(name, actual, expect) {
  const ok = actual === expect;
  ok ? pass++ : fail++;
  console.log(`  [${ok ? 'PASS' : 'FAIL'}] ${name}: got=${actual} expect=${expect}`);
}

async function main() {
  const login = await admin('/api/admin/login', { method: 'POST', body: { username: 'admin', password: 'admin123' } });
  adminToken = login.data.token;

  // alice 的 TOTP secret（之前已绑定；若未绑定则重新生成启用）
  const users = (await admin('/api/admin/users')).data.users;
  const alice = users.find(u => u.username === 'alice');
  const g = (await admin('/api/admin/users/' + alice.id + '/totp/generate', { method: 'POST' })).data;
  const en = await admin('/api/admin/users/' + alice.id + '/totp/enable', { method: 'POST', body: { code: totp(g.secret) }, raw: true });
  if (en.code !== 0) { console.log('  (alice TOTP 已绑定，跳过重新启用)'); }

  console.log('\n== 场景1：ops 平台配置 [用户名+密码 → TOTP] ==');
  const upd1 = await admin('/api/admin/platforms/1', { method: 'PUT', body: { login_methods: ['username_password', 'totp'] } });
  T('  更新平台登录方式', upd1.code, 0);
  const s1 = await verify('ops-platform', { method: 'username_password', identifier: 'alice', credential: 'Alice@123' });
  T('  第一步通过返回 ticket', s1.code === 0 && !!s1.data.ticket && s1.data.next_method === 'totp', true);
  const s2 = await verifyStep('ops-platform', s1.data.ticket, totp(g.secret));
  T('  第二步 TOTP 通过发 token', s2.code === 0 && !!s2.data.token, true);
  // 单步尝试（旧格式）应被拒：第一步方式必须是 username_password，但多步骤需要走 ticket？单步 username+password 仍走第一步并返回 ticket——验证返回的不是 token
  const s1b = await verify('ops-platform', { username: 'alice', password: 'Alice@123' });
  T('  多步骤配置下旧格式只返回 ticket（不直接发 token）', s1b.code === 0 && !!s1b.data.ticket && !s1b.data.token, true);

  console.log('\n== 场景2：monitor 平台配置 [邮箱+密码]，标识严格绑定 ==');
  const upd2 = await admin('/api/admin/platforms/3', { method: 'PUT', body: { login_methods: ['email_password'] } });
  T('  更新 monitor 登录方式', upd2.code, 0);
  const m1 = await verify('monitor-platform', { method: 'email_password', identifier: 'eve@example.com', credential: 'Eve@1234' });
  T('  邮箱+密码登录成功', m1.code === 0 && !!m1.data.token, true);
  const m2 = await verify('monitor-platform', { method: 'email_password', identifier: 'eve', credential: 'Eve@1234' });
  T('  用用户名代替邮箱被拒（严格校验）', m2.code, 1003);
  const m3 = await verify('monitor-platform', { username: 'eve', password: 'Eve@1234' });
  T('  旧格式(用户名+密码)在邮箱方式下被拒', m3.code, 1007);

  console.log('\n== 场景3：cmdb 未配置（系统默认 username_password）==');
  const c1 = await verify('cmdb-platform', { username: 'bob', password: 'Bob@1234' });
  T('  默认方式登录成功', c1.code === 0 && !!c1.data.token, true);

  console.log('\n== 场景4：平台配置校验 ==');
  const bad = await admin('/api/admin/platforms/2', { method: 'PUT', body: { login_methods: ['totp'] }, raw: true });
  T('  仅 TOTP 被拒', bad.code, 1007);
  const bad2 = await admin('/api/admin/platforms/2', { method: 'PUT', body: { login_methods: ['foo_bar'] }, raw: true });
  T('  未知方式被拒', bad2.code, 1007);

  console.log('\n== 场景5：平台列表输出 login_methods ==');
  const list = (await admin('/api/admin/platforms')).data.platforms;
  const ops = list.find(p => p.platform_id === 'ops-platform');
  const cmdb = list.find(p => p.platform_id === 'cmdb-platform');
  T('  ops 显示自定义方式', ops.login_methods_custom && ops.login_methods.length === 2, true);
  T('  cmdb 显示系统默认', !cmdb.login_methods_custom, true);

  console.log('\n== 收尾：清空平台自定义，恢复默认 ==');
  await admin('/api/admin/platforms/1', { method: 'PUT', body: { login_methods: [] } });
  await admin('/api/admin/platforms/3', { method: 'PUT', body: { login_methods: [] } });

  console.log(`\n===== 结果: ${pass} PASS, ${fail} FAIL =====`);
  process.exit(fail ? 1 : 0);
}
main().catch(e => { console.error('脚本异常:', e); process.exit(1); });

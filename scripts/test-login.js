// authPlatform 新功能端到端测试：多步骤登录 / TOTP / 手机验证码 / 后台IP白名单 / 设置
// 用法: node scripts/test-login.js
const crypto = require('crypto');
const BASE = 'http://127.0.0.1:8080';

// ---------- 基础工具 ----------
const SECRETS = {
  'ops-platform': '7020698c56ab7d58a9b0b8929be3bff2b19c0f25601f3f6bb215cfbd083ba8d4',
  'cmdb-platform': 'ec0fbf412d7e511eeb1d5834ada8ac3e5b68222f3c43b577be3e30d3d829c5c3',
  'monitor-platform': 'b8c0b49f89bb90e67187db8c7f391abc1106585c8e2882b50ab7c1c4d631d819',
};

let adminToken = '';
async function call(path, { method = 'GET', body, auth = false, platform, raw = false } = {}) {
  const headers = { 'Content-Type': 'application/json' };
  if (auth) headers['Authorization'] = 'Bearer ' + adminToken;
  let url = BASE + path;
  let payload;
  if (platform) {
    method = 'POST'; // 平台侧接口均为 POST（签名按 POST 计算）
    payload = typeof body === 'string' ? body : JSON.stringify(body);
    const ts = Math.floor(Date.now() / 1000);
    const bodyHash = crypto.createHash('sha256').update(payload).digest('hex');
    const sig = crypto.createHmac('sha256', platform.secret).update(`POST|${path}|${ts}|${bodyHash}`).digest('hex');
    headers['X-Platform-Id'] = platform.id;
    headers['X-Timestamp'] = String(ts);
    headers['X-Sign'] = sig;
    url = BASE + path;
  } else if (body !== undefined) {
    payload = JSON.stringify(body);
  }
  const resp = await fetch(url, { method, headers, body: payload });
  const json = await resp.json();
  if (json.code !== 0 && !platform && !raw) {
    throw Object.assign(new Error(json.msg || '请求失败'), { code: json.code });
  }
  return json;
}

// ---------- TOTP（RFC6238 客户端实现，与 Go 端对齐） ----------
const B32 = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567';
function b32decode(s) {
  s = s.toUpperCase().replace(/=+$/, '');
  let bits = '';
  for (const ch of s) {
    const v = B32.indexOf(ch);
    if (v < 0) throw new Error('bad base32');
    bits += v.toString(2).padStart(5, '0');
  }
  const out = [];
  for (let i = 0; i + 8 <= bits.length; i += 8) out.push(parseInt(bits.slice(i, i + 8), 2));
  return Buffer.from(out);
}
function totp(secret, t = Date.now()) {
  const key = b32decode(secret);
  const counter = Math.floor(t / 1000 / 30);
  const buf = Buffer.alloc(8);
  buf.writeBigUInt64BE(BigInt(counter));
  const h = crypto.createHmac('sha1', key).update(buf).digest();
  const off = h[h.length - 1] & 0x0f;
  const code = ((h[off] & 0x7f) << 24 | h[off + 1] << 16 | h[off + 2] << 8 | h[off + 3]) % 1000000;
  return String(code).padStart(6, '0');
}

// ---------- 测试框架 ----------
let pass = 0, fail = 0;
function T(name, actual, expect) {
  const ok = actual === expect;
  ok ? pass++ : fail++;
  console.log(`  [${ok ? 'PASS' : 'FAIL'}] ${name}: got=${actual} expect=${expect}`);
}
async function loginAdmin() {
  const r = await call('/api/admin/login', { method: 'POST', body: { username: 'admin', password: 'admin123' } });
  adminToken = r.data.token;
  console.log('  admin token ok');
}
async function setMethods(methods) {
  const r = await call('/api/admin/settings/login_methods', { method: 'PUT', body: { methods }, auth: true });
  if (r.code !== 0) throw new Error('set methods fail: ' + r.msg);
}

const ops = { id: 'ops-platform', secret: SECRETS['ops-platform'] };

async function main() {
  console.log('== 准备：admin 登录 + 用户联系方式 ==');
  await loginAdmin();

  // alice/bob 补手机号邮箱（幂等）
  const list = (await call('/api/admin/users', { auth: true })).data.users;
  const alice = list.find(u => u.username === 'alice');
  const bob = list.find(u => u.username === 'bob');
  const eve = list.find(u => u.username === 'eve');
  await call('/api/admin/users/' + alice.id, { method: 'PUT', body: { phone: '13800138000', email: 'alice@example.com' }, auth: true });
  await call('/api/admin/users/' + bob.id, { method: 'PUT', body: { phone: '13800138001', email: 'bob@example.com' }, auth: true });
  await call('/api/admin/users/' + eve.id, { method: 'PUT', body: { email: 'eve@example.com' }, auth: true });
  console.log('  alice/bob/eve 联系方式已设置');

  console.log('\n== 场景1：默认单步 用户名+密码 ==');
  const r1 = await call('/api/auth/verify', { body: { username: 'alice', password: 'Alice@123', platform_id: 'ops-platform' }, platform: ops });
  T('alice 单步登录成功 code=0', r1.code, 0);
  T('  返回 token', r1.data && !!r1.data.token, true);

  console.log('\n== 场景2：多步骤 [用户名+密码 → TOTP] ==');
  await setMethods(['username_password', 'totp']);
  // 2.1 先给 alice 绑定 TOTP
  const g = (await call('/api/admin/users/' + alice.id + '/totp/generate', { method: 'POST', auth: true })).data;
  const codeNow = totp(g.secret);
  const en = await call('/api/admin/users/' + alice.id + '/totp/enable', { method: 'POST', body: { code: codeNow }, auth: true });
  T('  TOTP 启用成功 code=0', en.code, 0);
  const wrongEn = await call('/api/admin/users/' + alice.id + '/totp/enable', { method: 'POST', body: { code: '000000' }, auth: true, raw: true });
  T('  错误 TOTP 码启用被拒', wrongEn.code, 1007);
  // 2.2 两步登录
  const s1 = await call('/api/auth/verify', { body: { method: 'username_password', identifier: 'alice', credential: 'Alice@123', platform_id: 'ops-platform' }, platform: ops });
  T('  第一步通过，返回 ticket', s1.code === 0 && !!s1.data.ticket && s1.data.next_method === 'totp', true);
  const s2bad = await call('/api/auth/verify-step', { body: { ticket: s1.data.ticket, credential: '123456', platform_id: 'ops-platform' }, platform: ops });
  T('  错误 TOTP 第二步被拒(1003，ticket保留)', s2bad.code, 1003);
  const s2 = await call('/api/auth/verify-step', { body: { ticket: s1.data.ticket, credential: totp(g.secret), platform_id: 'ops-platform' }, platform: ops });
  T('  第二步 TOTP 通过，签发最终 token', s2.code === 0 && !!s2.data.token, true);
  const s2exp = await call('/api/auth/verify-step', { body: { ticket: 'deadbeef', credential: totp(g.secret), platform_id: 'ops-platform' }, platform: ops });
  T('  无效 ticket 被拒', s2exp.code, 1007);
  // 2.3 未启用 TOTP 的用户走多步骤：第一步后第二步 TOTP 报未启用
  const b1 = await call('/api/auth/verify', { body: { method: 'username_password', identifier: 'bob', credential: 'Bob@1234', platform_id: 'ops-platform' }, platform: ops });
  const b2 = await call('/api/auth/verify-step', { body: { ticket: b1.data.ticket, credential: '123456', platform_id: 'ops-platform' }, platform: ops });
  T('  未启用TOTP用户第二步被拒(1003)', b2.code, 1003);

  console.log('\n== 场景3：手机号+验证码 单步 ==');
  await setMethods(['phone_code']);
  const sc = await call('/api/auth/send-code', { body: { method: 'phone_code', identifier: '13800138000', platform_id: 'ops-platform' }, platform: ops });
  T('  send-code 返回 dev_code', sc.code === 0 && /^\d{6}$/.test(sc.data.dev_code), true);
  const v1 = await call('/api/auth/verify', { body: { method: 'phone_code', identifier: '13800138000', credential: sc.data.dev_code, platform_id: 'ops-platform' }, platform: ops });
  T('  手机号+验证码登录成功', v1.code === 0 && !!v1.data.token, true);
  const v2 = await call('/api/auth/verify', { body: { method: 'phone_code', identifier: '13800138000', credential: '999999', platform_id: 'ops-platform' }, platform: ops });
  T('  错误验证码被拒(1003)', v2.code, 1003);

  console.log('\n== 场景4：邮箱+密码 单步 ==');
  await setMethods(['email_password']);
  const e1 = await call('/api/auth/verify', { body: { method: 'email_password', identifier: 'alice@example.com', credential: 'Alice@123', platform_id: 'ops-platform' }, platform: ops });
  T('  邮箱+密码登录成功', e1.code === 0 && !!e1.data.token, true);

  console.log('\n== 场景5：登录方式配置校验 ==');
  const bad1 = await call('/api/admin/settings/login_methods', { method: 'PUT', body: { methods: [] }, auth: true, raw: true });
  T('  空方式被拒', bad1.code, 1007);
  const bad2 = await call('/api/admin/settings/login_methods', { method: 'PUT', body: { methods: ['totp'] }, auth: true, raw: true });
  T('  仅 TOTP 被拒', bad2.code, 1007);
  const bad3 = await call('/api/admin/settings/login_methods', { method: 'PUT', body: { methods: ['username_password', 'username_password'] }, auth: true, raw: true });
  T('  重复方式被拒', bad3.code, 1007);

  console.log('\n== 场景6：后台登录 IP 白名单 ==');
  const wl = await call('/api/admin/settings/admin_ip_whitelist', { method: 'PUT', body: { ips: ['1.2.3.4'] }, auth: true });
  T('  设置白名单 code=0', wl.code, 0);
  const blocked = await call('/api/admin/login', { method: 'POST', body: { username: 'admin', password: 'admin123' }, raw: true });
  T('  白名单外 IP 登录被拒(1009)', blocked.code, 1009);
  const wl2 = await call('/api/admin/settings/admin_ip_whitelist', { method: 'PUT', body: { ips: [] }, auth: true });
  T('  清空白名单 code=0', wl2.code, 0);
  await loginAdmin();
  T('  清空后 admin 恢复可登录', adminToken !== '', true);

  console.log('\n== 场景7：密码策略设置生效 ==');
  await call('/api/admin/settings/password_policy', { method: 'PUT', body: { min_length: 12, require_letter: true, require_digit: true, require_special: true }, auth: true });
  const weak = await call('/api/admin/users', { method: 'POST', body: { username: 'newbie', password: 'Short1', nickname: '新用户' }, auth: true, raw: true });
  T('  弱密码被拒(1007)', weak.code, 1007);
  const strong = await call('/api/admin/users', { method: 'POST', body: { username: 'newbie', password: 'Str0ng!Pass123', nickname: '新用户' }, auth: true });
  T('  强密码创建成功', strong.code, 0);
  await call('/api/admin/settings/password_policy', { method: 'PUT', body: { min_length: 8, require_letter: true, require_digit: true, require_special: false }, auth: true });
  // 清理测试用户
  const newbie = (await call('/api/admin/users?keyword=newbie', { auth: true })).data.users.find(u => u.username === 'newbie');
  if (newbie) await call('/api/admin/users/' + newbie.id, { method: 'DELETE', auth: true });

  console.log('\n== 收尾：恢复默认登录方式 ==');
  await setMethods(['username_password']);

  console.log(`\n===== 结果: ${pass} PASS, ${fail} FAIL =====`);
  process.exit(fail ? 1 : 0);
}

main().catch(e => { console.error('脚本异常:', e); process.exit(1); });

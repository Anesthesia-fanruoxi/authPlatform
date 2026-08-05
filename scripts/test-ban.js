// 黑名单管理端到端测试。用法: node scripts/test-ban.js
const crypto = require('crypto');
const BASE = 'http://127.0.0.1:8080';
const ops = { id: 'ops-platform', secret: '7020698c56ab7d58a9b0b8929be3bff2b19c0f25601f3f6bb215cfbd083ba8d4' };

let adminToken = '';
async function call(path, { method = 'GET', body, auth = false, platform, raw = false } = {}) {
  const headers = { 'Content-Type': 'application/json' };
  if (auth) headers['Authorization'] = 'Bearer ' + adminToken;
  let payload;
  if (platform) {
    method = 'POST';
    payload = typeof body === 'string' ? body : JSON.stringify(body);
    const ts = Math.floor(Date.now() / 1000);
    const bodyHash = crypto.createHash('sha256').update(payload).digest('hex');
    const sig = crypto.createHmac('sha256', platform.secret).update(`POST|${path}|${ts}|${bodyHash}`).digest('hex');
    headers['X-Platform-Id'] = platform.id;
    headers['X-Timestamp'] = String(ts);
    headers['X-Sign'] = sig;
  } else if (body !== undefined) {
    payload = JSON.stringify(body);
  }
  const resp = await fetch(BASE + path, { method, headers, body: payload });
  const json = await resp.json();
  if (json.code !== 0 && !platform && !raw) throw Object.assign(new Error(json.msg), { code: json.code });
  return json;
}

let pass = 0, fail = 0;
function T(name, actual, expect) {
  const ok = actual === expect;
  ok ? pass++ : fail++;
  console.log(`  [${ok ? 'PASS' : 'FAIL'}] ${name}: got=${actual} expect=${expect}`);
}
async function verifyLogin(username, password) {
  return call('/api/auth/verify', { body: { username, password, platform_id: 'ops-platform' }, platform: ops });
}

async function main() {
  const r = await call('/api/admin/login', { method: 'POST', body: { username: 'admin', password: 'admin123' } });
  adminToken = r.data.token;
  console.log('== 场景1：手动拉黑（永久）与解除 ==');
  const add = await call('/api/admin/bans', { method: 'POST', body: { username: 'alice', reason: '违规操作' }, auth: true });
  T('  手动拉黑 alice', add.code, 0);
  const v1 = await verifyLogin('alice', 'Alice@123');
  T('  alice 登录被拒(1005)', v1.code, 1005);
  T('  提示包含黑名单', v1.msg.includes('黑名单'), true);
  const list1 = (await call('/api/admin/bans', { auth: true })).data.bans;
  const aliceRec = list1.find(b => b.username === 'alice');
  T('  列表含 alice 手动记录', !!aliceRec && aliceRec.source === 'manual' && aliceRec.status === 'permanent', true);
  const rm = await call('/api/admin/bans/alice', { method: 'DELETE', auth: true });
  T('  解除 alice', rm.code, 0);
  const v2 = await verifyLogin('alice', 'Alice@123');
  T('  解除后 alice 登录成功', v2.code === 0, true);

  console.log('\n== 场景2：自动锁定写入黑名单与解除 ==');
  for (let i = 0; i < 5; i++) await verifyLogin('frank', 'wrong-pass-' + i);
  const v3 = await verifyLogin('frank', 'Frank@123');
  T('  frank 第6次(密码正确)被锁', v3.code, 1005);
  const list2 = (await call('/api/admin/bans', { auth: true })).data.bans;
  const frankRec = list2.find(b => b.username === 'frank');
  T('  列表含 frank 自动锁定', !!frankRec && frankRec.source === 'auto', true);
  await call('/api/admin/bans/frank', { method: 'DELETE', auth: true });
  const v4 = await verifyLogin('frank', 'Frank@123');
  T('  解除后 frank 登录成功', v4.code === 0, true);

  console.log('\n== 场景3：临时黑名单（过去时间=已过期，不拦截） ==');
  const past = '2000-01-01T00:00:00';
  const add2 = await call('/api/admin/bans', { method: 'POST', body: { username: 'bob', reason: '临时', expires_at: past }, auth: true });
  T('  添加已过期黑名单', add2.code, 0);
  const list3 = (await call('/api/admin/bans', { auth: true })).data.bans;
  const bobRec = list3.find(b => b.username === 'bob');
  T('  列表标记已过期', !!bobRec && bobRec.status === 'expired', true);
  const v5 = await verifyLogin('bob', 'Bob@1234');
  T('  bob 登录不受过期黑名单影响', v5.code, 0);
  // 清理
  await call('/api/admin/bans/bob', { method: 'DELETE', auth: true });

  console.log('\n== 场景4：黑名单账号不存在校验 ==');
  const add3 = await call('/api/admin/bans', { method: 'POST', body: { username: 'ghost_user', reason: 'x' }, auth: true, raw: true });
  T('  不存在账号拉黑被拒(1007)', add3.code, 1007);

  console.log(`\n===== 结果: ${pass} PASS, ${fail} FAIL =====`);
  process.exit(fail ? 1 : 0);
}
main().catch(e => { console.error('脚本异常:', e); process.exit(1); });

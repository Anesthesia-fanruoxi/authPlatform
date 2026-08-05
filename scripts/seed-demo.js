// authPlatform 演示数据造数 + 登录测试脚本（可重复运行：已存在的平台/用户自动跳过）
// 用法: node scripts/seed-demo.js
const crypto = require('crypto');
const BASE = process.env.APP_BASE || 'http://127.0.0.1:8080';

const PLATFORMS = [
  { platform_id: 'ops-platform', name: '运维平台', secret: '7020698c56ab7d58a9b0b8929be3bff2b19c0f25601f3f6bb215cfbd083ba8d4' },
  { platform_id: 'cmdb-platform', name: '资产CMDB', secret: 'ec0fbf412d7e511eeb1d5834ada8ac3e5b68222f3c43b577be3e30d3d829c5c3' },
  { platform_id: 'monitor-platform', name: '监控平台', secret: 'b8c0b49f89bb90e67187db8c7f391abc1106585c8e2882b50ab7c1c4d631d819' },
  { platform_id: 'data-platform', name: '数据平台', secret: '1c0a3783f985164d1f74e51dd9f5017d873a199e82c3c3c42b3f3424f46b2f81' },
];

const USERS = [
  { username: 'alice', password: 'Alice@123', nickname: '爱丽丝', grants: ['ops-platform', 'cmdb-platform', 'monitor-platform', 'data-platform'] },
  { username: 'bob', password: 'Bob@1234', nickname: '鲍勃', grants: ['ops-platform', 'cmdb-platform'] },
  { username: 'carol', password: 'Carol@123', nickname: '卡罗尔', grants: ['ops-platform'], disabled: true },
  { username: 'dave', password: 'Dave@123', nickname: '戴夫', grants: [] },
  { username: 'eve', password: 'Eve@1234', nickname: '伊芙', grants: ['monitor-platform'] },
  { username: 'frank', password: 'Frank@123', nickname: '弗兰克', grants: ['ops-platform'] }, // 限流演示专用
];

// ---------- HTTP 封装 ----------
let adminToken = '';
async function call(path, { method = 'GET', body, auth = false } = {}) {
  const headers = { 'Content-Type': 'application/json' };
  if (auth) headers['Authorization'] = 'Bearer ' + adminToken;
  const resp = await fetch(BASE + path, { method, headers, body: body ? JSON.stringify(body) : undefined });
  const json = await resp.json();
  if (json.code !== 0) throw Object.assign(new Error(json.msg || '请求失败'), { code: json.code, data: json });
  return json.data;
}

// ---------- 平台签名 ----------
function sign(secret, method, path, timestamp, body) {
  const bodyHash = crypto.createHash('sha256').update(body).digest('hex');
  return crypto.createHmac('sha256', secret).update(`${method}|${path}|${timestamp}|${bodyHash}`).digest('hex');
}
async function verify(platformId, secret, username, password, opts = {}) {
  const body = JSON.stringify({ username, password, platform_id: platformId });
  const ts = opts.timestamp !== undefined ? opts.timestamp : Math.floor(Date.now() / 1000);
  const sig = opts.badSign ? 'deadbeef'.repeat(8) : sign(secret, 'POST', '/api/auth/verify', String(ts), body);
  const headers = {
    'Content-Type': 'application/json',
    'X-Platform-Id': opts.platformHeader !== undefined ? opts.platformHeader : platformId,
    'X-Timestamp': String(ts),
    'X-Sign': sig,
    ...(opts.headers || {}),
  };
  const resp = await fetch(BASE + '/api/auth/verify', { method: 'POST', headers, body });
  return resp.json();
}

// ---------- 造数据 ----------
async function main() {
  console.log('== 1. 管理员登录 ==');
  const login = await call('/api/admin/login', { method: 'POST', body: { username: 'admin', password: 'admin123' } });
  adminToken = login.token;
  console.log('  admin 登录成功:', login.user.username, login.user.uid);

  console.log('\n== 2. 注册平台 ==');
  const platformIds = {};   // platform_id -> {id, secret}
  const platformSecrets = {};
  for (const p of PLATFORMS) {
    try {
      const data = await call('/api/admin/platforms', { method: 'POST', body: p, auth: true });
      platformIds[p.platform_id] = data.id;
      platformSecrets[p.platform_id] = data.secret;
      console.log(`  [新建] ${p.platform_id.padEnd(17)} id=${data.id}`);
    } catch (e) {
      if (e.code === 1007) {
        const list = await call('/api/admin/platforms', { auth: true });
        const found = list.platforms.find(x => x.platform_id === p.platform_id);
        platformIds[p.platform_id] = found.id;
        platformSecrets[p.platform_id] = p.secret; // 演示脚本：使用首次展示并固化的 secret
        console.log(`  [已存在] ${p.platform_id.padEnd(17)} id=${found.id}`);
      } else {
        console.log(`  [失败] ${p.platform_id}: ${e.message}`);
      }
    }
  }

  console.log('\n== 3. 创建用户 ==');
  for (const u of USERS) {
    try {
      await call('/api/admin/users', { method: 'POST', body: { username: u.username, password: u.password, nickname: u.nickname }, auth: true });
      console.log(`  [新建] ${u.username}`);
    } catch (e) {
      if (e.code === 1008) {
        console.log(`  [已存在] ${u.username}`);
      } else {
        console.log(`  [失败] ${u.username}: ${e.message}`);
      }
    }
  }
  // 用户 id 需从授权矩阵接口获取（SafeUser 不返回 id）
  const userIds = {};
  const matrix = await call('/api/admin/grants', { auth: true });
  for (const u of matrix.users) userIds[u.username] = u.id;
  for (const u of USERS) console.log(`  id: ${u.username} = ${userIds[u.username]}`);

  console.log('\n== 4. 配置授权 ==');
  for (const u of USERS) {
    const ids = u.grants.map(g => platformIds[g]).filter(Boolean);
    await call(`/api/admin/users/${userIds[u.username]}/grants`, { method: 'POST', body: { platform_ids: ids }, auth: true });
    console.log(`  ${u.username.padEnd(8)} -> ${u.grants.join(', ') || '(无授权)'}`);
    if (u.disabled) {
      await call(`/api/admin/users/${userIds[u.username]}`, { method: 'PUT', body: { status: 0 }, auth: true });
      console.log(`  ${u.username.padEnd(8)} 状态已改为【禁用】`);
    }
  }

  // ---------- 登录测试 ----------
  console.log('\n== 5. 登录测试 ==');
  const S = platformSecrets; // 仅新建平台有 secret
  const cases = [];
  const addCase = (name, fn) => cases.push({ name, fn });

  if (S['ops-platform'] && S['cmdb-platform'] && S['monitor-platform']) {
    addCase('alice 正确密码登录 ops-platform', () =>
      verify('ops-platform', S['ops-platform'], 'alice', 'Alice@123'));
    addCase('bob 正确密码登录 cmdb-platform', () =>
      verify('cmdb-platform', S['cmdb-platform'], 'bob', 'Bob@1234'));
    addCase('eve 正确密码登录 monitor-platform', () =>
      verify('monitor-platform', S['monitor-platform'], 'eve', 'Eve@1234'));
    addCase('alice 错误密码登录 ops-platform', () =>
      verify('ops-platform', S['ops-platform'], 'alice', 'wrong-pass-1'));
    addCase('bob 登录未授权的 monitor-platform', () =>
      verify('monitor-platform', S['monitor-platform'], 'bob', 'Bob@1234'));
    addCase('dave（无授权）登录 ops-platform', () =>
      verify('ops-platform', S['ops-platform'], 'dave', 'Dave@123'));
    addCase('carol（已禁用）登录 ops-platform', () =>
      verify('ops-platform', S['ops-platform'], 'carol', 'Carol@123'));
    addCase('eve 错误签名登录 monitor-platform', () =>
      verify('monitor-platform', S['monitor-platform'], 'eve', 'Eve@123', { badSign: true }));
    addCase('eve 登录不存在的 ghost-platform', () =>
      verify('ghost-platform', 'fake-secret', 'eve', 'Eve@123'));
    addCase('eve 过期时间戳(600s前)', () =>
      verify('monitor-platform', S['monitor-platform'], 'eve', 'Eve@123', { timestamp: Math.floor(Date.now() / 1000) - 600 }));
    addCase('frank 连续5次错密码后正确密码（限流锁定）', async () => {
      for (let i = 0; i < 5; i++) await verify('ops-platform', S['ops-platform'], 'frank', 'wrong-pass-' + i);
      return verify('ops-platform', S['ops-platform'], 'frank', 'Frank@123');
    });
  } else {
    console.log('  [跳过] 部分平台已存在导致拿不到 secret，无法构造合法签名，仅保留错误签名类用例');
    addCase('eve 错误签名登录 monitor-platform', () =>
      verify('monitor-platform', 'any-secret', 'eve', 'Eve@123', { badSign: true }));
  }

  const expectMap = {
    'alice 正确密码登录 ops-platform': 0,
    'bob 正确密码登录 cmdb-platform': 0,
    'eve 正确密码登录 monitor-platform': 0,
    'alice 错误密码登录 ops-platform': 1003,
    'bob 登录未授权的 monitor-platform': 1006,
    'dave（无授权）登录 ops-platform': 1006,
    'carol（已禁用）登录 ops-platform': 1004,
    'eve 错误签名登录 monitor-platform': 1001,
    'eve 登录不存在的 ghost-platform': 1002,
    'eve 过期时间戳(600s前)': 1001,
    'frank 连续5次错密码后正确密码（限流锁定）': 1005,
  };

  for (const c of cases) {
    const res = await c.fn();
    const expect = expectMap[c.name];
    const pass = res.code === expect;
    const mark = pass ? 'PASS' : 'FAIL';
    console.log(`  [${mark}] ${c.name}: code=${res.code}（期望 ${expect}）${res.code === 0 ? ' token=' + String(res.data && res.data.token).slice(0, 16) + '...' : ' msg=' + res.msg}`);
  }

  const failCount = cases.filter(c => {
    return true;
  }).length;
  console.log(`\n  共 ${cases.length} 个用例`);
}

main().catch(e => { console.error('脚本异常:', e); process.exit(1); });

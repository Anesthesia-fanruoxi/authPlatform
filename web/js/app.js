// authPlatform 管理控制台前端（Vue 3 + Element Plus，无构建步骤）
// 视觉主题：深色侧边栏 + 玻璃拟态登录页 + 品牌蓝色系（详见 index.html 内联样式）
const { createApp, ref, reactive, computed, onMounted, onActivated, onBeforeUnmount } = Vue;
const { ElMessage, ElMessageBox } = ElementPlus;

// ---------- 内联 SVG 图标（stroke 风格，随 currentColor 渲染） ----------
const Ic = {
  shield: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/></svg>',
  users: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2"/><circle cx="9" cy="7" r="4"/><path d="M23 21v-2a4 4 0 0 0-3-3.87"/><path d="M16 3.13a4 4 0 0 1 0 7.75"/></svg>',
  grid: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="3" width="7" height="7" rx="1.5"/><rect x="14" y="3" width="7" height="7" rx="1.5"/><rect x="14" y="14" width="7" height="7" rx="1.5"/><rect x="3" y="14" width="7" height="7" rx="1.5"/></svg>',
  key: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 2l-2 2m-7.61 7.61a5.5 5.5 0 1 1-7.778 7.778 5.5 5.5 0 0 1 7.777-7.777zm0 0L15.5 7.5m0 0l3 3L22 7l-3-3m-3.5 3.5L19 4"/></svg>',
  list: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M8 6h13M8 12h13M8 18h13"/><path d="M3 6h.01M3 12h.01M3 18h.01"/></svg>',
  user: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2"/><circle cx="12" cy="7" r="4"/></svg>',
  lock: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="11" width="18" height="11" rx="2"/><path d="M7 11V7a5 5 0 0 1 10 0v4"/></svg>',
  refresh: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="23 4 23 10 17 10"/><path d="M20.49 15a9 9 0 1 1-2.12-9.36L23 10"/></svg>',
};
// 通用：把 SVG 图标字符串转成可插入模板的元素（避免模板里重复写长 path）
function svg(name, cls) {
  return `<span class="${cls || 'ic'}">${Ic[name]}</span>`;
}

// 登录方式选项（与后端 common.LoginMethod* 对齐）
const LOGIN_METHOD_OPTIONS = [
  { value: 'username_password', label: '用户名 + 密码' },
  { value: 'email_password', label: '邮箱 + 密码' },
  { value: 'phone_code', label: '手机号 + 验证码' },
  { value: 'totp', label: 'TOTP 双因子验证' },
];
function loginMethodLabel(m) {
  const found = LOGIN_METHOD_OPTIONS.find(o => o.value === m);
  return found ? found.label : m;
}

// ---------- API 封装 ----------
const api = {
  async request(path, options = {}) {
    const headers = { 'Content-Type': 'application/json', ...(options.headers || {}) };
    const token = localStorage.getItem('token');
    if (token) headers['Authorization'] = 'Bearer ' + token;
    const resp = await fetch(path, { ...options, headers });
    const body = await resp.json();
    if (body.code !== 0) {
      const err = new Error(body.msg || '请求失败');
      err.code = body.code;
      throw err;
    }
    return body.data;
  },
  login(username, password) {
    return this.request('/api/admin/login', { method: 'POST', body: JSON.stringify({ username, password }) });
  },
  me() { return this.request('/api/admin/me'); },
  // 用户管理
  listUsers(keyword) {
    const q = keyword ? '?keyword=' + encodeURIComponent(keyword) : '';
    return this.request('/api/admin/users' + q);
  },
  createUser(payload) { return this.request('/api/admin/users', { method: 'POST', body: JSON.stringify(payload) }); },
  updateUser(id, payload) { return this.request('/api/admin/users/' + id, { method: 'PUT', body: JSON.stringify(payload) }); },
  deleteUser(id) { return this.request('/api/admin/users/' + id, { method: 'DELETE' }); },
  resetPassword(id, new_password) {
    return this.request('/api/admin/users/' + id + '/reset-password', { method: 'POST', body: JSON.stringify({ new_password }) });
  },
  // 平台管理
  listPlatforms() { return this.request('/api/admin/platforms'); },
  createPlatform(payload) { return this.request('/api/admin/platforms', { method: 'POST', body: JSON.stringify(payload) }); },
  updatePlatform(id, payload) { return this.request('/api/admin/platforms/' + id, { method: 'PUT', body: JSON.stringify(payload) }); },
  deletePlatform(id) { return this.request('/api/admin/platforms/' + id, { method: 'DELETE' }); },
  rotateSecret(id) { return this.request('/api/admin/platforms/' + id + '/rotate-secret', { method: 'POST' }); },
  // 授权管理
  grantsMatrix() { return this.request('/api/admin/grants'); },
  setUserGrants(id, platform_ids) {
    return this.request('/api/admin/users/' + id + '/grants', { method: 'POST', body: JSON.stringify({ platform_ids }) });
  },
  // 审计日志
  listLogs(params) {
    const q = new URLSearchParams();
    Object.entries(params || {}).forEach(([k, v]) => { if (v !== '' && v !== undefined && v !== null) q.set(k, v); });
    const s = q.toString();
    return this.request('/api/admin/logs' + (s ? '?' + s : ''));
  },
  // 系统设置
  listSettings() { return this.request('/api/admin/settings'); },
  updateSettings(key, payload) {
    return this.request('/api/admin/settings/' + key, { method: 'PUT', body: JSON.stringify(payload) });
  },
  // 黑名单管理
  listBans() { return this.request('/api/admin/bans'); },
  addBan(payload) { return this.request('/api/admin/bans', { method: 'POST', body: JSON.stringify(payload) }); },
  removeBan(username) { return this.request('/api/admin/bans/' + encodeURIComponent(username), { method: 'DELETE' }); },
};

// ---------- 表格铺满工具：实测容器高度，避免百分比高度在切换/缩放后错乱 ----------
function useTableFill() {
  const tableHeight = ref(300);
  const boxRef = ref(null);
  let ro = null;
  const calc = () => {
    const box = boxRef.value;
    if (box) tableHeight.value = Math.max(120, box.clientHeight);
  };
  onMounted(() => {
    calc();
    ro = new ResizeObserver(calc);
    if (boxRef.value) ro.observe(boxRef.value);
    window.addEventListener('resize', calc);
  });
  onActivated(() => calc()); // keep-alive 切回时重算
  onBeforeUnmount(() => {
    if (ro) ro.disconnect();
    window.removeEventListener('resize', calc);
  });
  return { tableHeight, boxRef };
}

// ---------- 用户管理页 ----------
const UsersPage = {
  name: 'UsersPage',
  template: `
    <el-card class="page-card">
      <template #header>
        <div style="display:flex;justify-content:space-between;align-items:center">
          <div class="card-title">${svg('users')}用户管理</div>
          <div>
            <el-input v-model="keyword" placeholder="搜索用户名/昵称" style="width:200px;margin-right:8px" clearable @keyup.enter="load">
              <template #prefix>${svg('user', 'ipt-ic')}</template>
            </el-input>
            <el-button type="primary" @click="openCreate">新建用户</el-button>
          </div>
        </div>
      </template>
      <div class="table-box" ref="boxRef">
      <el-table :data="users" v-loading="loading" stripe :height="tableHeight">
        <el-table-column prop="uid" label="UID" width="230">
          <template #default="{row}"><span style="font-family:Consolas,Menlo,monospace;font-size:12px;color:#5a6b82">{{ row.uid }}</span></template>
        </el-table-column>
        <el-table-column label="用户名">
          <template #default="{row}">
            <span class="cell-user">
              <span class="avatar" :style="avatarStyle(row)">{{ (row.nickname || row.username).slice(0,1) }}</span>
              <b>{{ row.username }}</b>
            </span>
          </template>
        </el-table-column>
        <el-table-column prop="nickname" label="昵称" />
        <el-table-column label="手机号" min-width="120">
          <template #default="{row}"><span style="color:#5a6b82">{{ row.phone || '—' }}</span></template>
        </el-table-column>
        <el-table-column label="邮箱" min-width="160">
          <template #default="{row}"><span style="color:#5a6b82">{{ row.email || '—' }}</span></template>
        </el-table-column>
        <el-table-column label="状态" width="90" align="center">
          <template #default="{row}">
            <el-tag :type="row.status === 1 ? 'success' : 'danger'" effect="light" round>
              {{ row.status === 1 ? '启用' : '禁用' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="角色" width="90" align="center">
          <template #default="{row}">
            <el-tag v-if="row.is_admin" type="warning" effect="light" round>管理员</el-tag>
            <span v-else style="color:#8a97ab">普通</span>
          </template>
        </el-table-column>
        <el-table-column label="双因子" width="100" align="center">
          <template #default="{row}">
            <el-tag v-if="row.totp_enabled" type="success" effect="light" round>已启用</el-tag>
            <span v-else style="color:#8a97ab">未启用</span>
          </template>
        </el-table-column>
        <el-table-column prop="created_at" label="创建时间" width="190" />
        <el-table-column label="操作" width="300" align="center">
          <template #default="{row}">
            <el-button link type="primary" @click="openEdit(row)">编辑</el-button>
            <el-button link :type="row.status === 1 ? 'danger' : 'success'" @click="toggleStatus(row)">
              {{ row.status === 1 ? '禁用' : '启用' }}
            </el-button>
            <el-button link type="warning" @click="openReset(row)">重置密码</el-button>
            <el-button link type="danger" @click="del(row)">删除</el-button>
          </template>
        </el-table-column>
        <template #empty>
          <div class="table-empty">
            <span class="table-empty-ic">${svg('users')}</span>
            <p>暂无用户数据</p>
          </div>
        </template>
      </el-table>
      </div>
    </el-card>

    <el-dialog v-model="dlg.visible" :title="dlg.isEdit ? '编辑用户' : '新建用户'" width="420" align-center>
      <el-form label-width="80px">
        <el-form-item label="用户名"><el-input v-model="dlg.form.username" :disabled="dlg.isEdit" placeholder="请输入用户名" /></el-form-item>
        <el-form-item label="昵称"><el-input v-model="dlg.form.nickname" placeholder="请输入昵称" /></el-form-item>
        <el-form-item label="手机号"><el-input v-model="dlg.form.phone" placeholder="选填，用于手机号登录/验证码" /></el-form-item>
        <el-form-item label="邮箱"><el-input v-model="dlg.form.email" placeholder="选填，用于邮箱登录/验证码" /></el-form-item>
        <el-form-item v-if="!dlg.isEdit" label="密码">
          <el-input v-model="dlg.form.password" type="password" show-password placeholder="至少8位，含字母和数字" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dlg.visible = false">取消</el-button>
        <el-button type="primary" :loading="dlg.saving" @click="save">保存</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="pwdDlg.visible" title="重置密码" width="400" align-center>
      <el-form label-width="80px">
        <el-form-item label="账号">{{ pwdDlg.username }}</el-form-item>
        <el-form-item label="新密码">
          <el-input v-model="pwdDlg.password" type="password" show-password placeholder="至少8位，含字母和数字" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="pwdDlg.visible = false">取消</el-button>
        <el-button type="primary" :loading="pwdDlg.saving" @click="savePwd">确定</el-button>
      </template>
    </el-dialog>

  `,
  setup() {
    const users = ref([]);
    const keyword = ref('');
    const loading = ref(false);
    const { tableHeight, boxRef } = useTableFill();
    const dlg = reactive({ visible: false, isEdit: false, saving: false, form: { id: 0, username: '', nickname: '', phone: '', email: '', password: '' } });
    const pwdDlg = reactive({ visible: false, saving: false, id: 0, username: '', password: '' });

    const avatarPalette = ['#2f6fed', '#7a5cff', '#0ea5a4', '#e07a3f', '#d64f8c', '#3f9e4d'];
    function avatarStyle(row) {
      let h = 0;
      const s = row.username || '';
      for (let i = 0; i < s.length; i++) h = (h * 31 + s.charCodeAt(i)) >>> 0;
      const c = avatarPalette[h % avatarPalette.length];
      return { width: '26px', height: '26px', 'font-size': '12px', background: 'linear-gradient(135deg, ' + c + ', ' + c + 'cc)' };
    }

    async function load() {
      loading.value = true;
      try {
        const data = await api.listUsers(keyword.value);
        users.value = data.users || [];
      } catch (e) { ElMessage.error(e.message); }
      finally { loading.value = false; }
    }
    onMounted(load);

    function openCreate() {
      dlg.isEdit = false;
      dlg.form = { id: 0, username: '', nickname: '', phone: '', email: '', password: '' };
      dlg.visible = true;
    }
    function openEdit(row) {
      dlg.isEdit = true;
      dlg.form = { id: row.id, username: row.username, nickname: row.nickname, phone: row.phone || '', email: row.email || '', password: '' };
      dlg.visible = true;
    }
    async function save() {
      dlg.saving = true;
      try {
        if (dlg.isEdit) {
          await api.updateUser(dlg.form.id, { nickname: dlg.form.nickname, phone: dlg.form.phone, email: dlg.form.email });
        } else {
          await api.createUser({ username: dlg.form.username, password: dlg.form.password, nickname: dlg.form.nickname, phone: dlg.form.phone, email: dlg.form.email });
        }
        ElMessage.success('保存成功');
        dlg.visible = false;
        load();
      } catch (e) { ElMessage.error(e.message); }
      finally { dlg.saving = false; }
    }
    async function toggleStatus(row) {
      try {
        await api.updateUser(row.id, { status: row.status === 1 ? 0 : 1 });
        ElMessage.success('已更新');
        load();
      } catch (e) { ElMessage.error(e.message); }
    }
    function openReset(row) {
      pwdDlg.id = row.id;
      pwdDlg.username = row.username;
      pwdDlg.password = '';
      pwdDlg.visible = true;
    }
    async function savePwd() {
      pwdDlg.saving = true;
      try {
        await api.resetPassword(pwdDlg.id, pwdDlg.password);
        ElMessage.success('密码已重置');
        pwdDlg.visible = false;
      } catch (e) { ElMessage.error(e.message); }
      finally { pwdDlg.saving = false; }
    }
    async function del(row) {
      try {
        await ElMessageBox.confirm(`确定删除用户「${row.username}」？该操作不可恢复。`, '提示', { type: 'warning' });
      } catch { return; }
      try {
        await api.deleteUser(row.id);
        ElMessage.success('已删除');
        load();
      } catch (e) { ElMessage.error(e.message); }
    }

    return { users, keyword, loading, tableHeight, boxRef, dlg, pwdDlg, avatarStyle, load, openCreate, openEdit, save, toggleStatus, openReset, savePwd, del };
  },
};

// ---------- 平台管理页 ----------
const PlatformsPage = {
  name: 'PlatformsPage',
  template: `
    <el-card class="page-card">
      <template #header>
        <div style="display:flex;justify-content:space-between;align-items:center">
          <div class="card-title">${svg('grid')}平台管理</div>
          <el-button type="primary" @click="openCreate">注册平台</el-button>
        </div>
      </template>
      <div class="table-box" ref="boxRef">
      <el-table :data="platforms" v-loading="loading" stripe :height="tableHeight">
        <el-table-column prop="platform_id" label="平台标识" width="160">
          <template #default="{row}"><span style="font-family:Consolas,Menlo,monospace;color:#2f6fed">{{ row.platform_id }}</span></template>
        </el-table-column>
        <el-table-column prop="name" label="名称" />
        <el-table-column label="登录方式" min-width="170">
          <template #default="{row}">
            <template v-if="row.login_methods_custom && row.login_methods && row.login_methods.length">
              <el-tag v-for="m in row.login_methods" :key="m" size="small" type="primary" effect="plain" round style="margin-right:4px">{{ loginMethodLabel(m) }}</el-tag>
            </template>
            <el-tag v-else size="small" type="info" effect="plain" round>系统默认</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="加密盐" width="190">
          <template #default="{row}">
            <span style="font-family:Consolas,Menlo,monospace;font-size:12px;color:#5a6b82">{{ row.secret_masked || '***' }}</span>
            <el-tag v-if="row.has_old_secret" type="warning" size="small" effect="light" round style="margin-left:6px">过渡期</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="90" align="center">
          <template #default="{row}">
            <el-tag :type="row.status === 1 ? 'success' : 'danger'" effect="light" round>{{ row.status === 1 ? '启用' : '停用' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="created_at" label="创建时间" width="190" />
        <el-table-column label="操作" width="300" align="center">
          <template #default="{row}">
            <el-button link type="primary" @click="openEdit(row)">编辑</el-button>
            <el-button link type="warning" @click="rotate(row)">轮换密钥</el-button>
            <el-button link :type="row.status === 1 ? 'danger' : 'success'" @click="toggleStatus(row)">
              {{ row.status === 1 ? '停用' : '启用' }}
            </el-button>
            <el-button link type="danger" @click="del(row)">删除</el-button>
          </template>
        </el-table-column>
        <template #empty>
          <div class="table-empty">
            <span class="table-empty-ic">${svg('grid')}</span>
            <p>暂无平台数据，点击右上角注册平台</p>
          </div>
        </template>
      </el-table>
      </div>
    </el-card>

    <el-dialog v-model="dlg.visible" :title="dlg.isEdit ? '编辑平台' : '注册平台'" width="460" align-center>
      <el-form label-width="100px">
        <el-form-item label="平台标识">
          <el-input v-model="dlg.form.platform_id" :disabled="dlg.isEdit" placeholder="如 ops-platform" />
        </el-form-item>
        <el-form-item label="名称"><el-input v-model="dlg.form.name" placeholder="请输入平台名称" /></el-form-item>
        <el-form-item label="IP 白名单">
          <el-input v-model="dlg.form.ip_whitelist" placeholder='JSON 数组，如 ["1.2.3.4"]，留空不限制' />
        </el-form-item>
        <el-form-item label="登录方式">
          <el-checkbox-group v-model="dlg.form.login_methods" style="display:flex;flex-direction:column;gap:8px">
            <el-checkbox v-for="m in methodOptions" :key="m.value" :label="m.value">{{ m.label }}</el-checkbox>
          </el-checkbox-group>
          <div class="settings-tip">留空 = 使用系统设置中的「新平台默认登录方式」；多选 = 多步骤验证。</div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dlg.visible = false">取消</el-button>
        <el-button type="primary" :loading="dlg.saving" @click="save">保存</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="secretDlg.visible" title="平台加密盐（仅此一次展示，请立即保存）" width="560" align-center>
      <el-alert type="warning" :closable="false" show-icon style="margin-bottom:12px"
        title="请平台侧妥善保存该 secret，用于请求签名；关闭后将无法再次查看。" />
      <div style="display:flex;align-items:center;gap:8px">
        <el-input :model-value="secretDlg.secret" readonly>
          <template #append><el-button @click="copySecret">复制</el-button></template>
        </el-input>
      </div>
      <template #footer><el-button type="primary" @click="secretDlg.visible = false">我已保存</el-button></template>
    </el-dialog>
  `,
  setup() {
    const platforms = ref([]);
    const loading = ref(false);
    const { tableHeight, boxRef } = useTableFill();
    const dlg = reactive({ visible: false, isEdit: false, saving: false, form: { id: 0, platform_id: '', name: '', ip_whitelist: '', login_methods: [] } });
    const secretDlg = reactive({ visible: false, secret: '' });
    const methodOptions = LOGIN_METHOD_OPTIONS;

    async function load() {
      loading.value = true;
      try {
        const data = await api.listPlatforms();
        platforms.value = data.platforms || [];
      } catch (e) { ElMessage.error(e.message); }
      finally { loading.value = false; }
    }
    onMounted(load);

    function openCreate() {
      dlg.isEdit = false;
      dlg.form = { id: 0, platform_id: '', name: '', ip_whitelist: '', login_methods: [] };
      dlg.visible = true;
    }
    function openEdit(row) {
      dlg.isEdit = true;
      dlg.form = { id: row.id, platform_id: row.platform_id, name: row.name, ip_whitelist: row.ip_whitelist || '', login_methods: row.login_methods_custom ? [...(row.login_methods || [])] : [] };
      dlg.visible = true;
    }
    async function save() {
      dlg.saving = true;
      try {
        if (dlg.isEdit) {
          await api.updatePlatform(dlg.form.id, { name: dlg.form.name, ip_whitelist: dlg.form.ip_whitelist, login_methods: dlg.form.login_methods });
          ElMessage.success('保存成功');
        } else {
          const data = await api.createPlatform({ platform_id: dlg.form.platform_id, name: dlg.form.name, ip_whitelist: dlg.form.ip_whitelist, login_methods: dlg.form.login_methods });
          secretDlg.secret = data.secret;
          secretDlg.visible = true;
        }
        dlg.visible = false;
        load();
      } catch (e) { ElMessage.error(e.message); }
      finally { dlg.saving = false; }
    }
    async function toggleStatus(row) {
      try {
        await api.updatePlatform(row.id, { status: row.status === 1 ? 0 : 1 });
        ElMessage.success('已更新');
        load();
      } catch (e) { ElMessage.error(e.message); }
    }
    async function rotate(row) {
      try {
        await ElMessageBox.confirm(
          row.has_old_secret
            ? '当前处于双盐过渡期，再次轮换将吊销旧盐。确认继续？'
            : '轮换后将进入双盐过渡期（新旧盐同时可验）。确认继续？',
          '密钥轮换', { type: 'warning' });
      } catch { return; }
      try {
        const data = await api.rotateSecret(row.id);
        secretDlg.secret = data.secret;
        secretDlg.visible = true;
        load();
      } catch (e) { ElMessage.error(e.message); }
    }
    async function del(row) {
      try {
        await ElMessageBox.confirm(`确定删除平台「${row.name}」？相关授权将一并清理。`, '提示', { type: 'warning' });
      } catch { return; }
      try {
        await api.deletePlatform(row.id);
        ElMessage.success('已删除');
        load();
      } catch (e) { ElMessage.error(e.message); }
    }
    async function copySecret() {
      try {
        await navigator.clipboard.writeText(secretDlg.secret);
        ElMessage.success('已复制');
      } catch { ElMessage.warning('复制失败，请手动选择复制'); }
    }

    return { platforms, loading, tableHeight, boxRef, dlg, secretDlg, methodOptions, loginMethodLabel, load, openCreate, openEdit, save, toggleStatus, rotate, del, copySecret };
  },
};

// ---------- 授权管理页（用户行 × 平台列勾选矩阵） ----------
const GrantsPage = {
  name: 'GrantsPage',
  template: `
    <el-card class="page-card">
      <template #header>
        <div style="display:flex;justify-content:space-between;align-items:center">
          <div class="card-title">${svg('key')}授权管理</div>
          <div style="display:flex;gap:8px">
            <el-input v-model="filter" placeholder="子序列筛选：用户名/昵称" style="width:220px" clearable>
              <template #prefix>${svg('user', 'ipt-ic')}</template>
            </el-input>
            <el-button :loading="loading" @click="load">${svg('refresh', 'ic-btn')}刷新</el-button>
          </div>
        </div>
      </template>
      <el-alert type="info" :closable="false" show-icon style="margin-bottom:14px;border-radius:8px"
        title="勾选即配置该用户可登录的平台；未授权的平台无法登录，也拉取不到该用户信息。" />
      <div class="table-box" ref="boxRef">
      <el-table :data="filteredUsers" v-loading="loading" stripe :height="tableHeight">
        <el-table-column label="用户" min-width="220" fixed>
          <template #default="{row}">
            <span class="cell-user">
              <span class="avatar" :style="avatarStyle(row)">{{ (row.nickname || row.username).slice(0,1) }}</span>
              <span>
                <b>{{ row.username }}</b>
                <span v-if="row.nickname" style="color:#8a97ab;font-size:12px">（{{ row.nickname }}）</span>
              </span>
            </span>
          </template>
        </el-table-column>
        <el-table-column v-for="p in platforms" :key="p.id" :label="p.name" align="center" min-width="120">
          <template #default="{row}">
            <el-checkbox :model-value="!!row.grants[p.id]" @change="(v) => toggle(row, p, v)" />
          </template>
        </el-table-column>
        <template #empty>
          <div class="table-empty">
            <span class="table-empty-ic">${svg('key')}</span>
            <p>暂无授权数据</p>
          </div>
        </template>
      </el-table>
      </div>
    </el-card>
  `,
  setup() {
    const platforms = ref([]);
    const matrixUsers = ref([]);
    const loading = ref(false);
    const filter = ref('');
    const { tableHeight, boxRef } = useTableFill();

    // 子序列匹配：输入字符按序出现在目标字符串中即可命中（不要求连续，大小写不敏感）
    function isSubsequence(q, s) {
      const query = q.toLowerCase(), str = (s || '').toLowerCase();
      let i = 0;
      for (let j = 0; j < str.length && i < query.length; j++) {
        if (str[j] === query[i]) i++;
      }
      return i === query.length;
    }
    const filteredUsers = computed(() => {
      const q = filter.value.trim();
      if (!q) return matrixUsers.value;
      return matrixUsers.value.filter(u => isSubsequence(q, u.username) || isSubsequence(q, u.nickname));
    });

    const avatarPalette = ['#2f6fed', '#7a5cff', '#0ea5a4', '#e07a3f', '#d64f8c', '#3f9e4d'];
    function avatarStyle(row) {
      let h = 0;
      const s = row.username || '';
      for (let i = 0; i < s.length; i++) h = (h * 31 + s.charCodeAt(i)) >>> 0;
      const c = avatarPalette[h % avatarPalette.length];
      return { width: '26px', height: '26px', 'font-size': '12px', background: 'linear-gradient(135deg, ' + c + ', ' + c + 'cc)' };
    }

    async function load() {
      loading.value = true;
      try {
        const data = await api.grantsMatrix();
        platforms.value = data.platforms || [];
        const grants = (data.grants || []).filter(g => g.status === 1);
        const map = {};
        grants.forEach(g => {
          if (!map[g.user_id]) map[g.user_id] = {};
          map[g.user_id][g.platform_id] = true;
        });
        matrixUsers.value = (data.users || []).map(u => ({ id: u.id, username: u.username, nickname: u.nickname, grants: map[u.id] || {} }));
      } catch (e) { ElMessage.error(e.message); }
      finally { loading.value = false; }
    }
    onMounted(load);

    async function toggle(row, p, val) {
      row.grants[p.id] = !!val;
      const ids = Object.keys(row.grants).filter(k => row.grants[k]).map(Number);
      try {
        await api.setUserGrants(row.id, ids);
        ElMessage.success(`已更新「${row.username}」的授权`);
      } catch (e) {
        row.grants[p.id] = !val; // 回滚
        ElMessage.error(e.message);
      }
    }

    return { platforms, matrixUsers, filteredUsers, filter, loading, tableHeight, boxRef, avatarStyle, load, toggle };
  },
};

// ---------- 审计日志页 ----------
const LogsPage = {
  name: 'LogsPage',
  template: `
    <el-card class="page-card">
      <template #header>
        <div style="display:flex;justify-content:space-between;align-items:center;gap:8px">
          <div class="card-title">${svg('list')}审计日志</div>
          <div style="display:flex;gap:8px;flex-wrap:wrap">
            <el-input v-model="filters.username" placeholder="用户名" style="width:130px" clearable @keyup.enter="load" />
            <el-input v-model="filters.platform_id" placeholder="平台标识" style="width:130px" clearable @keyup.enter="load" />
            <el-select v-model="filters.success" placeholder="结果" style="width:100px" clearable>
              <el-option label="成功" :value="1" />
              <el-option label="失败" :value="0" />
            </el-select>
            <el-button type="primary" @click="load">查询</el-button>
          </div>
        </div>
      </template>
      <div class="table-box" ref="boxRef">
      <el-table :data="logs" v-loading="loading" stripe :height="tableHeight">
        <el-table-column prop="created_at" label="时间" min-width="200" />
        <el-table-column prop="username" label="用户名" min-width="110" />
        <el-table-column prop="platform_id" label="平台标识" min-width="120">
          <template #default="{row}"><span style="font-family:Consolas,Menlo,monospace;font-size:12px">{{ row.platform_id }}</span></template>
        </el-table-column>
        <el-table-column label="结果" min-width="80" align="center">
          <template #default="{row}">
            <el-tag :type="row.success === 1 ? 'success' : 'danger'" effect="light" round>{{ row.success === 1 ? '成功' : '失败' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="原因" min-width="130">
          <template #default="{row}"><span style="color:#5a6b82">{{ reasonText(row.reason) }}</span></template>
        </el-table-column>
        <el-table-column prop="ip" label="IP" min-width="140" />
        <template #empty>
          <div class="table-empty">
            <span class="table-empty-ic">${svg('list')}</span>
            <p>暂无审计日志</p>
          </div>
        </template>
      </el-table>
      </div>
    </el-card>
  `,
  setup() {
    const logs = ref([]);
    const loading = ref(false);
    const { tableHeight, boxRef } = useTableFill();
    const filters = reactive({ username: '', platform_id: '', success: undefined });

    async function load() {
      loading.value = true;
      try {
        const data = await api.listLogs({
          username: filters.username,
          platform_id: filters.platform_id,
          success: filters.success === undefined || filters.success === '' ? '' : filters.success,
        });
        logs.value = data.logs || [];
      } catch (e) { ElMessage.error(e.message); }
      finally { loading.value = false; }
    }
    onMounted(load);

    function reasonText(reason) {
      const map = {
        ok: '登录成功', step_ok: '步骤通过', bad_cred: '账号或密码错误', bad_code: '验证码错误',
        bad_totp: 'TOTP 验证码错误', totp_disabled: '未启用TOTP', disabled: '账号已禁用',
        unauthorized: '未授权登录', locked: '登录被限流锁定', sign_invalid: '签名无效', ip_denied: 'IP不在白名单',
      };
      return map[reason] || reason;
    }

    return { logs, loading, tableHeight, boxRef, filters, load, reasonText };
  },
};

// ---------- 黑名单管理页（自动锁定 + 手动拉黑 + 解除） ----------
const BansPage = {
  name: 'BansPage',
  template: `
    <el-card class="page-card">
      <template #header>
        <div style="display:flex;justify-content:space-between;align-items:center">
          <div class="card-title">${svg('lock')}黑名单管理</div>
          <div style="display:flex;gap:8px">
            <el-input v-model="filter" placeholder="搜索用户名" style="width:180px" clearable @keyup.enter="load" />
            <el-button type="danger" @click="openAdd">${svg('lock', 'ic-btn')}加入黑名单</el-button>
          </div>
        </div>
      </template>
      <div class="table-box" ref="boxRef">
        <el-table :data="filteredBans" v-loading="loading" stripe :height="tableHeight">
          <el-table-column prop="username" label="用户名" min-width="140">
            <template #default="{row}"><b>{{ row.username }}</b></template>
          </el-table-column>
          <el-table-column label="来源" width="110" align="center">
            <template #default="{row}">
              <el-tag :type="row.source === 'auto' ? 'warning' : 'danger'" effect="light" round>
                {{ row.source === 'auto' ? '自动锁定' : '手动拉黑' }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="reason" label="原因" min-width="170" />
          <el-table-column label="操作人" width="110">
            <template #default="{row}"><span style="color:#5a6b82">{{ row.operator || '—' }}</span></template>
          </el-table-column>
          <el-table-column prop="created_at" label="加入时间" width="180" />
          <el-table-column label="到期时间" width="180">
            <template #default="{row}">
              <span v-if="row.expires_at" style="color:#5a6b82">{{ row.expires_at }}</span>
              <el-tag v-else type="danger" effect="plain" round>永久</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="状态" width="90" align="center">
            <template #default="{row}">
              <el-tag :type="row.status === 'active' ? 'danger' : 'info'" effect="light" round>
                {{ row.status === 'active' ? '生效中' : row.status === 'permanent' ? '永久' : '已过期' }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column label="操作" width="90" align="center">
            <template #default="{row}">
              <el-button link type="primary" @click="unban(row)">解除</el-button>
            </template>
          </el-table-column>
          <template #empty>
            <div class="table-empty">
              <span class="table-empty-ic">${svg('lock')}</span>
              <p>暂无黑名单记录</p>
            </div>
          </template>
        </el-table>
      </div>
    </el-card>

    <el-dialog v-model="addDlg.visible" title="加入黑名单" width="440" align-center>
      <el-form label-width="90px">
        <el-form-item label="用户名">
          <el-input v-model="addDlg.username" placeholder="用户名 / 邮箱 / 手机号" />
        </el-form-item>
        <el-form-item label="原因">
          <el-input v-model="addDlg.reason" placeholder="选填" />
        </el-form-item>
        <el-form-item label="到期时间">
          <el-date-picker v-model="addDlg.expiresAt" type="datetime" placeholder="留空 = 永久封禁"
            value-format="YYYY-MM-DDTHH:mm:ss" style="width:100%" :clearable="true" />
          <div class="settings-tip">留空表示永久封禁；填写后到期自动解除（进程内生效）。</div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="addDlg.visible = false">取消</el-button>
        <el-button type="danger" :loading="addDlg.saving" @click="addBan">确定拉黑</el-button>
      </template>
    </el-dialog>
  `,
  setup() {
    const bans = ref([]);
    const filter = ref('');
    const loading = ref(false);
    const { tableHeight, boxRef } = useTableFill();
    const addDlg = reactive({ visible: false, saving: false, username: '', reason: '', expiresAt: '' });

    const filteredBans = computed(() => {
      const q = filter.value.trim().toLowerCase();
      if (!q) return bans.value;
      return bans.value.filter(b => (b.username || '').toLowerCase().includes(q));
    });

    async function load() {
      loading.value = true;
      try {
        const data = await api.listBans();
        bans.value = data.bans || [];
      } catch (e) { ElMessage.error(e.message); }
      finally { loading.value = false; }
    }
    onMounted(load);

    function openAdd() {
      addDlg.username = '';
      addDlg.reason = '';
      addDlg.expiresAt = '';
      addDlg.visible = true;
    }
    async function addBan() {
      if (!addDlg.username.trim()) { ElMessage.warning('请输入用户名'); return; }
      addDlg.saving = true;
      try {
        await api.addBan({ username: addDlg.username.trim(), reason: addDlg.reason, expires_at: addDlg.expiresAt });
        ElMessage.success('已加入黑名单');
        addDlg.visible = false;
        load();
      } catch (e) { ElMessage.error(e.message); }
      finally { addDlg.saving = false; }
    }
    async function unban(row) {
      try {
        await ElMessageBox.confirm(`确定解除「${row.username}」的黑名单/锁定？`, '提示', { type: 'warning' });
      } catch { return; }
      try {
        await api.removeBan(row.username);
        ElMessage.success('已解除');
        load();
      } catch (e) { ElMessage.error(e.message); }
    }

    return { bans, filteredBans, filter, loading, tableHeight, boxRef, addDlg, load, openAdd, addBan, unban };
  },
};

// ---------- 系统设置页（密码安全 / 登录限流 / 登录方式 / 后台IP白名单） ----------
const SettingsPage = {
  name: 'SettingsPage',
  template: `
    <div class="settings-grid">
      <el-card class="settings-card">
        <template #header><div class="card-title">${svg('lock')}密码安全设置</div></template>
        <el-form label-width="130px" label-position="left">
          <el-form-item label="最小长度">
            <el-input-number v-model="form.password_policy.min_length" :min="6" :max="64" />
          </el-form-item>
          <el-form-item label="需包含字母"><el-switch v-model="form.password_policy.require_letter" /></el-form-item>
          <el-form-item label="需包含数字"><el-switch v-model="form.password_policy.require_digit" /></el-form-item>
          <el-form-item label="需包含特殊字符"><el-switch v-model="form.password_policy.require_special" /></el-form-item>
        </el-form>
        <p class="settings-tip">设置后对所有用户生效（新建与修改密码时统一校验）。</p>
        <div class="settings-actions"><el-button type="primary" :loading="savingKey==='password_policy'" @click="save('password_policy')">保存</el-button></div>
      </el-card>

      <el-card class="settings-card">
        <template #header><div class="card-title">${svg('shield')}登录限流设置</div></template>
        <el-form label-width="130px" label-position="left">
          <el-form-item label="最大失败次数">
            <el-input-number v-model="form.login_limit.max_fails" :min="1" :max="100" />
          </el-form-item>
          <el-form-item label="统计窗口(分钟)">
            <el-input-number v-model="form.login_limit.window_minutes" :min="1" :max="1440" />
          </el-form-item>
          <el-form-item label="锁定时间(分钟)">
            <el-input-number v-model="form.login_limit.lock_minutes" :min="1" :max="1440" />
          </el-form-item>
        </el-form>
        <p class="settings-tip">账号维度兜底防爆破：窗口内失败达上限即锁定。保存后立即生效。</p>
        <div class="settings-actions"><el-button type="primary" :loading="savingKey==='login_limit'" @click="save('login_limit')">保存</el-button></div>
      </el-card>

      <el-card class="settings-card">
        <template #header><div class="card-title">${svg('key')}新平台默认登录方式</div></template>
        <el-checkbox-group v-model="loginMethods" style="display:flex;flex-direction:column;gap:10px">
          <el-checkbox v-for="m in methodOptions" :key="m.value" :label="m.value">{{ m.label }}</el-checkbox>
        </el-checkbox-group>
        <p class="settings-tip">
          作为「新注册平台」的默认登录方式（已存在的平台可在平台管理中单独配置）。<br>
          单选：一步直接完成登录；多选：多步骤依次验证（如「用户名+密码」→「TOTP 双因子」）。<br>
          TOTP 双因子不能单独选择（无法独立标识用户）。
        </p>
        <div class="settings-actions"><el-button type="primary" :loading="savingKey==='login_methods'" @click="save('login_methods')">保存</el-button></div>
      </el-card>

      <el-card class="settings-card">
        <template #header><div class="card-title">${svg('grid')}后台登录 IP 白名单</div></template>
        <el-input v-model="ipText" type="textarea" :rows="5" placeholder="每行一个 IP，如：&#10;127.0.0.1&#10;10.0.0.5" />
        <p class="settings-tip">仅管理后台登录生效（管理端部署于内网）。留空 = 不限制。</p>
        <div class="settings-actions"><el-button type="primary" :loading="savingKey==='admin_ip_whitelist'" @click="save('admin_ip_whitelist')">保存</el-button></div>
      </el-card>
    </div>
  `,
  setup() {
    const methodOptions = LOGIN_METHOD_OPTIONS;
    const form = reactive({
      password_policy: { min_length: 8, require_letter: true, require_digit: true, require_special: false },
      login_limit: { max_fails: 5, window_minutes: 15, lock_minutes: 15 },
    });
    const loginMethods = ref([]);
    const ipText = ref('');
    const savingKey = ref('');

    async function load() {
      try {
        const d = await api.listSettings();
        if (d.password_policy) Object.assign(form.password_policy, d.password_policy);
        if (d.login_limit) Object.assign(form.login_limit, d.login_limit);
        loginMethods.value = (d.login_methods && d.login_methods.methods) || ['username_password'];
        ipText.value = ((d.admin_ip_whitelist && d.admin_ip_whitelist.ips) || []).join('\n');
      } catch (e) { ElMessage.error(e.message); }
    }
    onMounted(load);

    async function save(key) {
      savingKey.value = key;
      try {
        if (key === 'password_policy') {
          await api.updateSettings(key, form.password_policy);
        } else if (key === 'login_limit') {
          await api.updateSettings(key, form.login_limit);
        } else if (key === 'login_methods') {
          if (loginMethods.value.length === 0) { ElMessage.warning('至少选择一种登录方式'); return; }
          const order = methodOptions.map(m => m.value);
          await api.updateSettings(key, {
            methods: loginMethods.value.filter(v => order.includes(v)).sort((a, b) => order.indexOf(a) - order.indexOf(b)),
          });
        } else if (key === 'admin_ip_whitelist') {
          const ips = ipText.value.split('\n').map(s => s.trim()).filter(Boolean);
          await api.updateSettings(key, { ips });
        }
        ElMessage.success('保存成功');
      } catch (e) { ElMessage.error(e.message); }
      finally { savingKey.value = ''; }
    }

    return { methodOptions, form, loginMethods, ipText, savingKey, load, save };
  },
};

// ---------- 根组件（登录 + 布局） ----------
const Root = {
  template: `
    <!-- 未登录：登录页 -->
    <div v-if="!user.uid" class="login-wrap">
      <div class="login-grid"></div>
      <div class="login-card">
        <div class="login-brand">
          <span class="login-logo">${Ic.shield}</span>
          <h1 class="login-title">authPlatform</h1>
          <p class="login-sub">统一鉴权中心 · 管理控制台</p>
        </div>
        <el-form @submit.prevent="doLogin" class="login-form">
          <el-form-item>
            <el-input v-model="username" placeholder="请输入用户名" size="large" @keyup.enter="doLogin">
              <template #prefix>${Ic.user}</template>
            </el-input>
          </el-form-item>
          <el-form-item>
            <el-input v-model="password" type="password" show-password placeholder="请输入密码" size="large" @keyup.enter="doLogin">
              <template #prefix>${Ic.lock}</template>
            </el-input>
          </el-form-item>
          <el-button type="primary" class="login-btn" style="width:100%" :loading="loading" @click="doLogin">登 录</el-button>
        </el-form>
        <p class="login-foot">仅管理员账号可登录本控制台</p>
      </div>
    </div>

    <!-- 已登录：布局 + 页面 -->
    <el-container v-else class="layout">
      <el-aside width="220px" class="sidebar">
        <div class="logo">
          <span class="logo-icon">${Ic.shield}</span>
          <span class="logo-text">authPlatform</span>
        </div>
        <el-menu :default-active="route" @select="onMenu" class="side-menu">
          <el-menu-item index="/users">${svg('users', 'm-icon')}<span>用户管理</span></el-menu-item>
          <el-menu-item index="/platforms">${svg('grid', 'm-icon')}<span>平台管理</span></el-menu-item>
          <el-menu-item index="/grants">${svg('key', 'm-icon')}<span>授权管理</span></el-menu-item>
          <el-menu-item index="/logs">${svg('list', 'm-icon')}<span>审计日志</span></el-menu-item>
          <el-menu-item index="/bans">${svg('lock', 'm-icon')}<span>黑名单管理</span></el-menu-item>
          <el-menu-item index="/settings">${svg('grid', 'm-icon')}<span>系统设置</span></el-menu-item>
        </el-menu>
        <div class="side-foot">统一鉴权中心 · v1.0</div>
      </el-aside>
      <el-container>
        <el-header class="hdr">
          <span class="hdr-title">{{ pageTitle }}</span>
          <div class="hdr-right">
            <span class="avatar">{{ (user.nickname || user.username).slice(0,1) }}</span>
            <span class="hdr-user">{{ user.nickname || user.username }}</span>
            <el-button link type="primary" @click="logout">退出登录</el-button>
          </div>
        </el-header>
        <el-main class="main">
          <!-- 标签页快照冻结：keep-alive 缓存各页面实例，首次进入请求接口，切换后保留数据快照不再重新请求 -->
          <keep-alive :include="pageNames">
            <component :is="pageComponent" :key="route" />
          </keep-alive>
        </el-main>
      </el-container>
    </el-container>
  `,
  setup() {
    const user = ref({});
    const username = ref('');
    const password = ref('');
    const loading = ref(false);

    const route = ref(location.hash.replace(/^#/, '') || '/users');
    const pageComponent = computed(() => {
      switch (route.value) {
        case '/platforms': return PlatformsPage;
        case '/grants': return GrantsPage;
        case '/logs': return LogsPage;
        case '/bans': return BansPage;
        case '/settings': return SettingsPage;
        default: return UsersPage;
      }
    });
    const pageNames = ['UsersPage', 'PlatformsPage', 'GrantsPage', 'LogsPage', 'BansPage', 'SettingsPage'];
    const pageTitle = computed(() => {
      switch (route.value) {
        case '/platforms': return '平台管理';
        case '/grants': return '授权管理';
        case '/logs': return '审计日志';
        case '/bans': return '黑名单管理';
        case '/settings': return '系统设置';
        default: return '用户管理';
      }
    });
    function onMenu(idx) { location.hash = idx; route.value = idx; }
    window.addEventListener('hashchange', () => {
      route.value = location.hash.replace(/^#/, '') || '/users';
    });

    async function doLogin() {
      if (!username.value || !password.value) { ElMessage.warning('请输入用户名和密码'); return; }
      loading.value = true;
      try {
        const data = await api.login(username.value, password.value);
        localStorage.setItem('token', data.token);
        user.value = data.user;
        ElMessage.success('登录成功');
      } catch (e) { ElMessage.error(e.message); }
      finally { loading.value = false; }
    }
    function logout() {
      localStorage.removeItem('token');
      user.value = {};
      route.value = '/users';
      location.hash = '';
    }
    onMounted(async () => {
      const token = localStorage.getItem('token');
      if (!token) return;
      try {
        user.value = await api.me();
      }
      catch { localStorage.removeItem('token'); }
    });

    return { user, username, password, loading, route, pageComponent, pageNames, pageTitle, onMenu, doLogin, logout };
  },
};

createApp(Root).use(ElementPlus, { locale: ElementPlusLocaleZhCn }).mount('#app');

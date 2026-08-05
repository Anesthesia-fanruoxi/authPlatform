// authPlatform 管理控制台前端（CDN Vue 3 + Element Plus，无构建步骤）
const { createApp, ref, reactive, computed, onMounted } = Vue;
const { ElMessage, ElMessageBox } = ElementPlus;

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
};

// ---------- 用户管理页 ----------
const UsersPage = {
  template: `
    <el-card>
      <template #header>
        <div style="display:flex;justify-content:space-between;align-items:center">
          <b>用户管理</b>
          <div>
            <el-input v-model="keyword" placeholder="搜索用户名/昵称" style="width:200px;margin-right:8px" clearable @keyup.enter="load" />
            <el-button type="primary" @click="openCreate">新建用户</el-button>
          </div>
        </div>
      </template>
      <el-table :data="users" v-loading="loading" border stripe>
        <el-table-column prop="uid" label="UID" width="230" />
        <el-table-column prop="username" label="用户名" />
        <el-table-column prop="nickname" label="昵称" />
        <el-table-column label="状态" width="90">
          <template #default="{row}">
            <el-tag :type="row.status === 1 ? 'success' : 'danger'">{{ row.status === 1 ? '启用' : '禁用' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="角色" width="90">
          <template #default="{row}">
            <el-tag v-if="row.is_admin" type="warning">管理员</el-tag>
            <span v-else>普通</span>
          </template>
        </el-table-column>
        <el-table-column prop="created_at" label="创建时间" width="190" />
        <el-table-column label="操作" width="300">
          <template #default="{row}">
            <el-button link type="primary" @click="openEdit(row)">编辑</el-button>
            <el-button link :type="row.status === 1 ? 'danger' : 'success'" @click="toggleStatus(row)">
              {{ row.status === 1 ? '禁用' : '启用' }}
            </el-button>
            <el-button link type="warning" @click="openReset(row)">重置密码</el-button>
            <el-button link type="danger" @click="del(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-dialog v-model="dlg.visible" :title="dlg.isEdit ? '编辑用户' : '新建用户'" width="420">
      <el-form label-width="80px">
        <el-form-item label="用户名"><el-input v-model="dlg.form.username" :disabled="dlg.isEdit" /></el-form-item>
        <el-form-item label="昵称"><el-input v-model="dlg.form.nickname" /></el-form-item>
        <el-form-item v-if="!dlg.isEdit" label="密码">
          <el-input v-model="dlg.form.password" type="password" show-password placeholder="至少8位，含字母和数字" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dlg.visible = false">取消</el-button>
        <el-button type="primary" :loading="dlg.saving" @click="save">保存</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="pwdDlg.visible" title="重置密码" width="400">
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
    const dlg = reactive({ visible: false, isEdit: false, saving: false, form: { id: 0, username: '', nickname: '', password: '' } });
    const pwdDlg = reactive({ visible: false, saving: false, id: 0, username: '', password: '' });

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
      dlg.form = { id: 0, username: '', nickname: '', password: '' };
      dlg.visible = true;
    }
    function openEdit(row) {
      dlg.isEdit = true;
      dlg.form = { id: row.id, username: row.username, nickname: row.nickname, password: '' };
      dlg.visible = true;
    }
    async function save() {
      dlg.saving = true;
      try {
        if (dlg.isEdit) {
          await api.updateUser(dlg.form.id, { nickname: dlg.form.nickname });
        } else {
          await api.createUser({ username: dlg.form.username, password: dlg.form.password, nickname: dlg.form.nickname });
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

    return { users, keyword, loading, dlg, pwdDlg, load, openCreate, openEdit, save, toggleStatus, openReset, savePwd, del };
  },
};

// ---------- 占位页（M3/M4/M5 实现） ----------
function PlaceholderPage(title, desc) {
  return {
    template: `<el-card><el-empty description="${desc}" /></el-card>`,
    setup() { return {}; },
  };
}
// ---------- 平台管理页 ----------
const PlatformsPage = {
  template: `
    <el-card>
      <template #header>
        <div style="display:flex;justify-content:space-between;align-items:center">
          <b>平台管理</b>
          <el-button type="primary" @click="openCreate">注册平台</el-button>
        </div>
      </template>
      <el-table :data="platforms" v-loading="loading" border stripe>
        <el-table-column prop="platform_id" label="平台标识" width="160" />
        <el-table-column prop="name" label="名称" />
        <el-table-column label="加密盐" width="150">
          <template #default="{row}">
            <span style="font-family:monospace">{{ row.secret_masked || '***' }}</span>
            <el-tag v-if="row.has_old_secret" type="warning" size="small" style="margin-left:6px">过渡期</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="90">
          <template #default="{row}">
            <el-tag :type="row.status === 1 ? 'success' : 'danger'">{{ row.status === 1 ? '启用' : '停用' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="created_at" label="创建时间" width="190" />
        <el-table-column label="操作" width="300">
          <template #default="{row}">
            <el-button link type="primary" @click="openEdit(row)">编辑</el-button>
            <el-button link type="warning" @click="rotate(row)">轮换密钥</el-button>
            <el-button link :type="row.status === 1 ? 'danger' : 'success'" @click="toggleStatus(row)">
              {{ row.status === 1 ? '停用' : '启用' }}
            </el-button>
            <el-button link type="danger" @click="del(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-dialog v-model="dlg.visible" :title="dlg.isEdit ? '编辑平台' : '注册平台'" width="460">
      <el-form label-width="100px">
        <el-form-item label="平台标识">
          <el-input v-model="dlg.form.platform_id" :disabled="dlg.isEdit" placeholder="如 ops-platform" />
        </el-form-item>
        <el-form-item label="名称"><el-input v-model="dlg.form.name" /></el-form-item>
        <el-form-item label="IP 白名单">
          <el-input v-model="dlg.form.ip_whitelist" placeholder='JSON 数组，如 ["1.2.3.4"]，留空不限制' />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dlg.visible = false">取消</el-button>
        <el-button type="primary" :loading="dlg.saving" @click="save">保存</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="secretDlg.visible" title="平台加密盐（仅此一次展示，请立即保存）" width="560">
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
    const dlg = reactive({ visible: false, isEdit: false, saving: false, form: { id: 0, platform_id: '', name: '', ip_whitelist: '' } });
    const secretDlg = reactive({ visible: false, secret: '' });

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
      dlg.form = { id: 0, platform_id: '', name: '', ip_whitelist: '' };
      dlg.visible = true;
    }
    function openEdit(row) {
      dlg.isEdit = true;
      dlg.form = { id: row.id, platform_id: row.platform_id, name: row.name, ip_whitelist: row.ip_whitelist || '' };
      dlg.visible = true;
    }
    async function save() {
      dlg.saving = true;
      try {
        if (dlg.isEdit) {
          await api.updatePlatform(dlg.form.id, { name: dlg.form.name, ip_whitelist: dlg.form.ip_whitelist });
          ElMessage.success('保存成功');
        } else {
          const data = await api.createPlatform({ platform_id: dlg.form.platform_id, name: dlg.form.name, ip_whitelist: dlg.form.ip_whitelist });
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

    return { platforms, loading, dlg, secretDlg, load, openCreate, openEdit, save, toggleStatus, rotate, del, copySecret };
  },
};
// ---------- 授权管理页（用户行 × 平台列勾选矩阵） ----------
const GrantsPage = {
  template: `
    <el-card>
      <template #header>
        <div style="display:flex;justify-content:space-between;align-items:center">
          <b>授权管理</b>
          <el-button :loading="loading" @click="load">刷新</el-button>
        </div>
      </template>
      <el-alert type="info" :closable="false" show-icon style="margin-bottom:12px"
        title="勾选即配置该用户可登录的平台；未授权的平台无法登录，也拉取不到该用户信息。" />
      <el-table :data="matrixUsers" v-loading="loading" border stripe>
        <el-table-column label="用户" width="200" fixed>
          <template #default="{row}">
            {{ row.username }}<span v-if="row.nickname">（{{ row.nickname }}）</span>
          </template>
        </el-table-column>
        <el-table-column v-for="p in platforms" :key="p.id" :label="p.name" align="center" width="120">
          <template #default="{row}">
            <el-checkbox :model-value="!!row.grants[p.id]" @change="(v) => toggle(row, p, v)" />
          </template>
        </el-table-column>
      </el-table>
    </el-card>
  `,
  setup() {
    const platforms = ref([]);
    const matrixUsers = ref([]);
    const loading = ref(false);

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

    return { platforms, matrixUsers, loading, load, toggle };
  },
};
// ---------- 审计日志页 ----------
const LogsPage = {
  template: `
    <el-card>
      <template #header>
        <div style="display:flex;justify-content:space-between;align-items:center;gap:8px">
          <b>审计日志</b>
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
      <el-table :data="logs" v-loading="loading" border stripe>
        <el-table-column prop="created_at" label="时间" width="200" />
        <el-table-column prop="username" label="用户名" width="130" />
        <el-table-column prop="platform_id" label="平台标识" width="140" />
        <el-table-column label="结果" width="90">
          <template #default="{row}">
            <el-tag :type="row.success === 1 ? 'success' : 'danger'">{{ row.success === 1 ? '成功' : '失败' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="原因" width="140">
          <template #default="{row}">{{ reasonText(row.reason) }}</template>
        </el-table-column>
        <el-table-column prop="ip" label="IP" width="150" />
      </el-table>
    </el-card>
  `,
  setup() {
    const logs = ref([]);
    const loading = ref(false);
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
        ok: '登录成功', bad_cred: '账号或密码错误', disabled: '账号已禁用',
        unauthorized: '未授权登录', locked: '登录被限流锁定', sign_invalid: '签名无效',
      };
      return map[reason] || reason;
    }

    return { logs, loading, filters, load, reasonText };
  },
};

// ---------- 根组件（登录 + 布局） ----------
const Root = {
  template: `
    <!-- 未登录：登录页 -->
    <div v-if="!user.uid" class="login-wrap">
      <el-card class="login-card">
        <template #header><b>authPlatform 管理控制台</b></template>
        <el-form @submit.prevent="doLogin">
          <el-form-item label="用户名"><el-input v-model="username" placeholder="请输入用户名" @keyup.enter="doLogin" /></el-form-item>
          <el-form-item label="密码">
            <el-input v-model="password" type="password" show-password placeholder="请输入密码" @keyup.enter="doLogin" />
          </el-form-item>
          <el-button type="primary" style="width:100%" :loading="loading" @click="doLogin">登 录</el-button>
        </el-form>
      </el-card>
    </div>

    <!-- 已登录：布局 + 页面 -->
    <el-container v-else class="layout">
      <el-aside width="200px">
        <div class="logo">authPlatform</div>
        <el-menu :default-active="route" @select="onMenu">
          <el-menu-item index="/users">用户管理</el-menu-item>
          <el-menu-item index="/platforms">平台管理</el-menu-item>
          <el-menu-item index="/grants">授权管理</el-menu-item>
          <el-menu-item index="/logs">审计日志</el-menu-item>
        </el-menu>
      </el-aside>
      <el-container>
        <el-header class="hdr">
          <span style="margin-right:12px">{{ user.nickname || user.username }}</span>
          <el-button link type="primary" @click="logout">退出</el-button>
        </el-header>
        <el-main><component :is="pageComponent" /></el-main>
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
        default: return UsersPage;
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
      try { user.value = await api.me(); }
      catch { localStorage.removeItem('token'); }
    });

    return { user, username, password, loading, route, pageComponent, onMenu, doLogin, logout };
  },
};

createApp(Root).use(ElementPlus, { locale: ElementPlusLocaleZhCn }).mount('#app');

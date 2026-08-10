// authPlatform 桌面令牌登录前端（Tauri webview，无构建）
const { createApp, ref } = Vue;
const invoke = window.__TAURI__.core.invoke;
const listen = window.__TAURI__.event.listen;

let currentMethod = 'username_password'; // username_password | username_totp

function setMsg(sel, text, isErr) {
  const el = document.querySelector(sel);
  el.textContent = text;
  el.className = 'status' + (isErr ? ' error' : '');
}

function showLogin() { document.getElementById('view-login').classList.remove('hide'); }
function hideLogin() { document.getElementById('view-login').classList.add('hide'); }
function showMain() { document.getElementById('view-main').classList.remove('hide'); }
function hideMain() { document.getElementById('view-main').classList.add('hide'); }

async function refreshIdentity() {
  try {
    const id = await invoke('identity');
    if (id.logged_in) {
      hideLogin();
      showMain();
      document.getElementById('avatar').textContent = (id.user.nickname || id.user.username).slice(0, 1).toUpperCase();
      document.getElementById('user-name').textContent = id.user.nickname || id.user.username;
      document.getElementById('user-detail').textContent = id.user.username + '（' + id.user.uid + '）';
      refreshPending();
    } else {
      showLogin();
      hideMain();
    }
  } catch (e) { console.error(e); }
}

async function refreshPending() {
  try {
    const p = await invoke('pending');
    const box = document.getElementById('confirm-box');
    if (p) {
      document.getElementById('confirm-text').textContent =
        `平台「${p.platform || '未知平台'}」请求以当前账号身份登录，是否确认？`;
      box.classList.remove('hide');
      setMsg('#main-msg', '');
    } else {
      box.classList.add('hide');
    }
  } catch (e) { console.error(e); }
}

async function doLogin() {
  const baseUrl = document.getElementById('base-url').value.trim();
  const username = document.getElementById('username').value.trim();
  const credential = document.getElementById('credential').value.trim();
  if (!baseUrl || !username || !credential) { setMsg('#login-msg', '请填写完整信息', true); return; }
  setMsg('#login-msg', '登录中…');
  try {
    const user = await invoke('login', {
      baseUrl, method: currentMethod, identifier: username, credential,
    });
    setMsg('#login-msg', '登录成功：' + (user.nickname || user.username));
    refreshIdentity();
  } catch (e) {
    setMsg('#login-msg', e, true);
  }
}

async function doConfirm() {
  setMsg('#main-msg', '确认中…');
  try {
    await invoke('confirm_pending');
    setMsg('#main-msg', '已确认，平台可继续登录');
    refreshPending();
  } catch (e) {
    setMsg('#main-msg', e, true);
  }
}

async function doReject() {
  await invoke('reject_pending');
  setMsg('#main-msg', '已拒绝');
  refreshPending();
}

async function doLogout() {
  await invoke('logout');
  document.getElementById('credential').value = '';
  refreshIdentity();
}

document.addEventListener('DOMContentLoaded', () => {
  document.getElementById('btn-login').addEventListener('click', doLogin);
  document.getElementById('btn-confirm').addEventListener('click', doConfirm);
  document.getElementById('btn-reject').addEventListener('click', doReject);
  document.getElementById('btn-logout').addEventListener('click', doLogout);
  document.getElementById('method-switch').addEventListener('click', () => {
    currentMethod = currentMethod === 'username_password' ? 'username_totp' : 'username_password';
    document.getElementById('cred-label').textContent =
      currentMethod === 'username_password' ? '密码' : 'TOTP 验证码';
    document.getElementById('credential').placeholder =
      currentMethod === 'username_password' ? '请输入密码' : '6 位动态码';
    document.getElementById('method-switch').textContent =
      currentMethod === 'username_password' ? '切换为 TOTP 验证码' : '切换为密码';
  });
  // 监听平台推送
  listen('desktop-pending', () => refreshPending());
  refreshIdentity();
});

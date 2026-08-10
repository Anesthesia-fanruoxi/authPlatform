#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
平台侧「桌面令牌登录」接入示例（模拟平台登录页后端流程）
======================================================
流程：用户点击「桌面令牌登录」→ 本脚本：
  1. initiate      向认证中心发起免密登录，拿到一次性 request_id（60s）
  2. 探测本地桌面客户端(127.0.0.1:5712)，推送 request_id，客户端弹确认窗
  3. poll          轮询用户是否在桌面客户端确认（60s 内）
  4. exchange      确认后用 request_id 兑换平台 token（一次性）

依赖：pip install requests
配套：认证中心需已启动；桌面客户端需已启动并登录（desktop/，Tauri）。
"""
import hashlib
import hmac
import json
import sys
import time

import requests

# ============ 平台接入配置（在认证中心「平台管理」注册获得） ============
BASE_URL = "http://127.0.0.1:8080"
PLATFORM_ID = "ops-platforms"            # 认证中心注册的平台标识
SECRET = "e2edb44d604be4ce6a69b4b21cf856cc01d2051fef48d0717913b3d84e60a0e5"  # 平台独立盐
CLIENT_PORT = 5712                       # 桌面客户端本地端口
# ====================================================================


def signed_request(path, payload=None):
    """平台签名请求（与接入文档 §2 协议一致）"""
    method = "POST" if payload is not None else "GET"
    body = json.dumps(payload).encode("utf-8") if payload is not None else b""
    ts = str(int(time.time()))
    msg = f"{method}|{path}|{ts}|{hashlib.sha256(body).hexdigest()}"
    sign = hmac.new(SECRET.encode(), msg.encode(), hashlib.sha256).hexdigest()
    headers = {
        "X-Platform-Id": PLATFORM_ID,
        "X-Timestamp": ts,
        "X-Sign": sign,
        "Content-Type": "application/json",
    }
    url = BASE_URL + path
    if method == "POST":
        resp = requests.post(url, data=body, headers=headers, timeout=5)
    else:
        resp = requests.get(url, headers=headers, timeout=5)
    resp.raise_for_status()
    return resp.json()


def desktop_token_login():
    """1. 发起免密登录，拿到一次性 request_id"""
    r = signed_request("/api/auth/desktop/initiate", {})  # POST（空 body）
    if r["code"] != 0:
        print("发起失败:", r.get("msg"))
        return None
    data = r["data"]
    print(f"[1] 发起免密登录 request_id={data['request_id']}（{data['expires_in']}s 有效）")
    return data["request_id"]


def push_to_client(request_id):
    """2. 探测本地桌面客户端并推送确认请求"""
    try:
        identity = requests.get(f"http://127.0.0.1:{CLIENT_PORT}/identity", timeout=2).json()
    except Exception:
        print("[2] 未检测到桌面客户端（请确认客户端已启动）")
        return None
    if not identity.get("logged_in"):
        print("[2] 桌面客户端未登录，请先打开客户端登录")
        return None
    user = identity["user"]
    try:
        resp = requests.post(
            f"http://127.0.0.1:{CLIENT_PORT}/pending",
            json={"request_id": request_id, "platform": PLATFORM_ID},
            timeout=2,
        )
        resp.raise_for_status()
    except Exception as e:
        print("[2] 推送失败:", e)
        return None
    print(f"[2] 已推送确认请求到桌面客户端（用户：{user.get('nickname') or user.get('username')}）")
    return user


def wait_confirmed(request_id, timeout=60):
    """3. 轮询用户是否确认（60s 内，5s 一次）"""
    deadline = time.time() + timeout
    while time.time() < deadline:
        r = signed_request(f"/api/auth/desktop/poll?request_id={request_id}")
        if r["code"] != 0:
            print("[3] 轮询失败:", r.get("msg"))
            return False
        status = r["data"]["status"]
        if status == "confirmed":
            print("[3] 用户在桌面客户端确认了登录请求")
            return True
        if status in ("expired", "used"):
            print("[3] 请求已过期或已使用")
            return False
        time.sleep(5)
    print("[3] 等待用户确认超时")
    return False


def exchange_token(request_id):
    """4. 用确认后的 request_id 兑换平台 token（一次性）"""
    r = signed_request("/api/auth/desktop/exchange", {"request_id": request_id})
    if r["code"] == 0:
        data = r["data"]
        print(f"[4] 登录成功 token={data['token'][:12]}… 用户={data['user']['username']}")
        return data
    print("[4] 兑换失败:", r.get("msg"))
    return None


def main():
    request_id = desktop_token_login()
    if not request_id:
        return
    push_to_client(request_id)  # 探测失败不影响后续（真实接入时前端在此提示引导）
    if not wait_confirmed(request_id):
        return
    exchange_token(request_id)


if __name__ == "__main__":
    sys.exit(main())

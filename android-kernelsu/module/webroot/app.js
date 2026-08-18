import { exec, toast } from "./kernelsu.js";

const CONTROL = "/data/adb/modules/fastcopy_kernelsu/bin/fastcopyctl";
const DATA_DIR = "/data/adb/fastcopy";

const elements = {
  form: document.querySelector("#settings"),
  enabled: document.querySelector("#enabled"),
  serverURL: document.querySelector("#server-url"),
  accountSection: document.querySelector("#account-section"),
  sessionSection: document.querySelector("#session-section"),
  account: document.querySelector("#account"),
  password: document.querySelector("#password"),
  save: document.querySelector("#save"),
  logout: document.querySelector("#logout"),
  refresh: document.querySelector("#refresh"),
  statusDot: document.querySelector("#status-dot"),
  statusTitle: document.querySelector("#status-title"),
  statusMessage: document.querySelector("#status-message"),
  devicesSection: document.querySelector("#devices-section"),
  devicesList: document.querySelector("#devices-list"),
  deviceCount: document.querySelector("#device-count"),
  logs: document.querySelector("#logs"),
};

let currentConfig = {
  enabled: false,
  server_url: "https://zhy.hair/fastcopy",
  account: "",
  password: "",
};

let authenticated = false;
let statusLoading = false;
let deviceRefreshInFlight = false;
let latestStatus = null;

function encodeBase64(value) {
  const bytes = new TextEncoder().encode(value);
  let binary = "";
  for (const byte of bytes) binary += String.fromCharCode(byte);
  return btoa(binary);
}

function decodeBase64(value) {
  const binary = atob(value.trim());
  const bytes = Uint8Array.from(binary, (character) => character.charCodeAt(0));
  return new TextDecoder().decode(bytes);
}

async function loadConfig() {
  const result = await exec(`${CONTROL} config-get`);
  if (result.errno === 0 && result.stdout.trim()) {
    try {
      currentConfig = { ...currentConfig, ...JSON.parse(decodeBase64(result.stdout)) };
    } catch (error) {
      toast("配置文件无法解析");
    }
  }
  elements.enabled.checked = Boolean(currentConfig.enabled);
  elements.serverURL.value = currentConfig.server_url || "https://zhy.hair/fastcopy";
  elements.account.value = currentConfig.account || currentConfig.email || "";
  elements.password.value = currentConfig.password || "";
}

async function loadStatus() {
  if (statusLoading) return null;
  statusLoading = true;
  try {
    const [statusResult, logResult] = await Promise.all([
      exec(`cat ${DATA_DIR}/status.json 2>/dev/null`),
      exec(`${CONTROL} logs 60`),
    ]);
    elements.logs.textContent = logResult.stdout.trim() || "暂无日志";
    if (statusResult.errno !== 0 || !statusResult.stdout.trim()) {
      latestStatus = null;
      renderSession(false, {});
      setStatus("stopped", "守护进程未运行", "");
      return null;
    }
    const status = JSON.parse(statusResult.stdout);
    latestStatus = status;
    const labels = {
      ready: "同步就绪",
      connecting: "正在连接",
      offline: "等待网络恢复",
      disabled: "同步已关闭",
      unconfigured: "等待配置",
      auth_required: "需要重新登录",
      key_error: "加密状态异常",
      waiting_unlock: "等待设备解锁",
      stopped: "已停止",
      error: "运行错误",
    };
    const detail = [status.message, status.pending ? `待上传 ${status.pending}` : ""]
      .filter(Boolean)
      .join(" · ");
    setStatus(status.state, labels[status.state] || status.state, detail);
    const signedIn = typeof status.authenticated === "boolean"
      ? status.authenticated
      : Boolean(status.device_id && !["auth_required", "unconfigured"].includes(status.state));
    renderSession(signedIn, status);
    return status;
  } catch (error) {
    latestStatus = null;
    renderSession(false, {});
    setStatus("error", "状态数据无效", "");
    return null;
  } finally {
    statusLoading = false;
  }
}

function renderSession(signedIn, status) {
  authenticated = signedIn;
  elements.accountSection.hidden = signedIn;
  elements.sessionSection.hidden = !signedIn;
  elements.account.disabled = signedIn;
  elements.password.disabled = signedIn;
  elements.devicesSection.hidden = !signedIn;
  elements.save.textContent = signedIn ? "保存同步设置" : "登录或注册";
  if (signedIn) renderDevices(status);
}

function renderDevices(status) {
  elements.devicesList.replaceChildren();
  if (status.devices_refreshing) {
    renderDevicesMessage("正在刷新在线设备");
    elements.deviceCount.textContent = "";
    return;
  }
  if (status.devices_error && !status.devices_loaded) {
    renderDevicesMessage("在线设备刷新失败");
    elements.deviceCount.textContent = "";
    return;
  }
  if (!status.devices_loaded) {
    const message = status.state === "disabled"
      ? "启用后台同步后获取在线设备"
      : "正在获取在线设备";
    renderDevicesMessage(message);
    elements.deviceCount.textContent = "";
    return;
  }

  const devices = Array.isArray(status.online_devices)
    ? status.online_devices.filter((device) => device.online !== false)
    : [];
  elements.deviceCount.textContent = `${devices.length} 台`;
  if (devices.length === 0) {
    renderDevicesMessage("暂无在线设备");
    return;
  }

  for (const device of devices) {
    const row = document.createElement("div");
    row.className = "device-row";
    row.setAttribute("role", "listitem");

    const content = document.createElement("div");
    content.className = "device-content";
    const title = document.createElement("div");
    title.className = "device-title";
    const name = document.createElement("strong");
    name.textContent = device.display_name || "未命名设备";
    title.append(name);
    if (device.current) {
      const current = document.createElement("span");
      current.className = "current-device";
      current.textContent = "当前设备";
      title.append(current);
    }

    const meta = document.createElement("p");
    meta.className = "device-meta";
    meta.textContent = deviceDescription(device);
    content.append(title, meta);

    const online = document.createElement("span");
    online.className = "online-label";
    online.textContent = "在线";
    row.append(content, online);
    elements.devicesList.append(row);
  }
}

async function refreshDevices() {
  if (!authenticated || deviceRefreshInFlight || latestStatus?.state === "disabled") return;
  deviceRefreshInFlight = true;
  const previousUpdate = latestStatus?.devices_updated_at || "";
  renderDevices({ ...latestStatus, devices_refreshing: true });
  try {
    const result = await exec(`${CONTROL} refresh-devices`);
    if (result.errno !== 0) {
      toast("无法刷新在线设备");
      await loadStatus();
      return;
    }
    for (let attempt = 0; attempt < 12; attempt += 1) {
      await new Promise((resolve) => setTimeout(resolve, 500));
      const status = await loadStatus();
      if (!status) continue;
      const completed = !status.devices_refreshing
        && (status.devices_updated_at !== previousUpdate || Boolean(status.devices_error));
      if (completed) return;
    }
    toast("在线设备刷新超时");
  } finally {
    deviceRefreshInFlight = false;
  }
}

async function refreshPage() {
  await loadStatus();
  await refreshDevices();
}

function renderDevicesMessage(message) {
  const empty = document.createElement("p");
  empty.className = "devices-empty";
  empty.textContent = message;
  elements.devicesList.append(empty);
}

function deviceDescription(device) {
  const platformNames = {
    android: "Android",
    macos: "macOS",
    ios: "iOS",
    windows: "Windows",
    linux: "Linux",
  };
  return [platformNames[device.platform] || device.platform, device.os_version]
    .filter(Boolean)
    .join(" · ");
}

function setStatus(state, title, message) {
  elements.statusDot.className = "status-dot";
  if (state === "ready") elements.statusDot.classList.add("ready");
  if (["error", "auth_required", "key_error"].includes(state)) {
    elements.statusDot.classList.add("error");
  }
  elements.statusTitle.textContent = title;
  elements.statusMessage.textContent = message;
}

async function save(event) {
  event.preventDefault();
  elements.save.disabled = true;
  const wasAuthenticated = authenticated;
  const next = {
    enabled: elements.enabled.checked,
    server_url: elements.serverURL.value.trim().replace(/\/+$/, ""),
    account: wasAuthenticated ? currentConfig.account : elements.account.value.trim(),
    password: wasAuthenticated ? "" : elements.password.value,
  };
  const encoded = encodeBase64(JSON.stringify(next));
  const result = await exec(`${CONTROL} config-set '${encoded}'`);
  if (result.errno !== 0) {
    toast("保存失败");
    elements.save.disabled = false;
    return;
  }
  currentConfig = next;
  await exec(`${CONTROL} restart`);
  toast(wasAuthenticated ? "设置已保存" : "正在登录");
  for (let attempt = 0; attempt < 12; attempt += 1) {
    await new Promise((resolve) => setTimeout(resolve, attempt === 0 ? 1200 : 1000));
    await loadConfig();
    const status = await loadStatus();
    if (!status) continue;
    if (status.authenticated) {
      await refreshDevices();
      break;
    }
    if (wasAuthenticated || ["auth_required", "unconfigured", "error"].includes(status.state)) {
      break;
    }
  }
  elements.save.disabled = false;
}

async function signOut() {
  if (!authenticated || !window.confirm("确定退出当前账号？")) return;
  elements.logout.disabled = true;
  try {
    const result = await exec(`${CONTROL} logout`);
    if (result.errno !== 0) {
      toast("退出登录失败");
      return;
    }
    authenticated = false;
    latestStatus = null;
    elements.password.value = "";
    renderSession(false, {});
    await loadConfig();
    await loadStatus();
    toast("已退出登录");
  } finally {
    elements.logout.disabled = false;
  }
}

elements.form.addEventListener("submit", save);
elements.refresh.addEventListener("click", refreshPage);
elements.logout.addEventListener("click", signOut);

await loadConfig();
await loadStatus();
await refreshDevices();
setInterval(loadStatus, 10000);

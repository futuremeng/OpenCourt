<script setup lang="ts">
// App 外壳（v0.4 P1）：顶栏 / 案件切换 / 更新横幅 / 拖放遮罩 / 设置面板。
// 工作台与引导动线在 views/；菜单与快捷键经 Workspace 暴露的方法转发，
// 外壳不再持有各功能面板的 ref。
import { computed, nextTick, onMounted, onUnmounted, ref } from "vue";
import * as App from "../wailsjs/go/main/App";
import { EventsOn } from "../wailsjs/runtime/runtime";
import { errText, humanSize, profileLabel } from "./util";
import CaseBar from "./components/CaseBar.vue";
import SettingsPanel from "./components/SettingsPanel.vue";
import Workspace from "./views/Workspace.vue";

const info = ref<Record<string, any> | null>(null);
const error = ref("");

// 引导动线是 Profile 数据：court 声明四步，其余 Profile 为空数组（自由动线）。
const guide = computed<any[]>(() =>
  Array.isArray(info.value?.guide) ? info.value!.guide : []
);

// --- 显示设置（已批准画布 case-journey 的「实现待办」，本版兑现）---
// 字号三档缩放根字号令牌；深浅色默认跟随系统，手动选择不改其余界面。
// 选择存 localStorage，启动即应用；弹层键盘可达（aria-expanded/Escape/关闭回焦）。
const displayOpen = ref(false);
const displaySize = ref<"compact" | "normal" | "large">("normal");
const themeMode = ref<"system" | "light" | "dark">("system");
const displayBtn = ref<HTMLElement | null>(null);
const displayPop = ref<HTMLElement | null>(null);

function applyDisplay() {
  const el = document.documentElement;
  el.classList.toggle("fs-compact", displaySize.value === "compact");
  el.classList.toggle("fs-large", displaySize.value === "large");
  if (themeMode.value === "system") el.removeAttribute("data-theme");
  else el.setAttribute("data-theme", themeMode.value);
}
function saveDisplay() {
  localStorage.setItem("opencourt-display", JSON.stringify({ size: displaySize.value, theme: themeMode.value }));
  applyDisplay();
}
function loadDisplay() {
  try {
    const saved = JSON.parse(localStorage.getItem("opencourt-display") || "");
    if (saved && typeof saved === "object") {
      if (["compact", "normal", "large"].includes(saved.size)) displaySize.value = saved.size;
      if (["system", "light", "dark"].includes(saved.theme)) themeMode.value = saved.theme;
    }
  } catch {
    /* 存储损坏视同未设置，按默认跟随系统 */
  }
  applyDisplay();
}
function setDisplaySize(v: "compact" | "normal" | "large") { displaySize.value = v; saveDisplay(); }
function setTheme(v: "system" | "light" | "dark") { themeMode.value = v; saveDisplay(); }
function toggleDisplay() {
  displayOpen.value = !displayOpen.value;
  if (displayOpen.value) nextTick(() => displayPop.value?.focus());
}
function closeDisplay() {
  if (!displayOpen.value) return;
  displayOpen.value = false;
  displayBtn.value?.focus();
}
function onDisplayDocDown(e: MouseEvent) {
  if (!displayOpen.value) return;
  const t = e.target as Node;
  if (displayPop.value?.contains(t) || displayBtn.value?.contains(t)) return;
  closeDisplay();
}
loadDisplay();

// 顶栏只留对办案有后果的状态（v0.4 P3，docs/design-workflow.md）：
// 已启用不再用告警黄；使用统计（隐私相关）开启才显 warn；脱敏级别迁入工具区。
const roleLabel = computed(() => profileLabel(info.value?.profile));

const caseBarRef = ref(null);
const workspaceRef = ref(null);

// --- 桌面集成：设置面板（Cmd+,）与全局快捷键（与菜单栏同一落点） ---
const settingsOpen = ref(false);
const settingsMode = ref<"settings" | "about">("settings");

// 拖放遮罩：文件拖入窗口时提示落点（macOS 原生 drop 由 Go 侧 OnFileDrop
// 接管，这里只做视觉反馈，best-effort：WebView 不派发 DOM 拖拽事件时不显示）
const dragging = ref(false);
let dragDepth = 0;
function onDragEnter(e: DragEvent) {
  if (e.dataTransfer?.types.includes("Files")) {
    dragDepth++;
    dragging.value = true;
  }
}
function onDragLeave() {
  if (--dragDepth <= 0) {
    dragDepth = 0;
    dragging.value = false;
  }
}
function onDragEnd() {
  dragDepth = 0;
  dragging.value = false;
}

function onGlobalKey(e: KeyboardEvent) {
  if (e.key === "Escape") {
    if (settingsOpen.value) {
      settingsOpen.value = false;
      return;
    }
    closeDisplay();
    return;
  }
  // 与菜单栏同快捷键的浏览器兜底（wails dev 纯浏览器调试时菜单不可用）
  if (!(e.metaKey || e.ctrlKey) || e.key.toLowerCase() !== "k") return;
  e.preventDefault();
  if (e.shiftKey) (workspaceRef.value as any)?.focusCaseSearch?.();
  else (workspaceRef.value as any)?.focusLawSearch?.();
}

// 案件切换的唯一入口：一切按案件隔离的数据由 Workspace 统一刷新
async function onCaseSwitched() {
  try {
    await (workspaceRef.value as any)?.onCaseSwitched?.();
  } catch (e) {
    error.value = errText(e);
  }
}

// 左栏案件列表里切换案件：CaseBar 自带错误提示，刷新后走同一条统一刷新链路
// （与 CaseBar 自身切换同一落点，不开第二套刷新路径）
async function onCaseChanged() {
  await (caseBarRef.value as any)?.refresh?.();
  await onCaseSwitched();
}

// 向导「新建案件」落点：聚焦 CaseBar 的新案件输入
function focusNewCase() {
  (caseBarRef.value as any)?.focusNewCase?.();
}

// --- 程序自身更新（横幅 + 一键下载安装重启；高危动作用页内两步确认，不用原生 confirm） ---
const appUpdate = ref<any | null>(null);
const appUpdating = ref(false);
const appUpdateProg = ref<string[]>([]);
const showUpdateNotes = ref(false);
const updateConfirm = ref(false);

async function checkAppUpdate() {
  try {
    const r = await App.CheckAppUpdate();
    if (r.ok && r.available) appUpdate.value = r;
  } catch {
    /* 离线等情况静默跳过 */
  }
}

async function doAppUpdate() {
  if (!appUpdate.value) return;
  appUpdating.value = true;
  appUpdateProg.value = [];
  try {
    await App.InstallAppUpdate(); // 成功即退出重启
  } catch (e) {
    appUpdateProg.value = [...appUpdateProg.value, "更新失败: " + errText(e)];
    appUpdating.value = false;
  }
}

onMounted(async () => {
  try {
    info.value = await App.AppInfo();
  } catch (e) {
    error.value = errText(e);
  }
  EventsOn("appupdate:progress", (m: any) => {
    appUpdateProg.value = [...appUpdateProg.value.slice(-6), String(Array.isArray(m) ? m[0] : m)];
  });
  // 菜单栏动作（desktop.go buildMenu → EventsEmit）：菜单与 UI 同一落点
  EventsOn("menu:settings", () => {
    settingsMode.value = "settings";
    settingsOpen.value = true;
  });
  EventsOn("menu:about", () => {
    settingsMode.value = "about";
    settingsOpen.value = true;
  });
  EventsOn("menu:new-case", focusNewCase);
  EventsOn("menu:import-doc", () => (workspaceRef.value as any)?.importFile?.());
  EventsOn("menu:law-search", () => (workspaceRef.value as any)?.focusLawSearch?.());
  EventsOn("menu:case-search", () => (workspaceRef.value as any)?.focusCaseSearch?.());
  document.addEventListener("dragenter", onDragEnter);
  document.addEventListener("dragleave", onDragLeave);
  document.addEventListener("dragend", onDragEnd);
  document.addEventListener("mousedown", onDisplayDocDown);
  window.addEventListener("keydown", onGlobalKey);
  checkAppUpdate();
});

onUnmounted(() => {
  document.removeEventListener("dragenter", onDragEnter);
  document.removeEventListener("dragleave", onDragLeave);
  document.removeEventListener("dragend", onDragEnd);
  document.removeEventListener("mousedown", onDisplayDocDown);
  window.removeEventListener("keydown", onGlobalKey);
});
</script>

<template>
  <header class="topbar">
    <h1><span aria-hidden="true">⚖️</span> OpenCourt<span v-if="info" class="ver">v{{ info.version }}</span></h1>
    <div v-if="info" class="badges">
      <span class="badge profile">使用角色：{{ roleLabel }}</span>
      <span class="badge" :class="info.modelEnabled ? 'on' : ''">
        智能模型 {{ info.modelEnabled ? "已启用" : "离线" }}
      </span>
      <span class="badge" :class="info.browserEnabled ? 'on' : ''">
        外网检索 {{ info.browserEnabled ? "已启用" : "未启用" }}
      </span>
      <span class="badge" :class="info.telemetry ? 'warn' : ''">
        使用统计 {{ info.telemetry ? "开启" : "关闭" }}
      </span>
    </div>
    <div class="display-wrap">
      <button
        ref="displayBtn"
        class="badge display-btn"
        :aria-expanded="displayOpen ? 'true' : 'false'"
        aria-controls="display-pop"
        @click="toggleDisplay"
      >显示设置 ▾</button>
      <div
        v-if="displayOpen"
        id="display-pop"
        ref="displayPop"
        class="display-pop"
        tabindex="-1"
        role="group"
        aria-label="显示设置"
      >
        <p class="dp-label">字号</p>
        <div class="dp-row">
          <button class="btn sm" :aria-pressed="displaySize === 'compact'" @click="setDisplaySize('compact')">紧凑</button>
          <button class="btn sm" :aria-pressed="displaySize === 'normal'" @click="setDisplaySize('normal')">标准</button>
          <button class="btn sm" :aria-pressed="displaySize === 'large'" @click="setDisplaySize('large')">大字</button>
        </div>
        <p class="dp-label">配色</p>
        <div class="dp-row">
          <button class="btn sm" :aria-pressed="themeMode === 'system'" @click="setTheme('system')">跟随系统</button>
          <button class="btn sm" :aria-pressed="themeMode === 'light'" @click="setTheme('light')">浅色</button>
          <button class="btn sm" :aria-pressed="themeMode === 'dark'" @click="setTheme('dark')">深色</button>
        </div>
        <p class="dp-note">只影响字号与配色；默认跟随系统外观，手动选择会记住并下次启动继续生效。</p>
      </div>
    </div>
  </header>
  <p v-if="error" class="error" role="status">{{ error }}</p>

  <!-- 案件切换器（v0.2 M1）：一切事实/冲突/文档/要素按案件隔离 -->
  <CaseBar ref="caseBarRef" @switched="onCaseSwitched" />

  <!-- 程序更新横幅（发现新版才显示；两步确认替代原生 confirm） -->
  <div v-if="appUpdate && appUpdate.available" class="update-banner">
    <div class="ub-row">
      <span class="ub-title">新版本 {{ appUpdate.latest }}</span>
      <span class="muted">当前 {{ appUpdate.current }}<template v-if="appUpdate.package"> · {{ appUpdate.package }}（{{ humanSize(appUpdate.size || 0) }}）</template></span>
      <span class="spacer"></span>
      <button class="btn sm" @click="showUpdateNotes = !showUpdateNotes">更新说明</button>
      <template v-if="!updateConfirm">
        <button class="btn primary sm" :disabled="appUpdating" @click="updateConfirm = true">下载并更新</button>
      </template>
      <template v-else>
        <span class="ub-confirm" role="status">安装将退出并重启应用，确认？</span>
        <button class="btn primary sm" :disabled="appUpdating" @click="doAppUpdate">{{ appUpdating ? '下载中…' : '确认安装' }}</button>
        <button class="btn sm" :disabled="appUpdating" @click="updateConfirm = false">取消</button>
      </template>
    </div>
    <pre v-if="showUpdateNotes" class="ub-notes">{{ appUpdate.notes || '（无说明）' }}</pre>
    <ul v-if="appUpdateProg.length" class="prog-list" role="status">
      <li v-for="(l, i) in appUpdateProg" :key="i">{{ l }}</li>
    </ul>
  </div>

  <Workspace ref="workspaceRef" :info="info" :guide="guide" @focus-new-case="focusNewCase" @case-changed="onCaseChanged" />

  <!-- 拖放导入遮罩（文件拖入窗口时的落点提示；导入经 Go 侧同一管道） -->
  <div v-if="dragging" class="drop-overlay" role="status">
    <div class="drop-card">松开以导入卷宗文档（PDF / txt / md / docx；图片需 OCR 引擎）</div>
  </div>

  <!-- 设置 / 关于面板（Cmd+,） -->
  <SettingsPanel v-if="settingsOpen" :mode="settingsMode" :info="info" @close="settingsOpen = false" />

  <footer class="disclaimer">
    OpenCourt 为 AI 辅助工具：不预测判决结果、不引导决策、不替代律师与法官判断。最终裁判权与诉讼决策权归属人类。
  </footer>
</template>

<style scoped>
.topbar {
  display: flex; align-items: center; justify-content: space-between; gap: 10px;
  padding: 14px 24px; background: var(--topbar-bg); color: var(--text-inv); flex-wrap: wrap;
}
.topbar h1 { font-size: var(--fs-lg); margin: 0; }
.ver { margin-left: 8px; font-size: var(--fs-xs); font-weight: 400; opacity: 0.7; }
.badges { display: flex; gap: 8px; flex-wrap: wrap; }
.badge { background: var(--topbar-badge); border-radius: var(--r-pill); padding: 3px 12px; font-size: var(--fs-xs); }
.badge.profile, .badge.on { background: var(--accent); }
.badge.warn { background: var(--warn); }
/* 显示设置入口与弹层（顶栏右侧；弹层用表面令牌，深浅色自动跟随） */
.display-wrap { position: relative; }
.display-btn { border: 0; color: var(--text-inv); font-size: var(--fs-xs); min-height: 26px; padding: 3px 12px; border-radius: var(--r-pill); }
.display-btn:hover:not(:disabled) { background: var(--topbar-badge); }
.display-pop {
  position: absolute; right: 0; top: calc(100% + 8px); z-index: 30; width: 230px;
  background: var(--surface); color: var(--text-1); border: 1px solid var(--border-strong);
  border-radius: var(--r-2); padding: 10px 12px; box-shadow: 0 8px 24px rgba(0, 0, 0, 0.18);
  font-size: var(--fs-sm);
}
.dp-label { margin: 6px 0 4px; font-size: var(--fs-xs); color: var(--text-3); }
.dp-row { display: flex; gap: 6px; }
.dp-note { margin: 8px 0 0; font-size: var(--fs-xs); color: var(--text-3); }
.update-banner { margin: 12px 24px 0; border: 1px solid var(--info-border); background: var(--info-bg); border-radius: var(--r-3); padding: 10px 14px; }
.ub-row { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; }
.ub-title { font-weight: 600; color: var(--info); }
.ub-confirm { font-size: var(--fs-sm); color: var(--info); }
.ub-notes { margin: 8px 0 0; padding: 8px 10px; background: var(--surface); border-radius: var(--r-1); font-size: var(--fs-xs); white-space: pre-wrap; }
.prog-list { font-size: var(--fs-xs); color: var(--text-2); padding-left: 18px; margin: 8px 0 0; }
.spacer { flex: 1; }
.muted { color: var(--text-3); }
.error { color: var(--bad); font-size: var(--fs-sm); padding: 0 24px; }
.disclaimer {
  padding: 12px 24px; font-size: var(--fs-xs); color: var(--text-3);
  border-top: 1px solid var(--border); background: var(--surface);
}
.drop-overlay {
  position: fixed; inset: 0; z-index: 40; --wails-drop-target: drop;
  background: color-mix(in srgb, var(--accent) 12%, transparent);
  border: 3px dashed var(--accent); border-radius: 8px;
  display: flex; align-items: center; justify-content: center; pointer-events: none;
}
.drop-card {
  background: var(--surface); color: var(--accent-ink); border: 1px solid var(--accent);
  border-radius: var(--r-pill); padding: 10px 22px; font-size: var(--fs-md);
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.18);
}
</style>

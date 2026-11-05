<script setup lang="ts">
// App 外壳：顶栏 / 案件切换 / 更新横幅 / 事实录入 / 面板组装。
// 各功能区已抽出为 components/（distill）；案件切换统一走 onCaseSwitched。
import { onMounted, ref } from "vue";
import * as App from "../wailsjs/go/main/App";
import type { domain } from "../wailsjs/go/models";
import { EventsOn } from "../wailsjs/runtime/runtime";
import { humanSize } from "./util";
import FactBoard from "./components/FactBoard.vue";
import CaseBar from "./components/CaseBar.vue";
import LawSearch from "./components/LawSearch.vue";
import SimilarCases from "./components/SimilarCases.vue";
import CaseSearch from "./components/CaseSearch.vue";
import SimPanel from "./components/SimPanel.vue";
import LawDataPanel from "./components/LawDataPanel.vue";
import GateCheck from "./components/GateCheck.vue";
import DocsReader from "./components/DocsReader.vue";
import ReviewPanel from "./components/ReviewPanel.vue";
import BrowserOSPanel from "./components/BrowserOSPanel.vue";

const info = ref<Record<string, any> | null>(null);
const facts = ref<domain.Fact[]>([]);
const desc = ref("");
const sourceFile = ref("");
const pageNum = ref(1);
const snippet = ref("");
const fromOpponent = ref(false);
const error = ref("");

// 当事人姓名（脱敏替换用）：录入于此，供 BrowserOS 面板与报告导出共用
const webNames = ref("");
function nameList(): string[] {
  return webNames.value.split(/[,，]/).map((s) => s.trim()).filter(Boolean);
}

const lawSearchRef = ref(null);
const caseSearchRef = ref(null);
const docSearchRef = ref(null);
const simPanelRef = ref(null);
const docsReaderRef = ref(null);
const reviewPanelRef = ref(null);

async function refresh() {
  facts.value = await App.ListFacts();
}

// 案件切换后刷新当前案件的全部面板数据（事实/文档/时间线均按案件隔离）
async function onCaseSwitched() {
  try {
    await refresh();
    await (docsReaderRef.value as any)?.refresh?.();
    await (reviewPanelRef.value as any)?.refresh?.();
    (lawSearchRef.value as any)?.refresh?.();
    (caseSearchRef.value as any)?.refresh?.();
    (docSearchRef.value as any)?.refresh?.();
    (simPanelRef.value as any)?.refresh?.();
  } catch (e) {
    error.value = String(e);
  }
}

async function addFact() {
  error.value = "";
  try {
    await App.CreateFact(desc.value, sourceFile.value, pageNum.value, snippet.value, fromOpponent.value);
    desc.value = ""; snippet.value = "";
    await refresh();
  } catch (e) {
    error.value = String(e);
  }
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
    appUpdateProg.value = [...appUpdateProg.value, "更新失败: " + String(e)];
    appUpdating.value = false;
  }
}

onMounted(async () => {
  try {
    info.value = await App.AppInfo();
    await refresh();
  } catch (e) {
    error.value = String(e);
  }
  EventsOn("appupdate:progress", (m: any) => {
    appUpdateProg.value = [...appUpdateProg.value.slice(-6), String(Array.isArray(m) ? m[0] : m)];
  });
  checkAppUpdate();
});
</script>


<template>
  <header class="topbar">
    <h1><span aria-hidden="true">⚖️</span> OpenCourt</h1>
    <div v-if="info" class="badges">
      <span class="badge">v{{ info.version }}</span>
      <span class="badge profile">Profile: {{ info.profile }}</span>
      <span class="badge" :class="info.telemetry ? 'warn' : 'ok'">
        遥测 {{ info.telemetry ? "开启" : "关闭" }}
      </span>
      <span class="badge" :class="info.modelEnabled ? 'warn' : 'ok'">
        模型 {{ info.modelEnabled ? "已启用" : "离线" }}
      </span>
      <span class="badge" :class="info.browserEnabled ? 'warn' : 'ok'">
        BrowserOS {{ info.browserEnabled ? "已启用" : "禁用" }}
      </span>
      <span class="badge">脱敏级别: {{ info.redactLevel }}</span>
    </div>
  </header>

  <!-- 案件切换器（v0.2 M1）：一切事实/冲突/文档/要素按案件隔离 -->
  <CaseBar @switched="onCaseSwitched" />

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

  <main class="layout">
    <LawDataPanel />

    <!-- 法条检索与文书引用核验（v0.2 M2）：核验判定 = OpenLaw 错误码原样透传；
         UNKNOWN/pending/OL-1006 必须显式标出，不得静默通过 -->
    <section class="panel wide">
      <h2>法条检索与引用核验</h2>
      <LawSearch ref="lawSearchRef" />
    </section>

    <!-- 类案检索（v0.2 M3）：LocalCases 默认禁用；要旨强制脱敏；facet 附「不是裁判概率」标注 -->
    <SimilarCases ref="caseSearchRef" />

    <!-- 卷宗检索（v0.3 M5）：仅本案文档；降级必须显式标注；结果不进五态 -->
    <CaseSearch ref="docSearchRef" />

    <!-- 对抗推演与庭审流程（v0.3 M6）：全程免责标注；后端硬守卫；拦截必须可见 -->
    <section class="panel wide">
      <h2>对抗推演与庭审流程（模拟推演，非预测）</h2>
      <SimPanel ref="simPanelRef" />
    </section>

    <section class="panel">
      <h2>录入事实</h2>
      <p class="hint">初始状态由状态机判定：有引用→单源未补强；对方来源→对方主张；无来源→无依据。</p>
      <textarea v-model="desc" rows="2" aria-label="事实描述（必填）" placeholder="事实描述（必填）" class="full"></textarea>
      <div class="row">
        <input v-model="sourceFile" aria-label="来源文件（可空）" placeholder="来源文件（可空）" />
        <input v-model.number="pageNum" type="number" min="1" aria-label="页码" class="num" />
        <input v-model="snippet" aria-label="原文片段" placeholder="原文片段" />
      </div>
      <div class="row">
        <label><input type="checkbox" v-model="fromOpponent" /> 来自对方文书/陈述</label>
        <button @click="addFact">录入</button>
      </div>
      <p v-if="error" class="error" role="status">{{ error }}</p>
    </section>

    <GateCheck />

    <DocsReader ref="docsReaderRef" @changed="refresh" />

    <BrowserOSPanel :enabled="!!(info && info.browserEnabled)" v-model:names="webNames" @changed="refresh" />

    <ReviewPanel :names="nameList()" />

    <section class="panel wide">
      <h2>事实看板（五态）</h2>
      <FactBoard :facts="facts" @changed="refresh" />
    </section>
  </main>

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
.badges { display: flex; gap: 8px; flex-wrap: wrap; }
.badge { background: var(--topbar-badge); border-radius: var(--r-pill); padding: 3px 12px; font-size: var(--fs-xs); }
.badge.profile { background: var(--accent); }
.badge.ok { background: var(--ok); }
.badge.warn { background: var(--warn); }
/* 宽屏双栏、窄窗自动单栏（adapt） */
.layout { display: grid; grid-template-columns: repeat(auto-fit, minmax(460px, 1fr)); gap: 16px; padding: 20px 24px; }
@media (max-width: 560px) {
  .layout { grid-template-columns: 1fr; padding: 12px; }
}
.update-banner { margin: 12px 24px 0; border: 1px solid var(--info-border); background: var(--info-bg); border-radius: var(--r-3); padding: 10px 14px; }
.ub-row { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; }
.ub-title { font-weight: 600; color: var(--info); }
.ub-confirm { font-size: var(--fs-sm); color: var(--info); }
.ub-notes { margin: 8px 0 0; padding: 8px 10px; background: var(--surface); border-radius: var(--r-1); font-size: var(--fs-xs); white-space: pre-wrap; }
.prog-list { font-size: var(--fs-xs); color: var(--text-2); padding-left: 18px; margin: 8px 0 0; }
.spacer { flex: 1; }
.muted { color: var(--text-3); }
.panel { background: var(--surface); border: 1px solid var(--border); border-radius: var(--r-3); padding: 16px; }
.panel.wide { grid-column: 1 / -1; }
.panel h2 { margin: 0 0 8px; font-size: var(--fs-md); }
.hint { color: var(--text-3); font-size: var(--fs-xs); margin: 0 0 10px; }
.row { display: flex; gap: 8px; align-items: center; margin-top: 8px; flex-wrap: wrap; }
.full { width: 100%; }
.num { width: 80px; }
.error { color: var(--bad); font-size: var(--fs-sm); }
.disclaimer {
  padding: 12px 24px; font-size: var(--fs-xs); color: var(--text-3);
  border-top: 1px solid var(--border); background: var(--surface);
}
</style>



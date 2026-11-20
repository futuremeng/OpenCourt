<script setup lang="ts">
// 工作台（v0.4 P1，自 App.vue 外壳拆出）：面板组装 + 事实录入 + 案件切换刷新契约。
// 引导动线是 Profile 数据（profile.guide）：有则渲染向导，代码不判断 role。
import { computed, onMounted, ref } from "vue";
import * as App from "../../wailsjs/go/main/App";
import type { domain } from "../../wailsjs/go/models";
import FactBoard from "../components/FactBoard.vue";
import LawSearch from "../components/LawSearch.vue";
import SimilarCases from "../components/SimilarCases.vue";
import CaseSearch from "../components/CaseSearch.vue";
import SimPanel from "../components/SimPanel.vue";
import LawDataPanel from "../components/LawDataPanel.vue";
import GateCheck from "../components/GateCheck.vue";
import DocsReader from "../components/DocsReader.vue";
import ReviewPanel from "../components/ReviewPanel.vue";
import BrowserOSPanel from "../components/BrowserOSPanel.vue";
import Onboard from "./Onboard.vue";

const props = defineProps<{
  info: Record<string, any> | null;
  guide: { title: string; text: string; action: string }[];
}>();

// 向导里「新建案件」的落点在壳层的 CaseBar，经事件上抛
const emit = defineEmits<{ (e: "focus-new-case"): void }>();

const facts = ref<domain.Fact[]>([]);
const desc = ref("");
const sourceFile = ref("");
const pageNum = ref(1);
const snippet = ref("");
const fromOpponent = ref(false);
const error = ref("");

// 向导进度取真实状态（当前案件 / 本案文档数），不是点击计数
const currentCase = ref<any | null>(null);
const docCount = ref(0);

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

// 案件/文档变化后的向导状态（文档计数失败只说明还没有本案文档，不作错误弹窗）
async function refreshWizard() {
  try {
    currentCase.value = await App.CurrentCase();
    docCount.value = currentCase.value ? (await App.ListDocuments()).length : 0;
  } catch {
    currentCase.value = null;
    docCount.value = 0;
  }
}

// 案件切换后刷新当前案件的全部面板数据（事实/文档/时间线均按案件隔离）
async function onCaseSwitched() {
  error.value = "";
  try {
    await Promise.all([refresh(), refreshWizard()]);
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

// 文档接入（拖放或导入）后同步向导计数
async function onDocsChanged() {
  await Promise.all([refresh(), refreshWizard()]);
}

// 向导步骤的落点；未识别的 action 由 Onboard 只展示文字、不给按钮。
function stepAction(action: string) {
  if (action === "new-case") emit("focus-new-case");
  else if (action === "import-doc") importFile();
  else if (action === "document") scrollTo("sec-facts");
  else if (action === "report") scrollTo("sec-report");
}

function importFile() {
  (docsReaderRef.value as any)?.importFile?.();
}
function scrollTo(id: string) {
  document.getElementById(id)?.scrollIntoView({ block: "start" });
}

// 向导每步的真实状态：完成与否由当前案件/文档/事实计数推导，不是点击计数
const stepStatus = computed<Record<string, { done: boolean; detail: string }>>(() => {
  const s: Record<string, { done: boolean; detail: string }> = {};
  for (const g of props.guide) {
    if (g.action === "new-case") {
      s[g.action] = currentCase.value
        ? { done: true, detail: `当前案件：${currentCase.value.title}` }
        : { done: false, detail: "尚未选择案件；事实与文档都以案件为边界" };
    } else if (g.action === "import-doc") {
      s[g.action] = docCount.value > 0
        ? { done: true, detail: `本案已接入 ${docCount.value} 份文档` }
        : { done: false, detail: "本案还没有文档" };
    } else if (g.action === "document") {
      s[g.action] = facts.value.length > 0
        ? { done: true, detail: `已录入 ${facts.value.length} 条事实` }
        : { done: false, detail: "还没有录入事实" };
    } else if (g.action === "report") {
      s[g.action] = {
        done: false,
        detail: facts.value.length > 0 ? "可以审校：在「审校与报告」面板执行" : "审校需要至少一条事实",
      };
    } else {
      // 未识别的 action：状态未知就如实留空，Onboard 也不给按钮
      s[g.action] = { done: false, detail: "" };
    }
  }
  return s;
});

// 外壳经此暴露菜单/快捷键的落点
defineExpose({ onCaseSwitched, importFile,
  focusLawSearch: () => (lawSearchRef.value as any)?.focusInput?.(),
  focusCaseSearch: () => (caseSearchRef.value as any)?.focusInput?.(),
  focusDocSearch: () => (docSearchRef.value as any)?.focusInput?.(),
});

onMounted(async () => {
  await Promise.all([refresh(), refreshWizard()]);
});
</script>

<template>
  <!-- 引导动线（数据来自 Profile，无 role 分支）：每一步的完成状态是真实计数 -->
  <Onboard v-if="guide.length" :steps="guide" :status="stepStatus" @action="stepAction" />

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

    <DocsReader ref="docsReaderRef" @changed="onDocsChanged" />

    <BrowserOSPanel :enabled="!!(info && info.browserEnabled)" v-model:names="webNames" @changed="refresh" />

    <ReviewPanel id="sec-report" :names="nameList()" />

    <section class="panel wide" id="sec-facts">
      <h2>事实看板（五态）</h2>
      <FactBoard :facts="facts" @changed="refresh" />
    </section>
  </main>
</template>

<style scoped>
.layout { display: grid; grid-template-columns: repeat(auto-fit, minmax(460px, 1fr)); gap: 16px; padding: 20px 24px; }
@media (max-width: 560px) {
  .layout { grid-template-columns: 1fr; padding: 12px; }
}
.panel { background: var(--surface); border: 1px solid var(--border); border-radius: var(--r-3); padding: 16px; }
.panel.wide { grid-column: 1 / -1; }
.panel h2 { margin: 0 0 8px; font-size: var(--fs-md); }
.hint { color: var(--text-3); font-size: var(--fs-xs); margin: 0 0 10px; }
.row { display: flex; gap: 8px; align-items: center; margin-top: 8px; flex-wrap: wrap; }
.full { width: 100%; }
.num { width: 80px; }
.error { color: var(--bad); font-size: var(--fs-sm); }
</style>

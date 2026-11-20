<script setup lang="ts">
// 工作台（v0.4 P3）：按办案阶段重组——总览/阅卷/事实/核验/报告/工具与数据。
// 设计定稿见 docs/design-workflow.md。面板一律 v-show 隐藏（保留内部状态与 ref），
// 引导动线是 Profile 数据（profile.guide）：有则渲染向导，代码不判断 role。
import { computed, onMounted, ref } from "vue";
import * as App from "../../wailsjs/go/main/App";
import type { domain } from "../../wailsjs/go/models";
import { baseName, errText, FACT_STATE_NAMES, profileLabel } from "../util";
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
import EmptyState from "../components/EmptyState.vue";
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
// 本案已接入文档列表：事实录入与向导计数共用，下拉展示短名不泄露长路径
const docs = ref<any[]>([]);

// 办案阶段（v0.4 P3）：常驻导航，键盘 Tab+Enter 可达；切换不销毁面板（v-show）
type StageId = "overview" | "review" | "facts" | "verify" | "report" | "tools";
const stages: { id: StageId; label: string }[] = [
  { id: "overview", label: "案件总览" },
  { id: "review", label: "阅卷" },
  { id: "facts", label: "事实" },
  { id: "verify", label: "核验" },
  { id: "report", label: "报告" },
  { id: "tools", label: "工具与数据" },
];
const activeStage = ref<StageId>("overview");
function switchStage(id: StageId) {
  activeStage.value = id;
}

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
    docs.value = currentCase.value ? await App.ListDocuments() : [];
    docCount.value = docs.value.length;
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
    error.value = errText(e);
  }
}

async function addFact() {
  error.value = "";
  try {
    await App.CreateFact(desc.value, sourceFile.value, pageNum.value, snippet.value, fromOpponent.value);
    desc.value = ""; snippet.value = "";
    await refresh();
  } catch (e) {
    error.value = errText(e);
  }
}

// 文档接入（拖放或导入）后同步向导计数
async function onDocsChanged() {
  await Promise.all([refresh(), refreshWizard()]);
}

// 总览的本案状态：计数一律来自真实数据（当前案件/文档/事实五态）
const stateCounts = computed(() => {
  const counts: { key: string; label: string; n: number }[] = [];
  for (const [key, label] of Object.entries(FACT_STATE_NAMES)) {
    counts.push({ key, label, n: facts.value.filter((f) => f.state === key).length });
  }
  return counts;
});

// 运行状态（自顶栏迁入工具区）：逐项如实显示，不渲染假状态
const roleLabel = computed(() => profileLabel(props.info?.profile));

// 向导步骤的落点；未识别的 action 由 Onboard 只展示文字、不给按钮。
// 落点可能不在当前阶段：先切阶段再执行（不新增第二入口）。
function stepAction(action: string) {
  if (action === "new-case") emit("focus-new-case");
  else if (action === "import-doc") { switchStage("review"); importFile(); }
  else if (action === "document") { switchStage("facts"); scrollTo("sec-facts"); }
  else if (action === "report") { switchStage("report"); scrollTo("sec-report"); }
}

function importFile() {
  switchStage("review");
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
        detail: facts.value.length > 0 ? "可以审校：在「报告」阶段执行" : "审校需要至少一条事实",
      };
    } else {
      // 未识别的 action：状态未知就如实留空，Onboard 也不给按钮
      s[g.action] = { done: false, detail: "" };
    }
  }
  return s;
});

// 外壳经此暴露菜单/快捷键的落点；目标面板若不在当前阶段，先切阶段再聚焦
defineExpose({ onCaseSwitched, importFile,
  focusLawSearch: () => { switchStage("verify"); (lawSearchRef.value as any)?.focusInput?.(); },
  focusCaseSearch: () => { switchStage("verify"); (caseSearchRef.value as any)?.focusInput?.(); },
  focusDocSearch: () => { switchStage("review"); (docSearchRef.value as any)?.focusInput?.(); },
});

onMounted(async () => {
  await Promise.all([refresh(), refreshWizard()]);
});
</script>

<template>
  <!-- 办案阶段导航（常驻；主行动在各阶段内部，见 docs/design-workflow.md） -->
  <nav class="stages" aria-label="办案阶段">
    <button
      v-for="s in stages"
      :key="s.id"
      class="stage-tab"
      :class="{ active: activeStage === s.id }"
      :aria-current="activeStage === s.id ? 'page' : undefined"
      @click="switchStage(s.id)"
    >{{ s.label }}</button>
  </nav>

  <main class="layout">
    <!-- 案件总览：向导（Profile 数据驱动）+ 本案真实计数 -->
    <div v-show="activeStage === 'overview'" class="stage-block">
      <Onboard v-if="guide.length" :steps="guide" :status="stepStatus" @action="stepAction" />
      <div class="stage-grid">
        <section class="panel wide">
          <h2>本案状态</h2>
          <template v-if="currentCase">
            <div class="mrow">
              <span class="badge on">当前案件：{{ currentCase.title }}</span>
              <span class="badge">本案文档 {{ docCount }} 份</span>
              <span class="badge">本案事实 {{ facts.length }} 条</span>
            </div>
            <div class="mrow states">
              <span v-for="c in stateCounts" :key="c.key" class="state-chip" :class="c.key.toLowerCase()">
                {{ c.label }} {{ c.n }}
              </span>
            </div>
            <p class="hint">以上计数来自当前案件的真实数据；五态由状态机判定，前端不改判。</p>
          </template>
          <EmptyState v-else text="尚未选择案件。事实与文档都以案件为边界。">
            <button v-if="!guide.length" class="btn primary" @click="emit('focus-new-case')">新建案件</button>
          </EmptyState>
          <p v-if="error" class="error" role="status">{{ error }}</p>
        </section>
      </div>
    </div>

    <!-- 阅卷：文档接入（主行动：导入卷宗文档）与本案卷宗检索 -->
    <div v-show="activeStage === 'review'" class="stage-grid">
      <DocsReader ref="docsReaderRef" @changed="onDocsChanged" />
      <CaseSearch ref="docSearchRef" />
    </div>

    <!-- 事实：录入（主行动）与五态看板 -->
    <div v-show="activeStage === 'facts'" class="stage-grid">
      <section class="panel">
        <h2>录入事实</h2>
        <p class="hint">初始状态由状态机判定：有引用→单源未补强；对方来源→对方主张；无来源→无依据。手工录入的引用会立即回原文定位，定位失败拒绝入库并留审计。</p>
        <textarea v-model="desc" rows="2" aria-label="事实描述（必填）" placeholder="事实描述（必填）" class="full"></textarea>
        <div class="row">
          <select v-model="sourceFile" aria-label="来源文书（可空）">
            <option value="">无来源</option>
            <option v-for="d in docs" :key="d.doc_id" :value="d.file">{{ baseName(d.file) }}（{{ d.page_count }} 页）</option>
          </select>
          <input v-model.number="pageNum" type="number" min="1" aria-label="页码" class="num" :disabled="!sourceFile" />
          <input v-model="snippet" aria-label="原文片段" placeholder="原文片段" :disabled="!sourceFile" />
        </div>
        <p v-if="!docs.length" class="hint">本案尚未接入文书：可先在「阅卷」接入卷宗；无来源的事按状态机判为「无依据」。</p>
        <div class="row">
          <label><input type="checkbox" v-model="fromOpponent" /> 来自对方文书/陈述</label>
          <button class="primary" @click="addFact">录入</button>
        </div>
        <p v-if="error" class="error" role="status">{{ error }}</p>
      </section>

      <section class="panel wide" id="sec-facts">
        <h2>事实看板（五态）</h2>
        <FactBoard :facts="facts" @changed="refresh" />
      </section>
    </div>

    <!-- 核验：法条检索与引用核验（主行动）、类案检索 -->
    <div v-show="activeStage === 'verify'" class="stage-grid">
      <!-- 法条检索与文书引用核验（v0.2 M2）：核验判定 = OpenLaw 错误码原样透传；
           UNKNOWN/pending/OL-1006 必须显式标出，不得静默通过 -->
      <section class="panel wide">
        <h2>法条检索与引用核验</h2>
        <LawSearch ref="lawSearchRef" />
      </section>
      <!-- 类案检索（v0.2 M3）：LocalCases 默认禁用；要旨强制脱敏；facet 附「不是裁判概率」标注 -->
      <SimilarCases ref="caseSearchRef" />
    </div>

    <!-- 报告：输出门禁自检 + 审校与报告 -->
    <div v-show="activeStage === 'report'" class="stage-grid">
      <GateCheck />
      <ReviewPanel id="sec-report" :names="nameList()" :facts-count="facts.length" />
    </div>

    <!-- 工具与数据：非办案主链的工具与系统状态（渐进披露，逐项如实） -->
    <div v-show="activeStage === 'tools'" class="stage-grid">
      <LawDataPanel />
      <BrowserOSPanel :enabled="!!(info && info.browserEnabled)" v-model:names="webNames" @changed="refresh" />
      <!-- 对抗推演与庭审流程（v0.3 M6）：全程免责标注；后端硬守卫；拦截必须可见 -->
      <section class="panel wide">
        <h2>对抗推演与庭审流程（模拟推演，非预测）</h2>
        <SimPanel ref="simPanelRef" />
      </section>
      <section class="panel">
        <h2>运行状态</h2>
        <dl class="status-list">
          <div><dt>使用角色</dt><dd>{{ roleLabel }}</dd></div>
          <div><dt>智能模型</dt><dd>{{ info && info.modelEnabled ? "已启用" : "离线" }}</dd></div>
          <div><dt>外网检索</dt><dd>{{ info && info.browserEnabled ? "已启用" : "未启用" }}</dd></div>
          <div><dt>使用统计（遥测）</dt><dd>{{ info && info.telemetry ? "开启" : "关闭" }}</dd></div>
          <div><dt>脱敏级别</dt><dd>{{ info ? info.redactLevel : "未知" }}</dd></div>
        </dl>
        <p class="hint">以上状态影响数据来源与隐私边界，以设置与后端实际配置为准。</p>
      </section>
    </div>
  </main>
</template>

<style scoped>
/* 阶段导航：常驻、键盘可达（Tab+Enter），当前阶段 aria-current */
.stages {
  display: flex; gap: 6px; flex-wrap: wrap;
  padding: 10px 24px 0;
}
.stage-tab {
  border: 1px solid var(--border-strong); background: var(--surface); color: var(--text-2);
  border-radius: var(--r-pill); padding: 5px 14px; font-size: var(--fs-sm); min-height: 28px;
}
.stage-tab.active {
  background: var(--accent-soft); border-color: var(--accent); color: var(--accent-ink); font-weight: 600;
}
.layout { padding: 12px 24px 20px; }
.stage-block { display: block; }
.stage-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(460px, 1fr)); gap: 16px; }
@media (max-width: 560px) {
  .stages { padding: 8px 12px 0; }
  .layout { padding: 10px 12px 12px; }
  .stage-grid { grid-template-columns: 1fr; }
}
.panel { background: var(--surface); border: 1px solid var(--border); border-radius: var(--r-3); padding: 16px; }
.panel.wide { grid-column: 1 / -1; }
.panel h2 { margin: 0 0 8px; font-size: var(--fs-md); }
.hint { color: var(--text-3); font-size: var(--fs-xs); margin: 0 0 10px; }
.row { display: flex; gap: 8px; align-items: center; margin-top: 8px; flex-wrap: wrap; }
.full { width: 100%; }
.num { width: 80px; }
.error { color: var(--bad); font-size: var(--fs-sm); }
.mrow { display: flex; gap: 8px; flex-wrap: wrap; margin: 0 0 8px; }
.badge { border-radius: var(--r-pill); padding: 3px 12px; font-size: var(--fs-xs); background: var(--neutral-bg); color: var(--text-2); }
.badge.on { background: var(--accent-soft); color: var(--accent-ink); }
/* 五态计数：底色即五态令牌（DESIGN.md：五态色只用于看板与状态标注） */
.state-chip { border-radius: var(--r-pill); padding: 3px 12px; font-size: var(--fs-xs); color: var(--text-2); border: 1px solid var(--border); }
.state-chip.traced_confirmed { background: var(--state-confirmed); }
.state-chip.single_source { background: var(--state-single); }
.state-chip.opponent_claim { background: var(--state-opponent); }
.state-chip.conflicted { background: var(--state-conflicted); }
.state-chip.unsupported { background: var(--state-unsupported); }
.status-list { margin: 0; }
.status-list > div { display: flex; gap: 10px; padding: 6px 0; border-bottom: 1px solid var(--border); }
.status-list > div:last-child { border-bottom: 0; }
.status-list dt { flex: 1; color: var(--text-3); font-size: var(--fs-sm); }
.status-list dd { margin: 0; font-size: var(--fs-sm); color: var(--text-1); }
</style>

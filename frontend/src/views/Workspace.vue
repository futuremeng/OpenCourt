<script setup lang="ts">
// 工作台（v0.4 P3）：按办案阶段重组——总览/阅卷/事实/核验/报告/工具与数据。
// 设计定稿见 docs/design-workflow.md。面板一律 v-show 隐藏（保留内部状态与 ref），
// 引导动线是 Profile 数据（profile.guide）：有则渲染向导，代码不判断 role。
import { computed, onMounted, ref } from "vue";
import * as App from "../../wailsjs/go/main/App";
import type { domain, neutral } from "../../wailsjs/go/models";
import { baseName, CASE_STAGE_NAMES, errHelp, errText, FACT_STATE_NAMES, factOutcomeText, profileLabel, redactLabel, stageLabel } from "../util";
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

// 向导里「新建案件」的落点在壳层的 CaseBar，经事件上抛；
// 左栏案件列表切换案件也上抛，由壳层刷新 CaseBar 后走同一刷新链路
const emit = defineEmits<{ (e: "focus-new-case"): void; (e: "case-changed"): void }>();

const facts = ref<domain.Fact[]>([]);
const desc = ref("");
const sourceFile = ref("");
const pageNum = ref(1);
const snippet = ref("");
const fromOpponent = ref(false);
const error = ref("");
// 录入成功后的白话回执（v0.4 P10）：归入哪一档 + 为什么，全部取自后端回传的真实字段
const factResult = ref("");

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
    await Promise.all([refresh(), refreshWizard(), loadCaseSpace()]);
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
  factResult.value = "";
  try {
    const f = await App.CreateFact(desc.value, sourceFile.value, pageNum.value, snippet.value, fromOpponent.value);
    desc.value = ""; snippet.value = "";
    prefillNote.value = "";
    factResult.value = factOutcomeText(f);
    await refresh();
  } catch (e) {
    error.value = errText(e);
  }
}

// 失败回执的补救落点（v0.4 P11/P12）：按错误正文里的补救语跳转；
// 未匹配不出按钮，补救全在错误文字自身（errHelp 与 Go 文案是成对契约）。
// 录入表单与事实看板共用同一 gotoHelp，不另开第二套补救路径。
const factErrHelp = computed(() => errHelp(error.value));
function gotoHelp(kind: string) {
  if (kind === "locate") {
    switchStage("review");
    (docSearchRef.value as any)?.focusInput?.();
  } else if (kind === "ingest") {
    importFile();
  }
}

// 检索命中→预填事实表单（v0.4 P9）：只预填不写入；提交仍走录入侧 fail-closed
// 回原文定位，片段与原文核对不上不会入库。命中片段可能被检索截短，提示人工核对。
const prefillNote = ref("");
function prefillFromHit(h: { file: string; page: number; snippet: string }) {
  if (!h.file) return;
  factResult.value = "";
  sourceFile.value = h.file;
  pageNum.value = h.page || 1;
  snippet.value = h.snippet || "";
  prefillNote.value = "已从卷宗检索命中预填：请在文书中核对上下文后再提交；命中片段可能被检索截短，核对不上不会入库。";
  switchStage("facts");
  scrollTo("sec-fact-form");
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

// ---- 案件空间（已批准 design-flow case-journey O01 三栏）----
// 左：真实案件列表 + 阶段筛选；中：五态事实块（冲突原样并列）；右：按时间要素
// 锚定的时间线。阶段、日期与出处都取后端真实字段，前端不编；缺什么如实列出。
const cases = ref<any[]>([]);
const caseQuery = ref("");
const stageFilter = ref("");
const conflicts = ref<domain.Conflict[]>([]);
const timeline = ref<neutral.TimelineItem[]>([]);
const caseStages = Object.entries(CASE_STAGE_NAMES).map(([code, label]) => ({ code, label }));

// 筛选只按标题包含与阶段字段匹配（不编造「已归档」「待复核」等后端没有的状态）
const filteredCases = computed(() => {
  const q = caseQuery.value.trim();
  return cases.value.filter(
    (c) => (!stageFilter.value || c.stage === stageFilter.value) && (!q || String(c.title || "").includes(q))
  );
});

// 案件列表与本案冲突/时间线：挂载与案件切换后刷新；失败如实上报
async function loadCaseSpace() {
  try {
    const [cs, cf, tl] = await Promise.all([App.ListCases(), App.ListConflicts(), App.BuildTimeline()]);
    cases.value = cs || [];
    conflicts.value = cf || [];
    timeline.value = tl || [];
  } catch (e) {
    error.value = errText(e);
  }
}

// 左栏切换案件：SwitchCase 成功后上抛，壳层刷新 CaseBar 并回调 onCaseSwitched
async function selectCase(id: string) {
  if (currentCase.value?.case_id === id) return;
  error.value = "";
  try {
    await App.SwitchCase(id);
    emit("case-changed");
  } catch (e) {
    error.value = errText(e);
  }
}

// 中栏事实块按五态分组（五态由后端判定，前端只呈现）
const factById = computed(() => new Map(facts.value.map((f) => [f.fact_id, f])));
const confirmedFacts = computed(() => facts.value.filter((f) => f.state === "TRACED_CONFIRMED"));
const openFacts = computed(() => facts.value.filter((f) => f.state === "SINGLE_SOURCE" || f.state === "OPPONENT_CLAIM"));
const unsupportedFacts = computed(() => facts.value.filter((f) => f.state === "UNSUPPORTED"));
// 争议事实：未消解的冲突按事实分组原样并列；不属于未消解冲突的冲突态事实
// 单独列出，同样不自动消解
const conflictGroups = computed(() => {
  const groups = conflicts.value
    .filter((c) => !c.is_resolved)
    .map((c) => ({
      conflict_id: c.conflict_id,
      point: c.conflict_point,
      facts: (c.fact_ids || []).map((id) => factById.value.get(id)).filter((f): f is domain.Fact => !!f),
    }));
  const covered = new Set(groups.flatMap((g) => g.facts.map((f) => f.fact_id)));
  const orphans = facts.value.filter((f) => f.state === "CONFLICTED" && !covered.has(f.fact_id));
  return { groups, orphans };
});
const hasFacts = computed(() => facts.value.length > 0);

// 时间线：有日期依据的入节点，没有的不虚构、列入「待补」
const datedTimeline = computed(() => timeline.value.filter((t) => t.Date && t.Date.trim()));
const undatedTimeline = computed(() => timeline.value.filter((t) => !(t.Date && t.Date.trim())));

// 出处的法言法语：《文书名》第 N 页；没有来源字段就返回空串，不编出处
function citationText(citation?: domain.Citation | null): string {
  if (!citation || !citation.SourceFile) return "";
  const name = baseName(citation.SourceFile);
  return citation.PageNum >= 1 ? `《${name}》第 ${citation.PageNum} 页` : `《${name}》`;
}

// 运行状态（自顶栏迁入工具区）：逐项如实显示，不渲染假状态
const roleLabel = computed(() => profileLabel(props.info?.profile));

// 双受众信息层级（v0.4 P8）：主视图先给公众一条进度叙述（全部由本案真实计数推导），
// 专业计数与五态收进展开区；没有可读的「已出报告」状态，故「审校·报告」步骤如实
// 显示为待进行，不编造已完成状态。
const flowSteps = computed(() => {
  const hasFacts = facts.value.length > 0;
  return [
    { id: "case", label: "建案", done: !!currentCase.value, meta: "",
      todo: "先在顶栏新建一个案件，一切材料都按案件归档。", action: "new-case", actionLabel: "新建案件" },
    { id: "docs", label: "卷宗接入", done: docCount.value > 0, meta: `${docCount.value} 份文书`,
      todo: "把起诉状、合同等文书接入本案。", action: "import-doc", actionLabel: "导入文书" },
    { id: "facts", label: "整理事实", done: hasFacts, meta: `${facts.value.length} 条`,
      todo: "从文书中把有出处的事实逐条录入。", action: "document", actionLabel: "去录入事实" },
    { id: "report", label: "审校与报告", done: false, meta: "",
      todo: hasFacts ? "在「报告」阶段执行审校，通过审校后即可导出案件报告。" : "审校需要至少一条已录入的事实。", action: "report", actionLabel: "去审校" },
  ];
});
const currentStep = computed(() => {
  const i = flowSteps.value.findIndex((s) => !s.done);
  return i < 0 ? flowSteps.value.length - 1 : i;
});
const nextStep = computed(() => flowSteps.value[currentStep.value]);
const progressLine = computed(() => `现在进行到第 ${currentStep.value + 1} 步「${nextStep.value.label}」：${nextStep.value.todo}`);

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
  await Promise.all([refresh(), refreshWizard(), loadCaseSpace()]);
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
          <h2>本案进展</h2>
          <p class="progress-line">{{ progressLine }}</p>
          <ol class="flow-steps" aria-label="案件进度">
            <li
              v-for="(s, i) in flowSteps"
              :key="s.id"
              :class="{ done: s.done, current: i === currentStep }"
              :aria-current="i === currentStep ? 'step' : undefined"
            >
              <span class="flow-mark" aria-hidden="true">{{ s.done ? "✓" : i === currentStep ? "▶" : "·" }}</span>
              <span>{{ s.label }}</span>
              <span v-if="s.done && s.meta" class="step-meta">{{ s.meta }}</span>
            </li>
          </ol>
          <p class="mrow">
            <button class="btn primary" @click="stepAction(nextStep.action)">{{ nextStep.actionLabel }}</button>
          </p>
          <details v-if="currentCase" class="pro">
            <summary>专业细节：案件计数与事实可靠程度五态</summary>
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
            <p class="hint">以上计数来自当前案件的真实数据。五态即事实的可靠程度，由系统按来源情况自动判定：原文核对通过且多来源交叉一致才能进入最高档。</p>
          </details>
          <p v-else class="hint">事实与文档都以案件为边界：建好案件后，接入卷宗、录入事实都会归入本案。</p>
          <p v-if="error" class="error" role="status">{{ error }}</p>
        </section>

        <!-- 案件空间（已批准画布 case-journey O01）：左案件列表与筛选、中五态事实、右时间线；
             后端没有的状态不出现，没有依据的日期不虚构 -->
        <div class="case-space panel">
          <aside class="rail" aria-label="案件列表">
            <h2>案件</h2>
            <input v-model="caseQuery" class="full" placeholder="按案件标题过滤" aria-label="按案件标题过滤" />
            <div class="chips" role="group" aria-label="按案件阶段筛选">
              <button class="chip" :class="{ on: !stageFilter }" :aria-pressed="!stageFilter" @click="stageFilter = ''">全部</button>
              <button
                v-for="s in caseStages"
                :key="s.code"
                class="chip"
                :class="{ on: stageFilter === s.code }"
                :aria-pressed="stageFilter === s.code"
                @click="stageFilter = stageFilter === s.code ? '' : s.code"
              >{{ s.label }}</button>
            </div>
            <button
              v-for="c in filteredCases"
              :key="c.case_id"
              class="case-item"
              :class="{ on: currentCase && c.case_id === currentCase.case_id }"
              :aria-current="currentCase && c.case_id === currentCase.case_id ? 'true' : undefined"
              @click="selectCase(c.case_id)"
            >
              <b>{{ c.title }}</b>
              <span class="ci-meta"><template v-if="c.case_type">{{ c.case_type }} · </template>{{ stageLabel(c.stage) }}</span>
            </button>
            <p v-if="!cases.length" class="hint">还没有案件：在顶栏新建一个，一切材料都按案件归档。</p>
            <p v-else-if="!filteredCases.length" class="hint">没有符合条件的案件：筛选只按标题文字与阶段匹配。</p>
            <p class="muted">阶段与事实状态都取自后端保存的字段，此处只呈现，前端不判定。</p>
          </aside>

          <section class="center">
            <h2>本案事实与文书</h2>
            <div class="doc-tabs" aria-label="文书选项卡">
              <span class="doc-tab on">案件综述</span>
              <button v-for="d in docs" :key="d.doc_id" class="doc-tab" @click="switchStage('review')">{{ baseName(d.file) }}</button>
            </div>
            <template v-if="currentCase">
              <h3>案件背景</h3>
              <p class="muted">案件背景以带出处的事实块呈现；系统不生成叙述来补全卷宗里没有的信息。</p>
              <div v-if="confirmedFacts.length" class="fact-block confirmed">
                <h3>无争议事实 <span class="tag ok">已溯源确认</span></h3>
                <p v-for="f in confirmedFacts" :key="f.fact_id">{{ f.description }}<span class="vchip">{{ citationText(f.citation) || "已溯源确认" }}</span></p>
              </div>
              <div v-if="conflictGroups.groups.length || conflictGroups.orphans.length" class="fact-block conflicted">
                <h3>争议事实 <span class="tag bad">存在冲突</span></h3>
                <p v-for="g in conflictGroups.groups" :key="g.conflict_id">
                  <template v-if="g.point">{{ g.point }}：</template><span v-for="(f, i) in g.facts" :key="f.fact_id">{{ i ? "；" : "" }}{{ f.description }}<span v-if="citationText(f.citation)" class="vchip">{{ citationText(f.citation) }}</span></span>
                </p>
                <p v-for="f in conflictGroups.orphans" :key="f.fact_id">{{ f.description }}<span v-if="citationText(f.citation)" class="vchip">{{ citationText(f.citation) }}</span></p>
                <p class="muted">互相矛盾的说法原样并列，系统不自动消解，由承办人判断。</p>
              </div>
              <div v-if="openFacts.length" class="fact-block open">
                <h3>待核事实 <span class="tag info">尚未补强</span></h3>
                <p v-for="f in openFacts" :key="f.fact_id">{{ f.description }}<span v-if="citationText(f.citation)" class="vchip">{{ citationText(f.citation) }}</span><span class="vchip">{{ FACT_STATE_NAMES[f.state] || f.state }}</span></p>
                <p class="muted">这些事实只有单一来源或来自对方陈述，卷宗内暂无补强材料。</p>
                <button class="link-btn" @click="gotoHelp('locate')">去阅卷检索核对 ›</button>
              </div>
              <div v-if="unsupportedFacts.length" class="fact-block nobasis">
                <h3>无依据事实 <span class="tag">无依据</span></h3>
                <p v-for="f in unsupportedFacts" :key="f.fact_id">{{ f.description }}<span class="vchip">没有找到可核对的来源</span></p>
              </div>
              <p v-if="!hasFacts" class="hint">本案还没有已录入的事实：在「事实」阶段录入后，会按可靠程度出现在这里。</p>
              <p class="mrow"><button class="btn primary" @click="importFile()">接入新文书</button></p>
            </template>
            <p v-else class="hint">事实与文档都以案件为边界：在左侧列表选择或新建案件后，事实块与时间线会出现在这里。</p>
          </section>

          <aside class="rail" aria-label="案件时间线">
            <h2>案件时间线</h2>
            <p class="muted">按文书中的时间要素锚定；没有日期依据的节点不虚构，列入「待补」。</p>
            <ul v-if="datedTimeline.length" class="tl">
              <li v-for="t in datedTimeline" :key="t.Fact.fact_id">
                <span class="when">{{ t.Date }}</span>
                <span>{{ t.Fact.description }}</span><span class="vchip">{{ citationText(t.Fact.citation) || (FACT_STATE_NAMES[t.Fact.state] || t.Fact.state) }}</span>
              </li>
            </ul>
            <p v-else-if="timeline.length" class="hint">文书中未找到可锚定的时间要素；以下事实均无日期依据，列入「待补」。</p>
            <p v-else class="hint">本案还没有已录入的事实，时间线待案件推进后生成。</p>
            <template v-if="undatedTimeline.length">
              <h3>待补</h3>
              <ul class="tl pending">
                <li v-for="t in undatedTimeline" :key="t.Fact.fact_id">
                  <span class="when">待补</span>
                  <span>{{ t.Fact.description }}</span><span class="vchip">{{ citationText(t.Fact.citation) || "卷宗内暂无日期依据" }}</span>
                </li>
              </ul>
            </template>
          </aside>
        </div>
      </div>
    </div>

    <!-- 阅卷：文档接入（主行动：导入卷宗文档）与本案卷宗检索 -->
    <div v-show="activeStage === 'review'" class="stage-grid">
      <DocsReader ref="docsReaderRef" @changed="onDocsChanged" />
      <CaseSearch ref="docSearchRef" @save-fact="prefillFromHit" />
    </div>

    <!-- 事实：录入（主行动）与五态看板 -->
    <div v-show="activeStage === 'facts'" class="stage-grid">
      <section class="panel" id="sec-fact-form">
        <h2>录入事实</h2>
        <p v-if="prefillNote" class="hint" role="status">{{ prefillNote }}</p>
        <p class="hint">录入后系统按来源自动标注可靠程度：能回原文定位→「单源未补强」；来自对方陈述→「对方主张未验证」；两者皆无→「无依据」。手工填写的原文片段会立即回原文核对，核对不上不会入库，并留下记录。</p>
        <textarea v-model="desc" rows="2" aria-label="事实描述（必填）" placeholder="事实描述（必填）" class="full"></textarea>
        <div class="row">
          <select v-model="sourceFile" aria-label="来源文书（可空）">
            <option value="">无来源</option>
            <option v-for="d in docs" :key="d.doc_id" :value="d.file">{{ baseName(d.file) }}（{{ d.page_count }} 页）</option>
          </select>
          <input v-model.number="pageNum" type="number" min="1" aria-label="页码" class="num" :disabled="!sourceFile" />
          <input v-model="snippet" aria-label="原文片段" placeholder="原文片段" :disabled="!sourceFile" />
        </div>
        <p v-if="!docs.length" class="hint">本案尚未接入文书：可先在「阅卷」接入卷宗；无来源的事实会标注为「无依据」。</p>
        <div class="row">
          <label><input type="checkbox" v-model="fromOpponent" /> 来自对方文书/陈述</label>
          <button class="primary" @click="addFact">录入</button>
        </div>
        <p v-if="factResult" class="fact-result" role="status">{{ factResult }}</p>
        <p v-if="error" class="error" role="status">
          {{ error }}
          <button v-if="factErrHelp" class="primary" style="margin-left: 8px" @click="gotoHelp(factErrHelp)">
            {{ factErrHelp === "locate" ? "去卷宗检索核对" : "去接入该文书" }}
          </button>
        </p>
      </section>

      <section class="panel wide" id="sec-facts">
        <h2>事实看板（按可靠程度分列）</h2>
        <p class="hint">只有同时满足原文核对通过、多来源一致、人工确认，才能升级为「已溯源确认」；不满足会被拒绝并说明原因。</p>
        <FactBoard :facts="facts" @changed="refresh" @goto="gotoHelp" />
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
          <div><dt>使用统计</dt><dd>{{ info && info.telemetry ? "开启" : "关闭" }}</dd></div>
          <div><dt>脱敏级别</dt><dd>{{ redactLabel(info ? info.redactLevel : "") }}</dd></div>
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
.fact-result { color: var(--ok); font-size: var(--fs-sm); margin: 6px 0 0; }
.mrow { display: flex; gap: 8px; flex-wrap: wrap; margin: 0 0 8px; }
/* 本案进展（v0.4 P8 双受众层级） */
.progress-line { margin: 6px 0 10px; font-size: var(--fs-md); }
.flow-steps { list-style: none; display: flex; gap: 16px; padding: 0; margin: 0 0 12px; flex-wrap: wrap; font-size: var(--fs-sm); color: var(--text-2); }
.flow-steps li { display: flex; gap: 5px; align-items: baseline; }
.flow-steps li.done { color: var(--text-3); }
.flow-steps li.current { color: var(--accent-ink); font-weight: 600; }
.step-meta { color: var(--text-3); font-size: var(--fs-xs); }
.pro { margin-top: 10px; border-top: 1px solid var(--border); padding-top: 10px; }
.pro summary { cursor: pointer; font-size: var(--fs-sm); color: var(--text-2); }
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
/* 案件空间（case-journey O01 三栏）：左案件栏 / 中事实栏 / 右时间线栏；<900px 堆叠为单列 */
.case-space { grid-column: 1 / -1; display: grid; grid-template-columns: 250px minmax(0, 1fr) 280px; gap: 16px; align-items: start; }
@media (max-width: 900px) {
  .case-space { grid-template-columns: 1fr; }
}
.case-space h2 { margin: 0 0 8px; font-size: var(--fs-md); }
.case-space h3 { margin: 12px 0 6px; font-size: var(--fs-sm); color: var(--text-2); }
.case-space .muted { color: var(--text-3); font-size: var(--fs-xs); margin: 6px 0; }
.case-space input.full { width: 100%; }
.chips { display: flex; flex-wrap: wrap; gap: 6px; margin: 8px 0; }
.chip {
  border: 1px solid var(--border-strong); background: var(--surface); color: var(--text-2);
  border-radius: var(--r-pill); padding: 3px 10px; font-size: var(--fs-xs); min-height: 26px;
}
.chip.on { background: var(--accent-soft); border-color: var(--accent); color: var(--accent-ink); font-weight: 600; }
.case-item {
  display: block; width: 100%; text-align: left; background: var(--surface);
  border: 1px solid var(--border); border-radius: var(--r-2); padding: 8px 10px; margin-bottom: 8px;
}
.case-item.on { border-color: var(--accent); background: var(--accent-soft); }
.case-item b { display: block; font-size: var(--fs-sm); font-weight: 600; }
.ci-meta { color: var(--text-3); font-size: var(--fs-xs); }
.doc-tabs { display: flex; gap: 6px; flex-wrap: wrap; border-bottom: 1px solid var(--border); padding-bottom: 8px; margin-bottom: 12px; }
.doc-tab {
  border: 1px solid var(--border-strong); background: var(--surface-sunken); color: var(--text-2);
  border-radius: var(--r-1); padding: 4px 10px; font-size: var(--fs-xs); min-height: 26px;
}
.doc-tab.on { background: var(--accent); border-color: var(--accent); color: var(--text-inv); }
/* 五态事实块：底色即五态令牌（DESIGN.md：五态色只用于看板与状态标注） */
.fact-block { border: 1px solid var(--border); border-radius: var(--r-2); padding: 10px 12px; margin-bottom: 10px; }
.fact-block.confirmed { background: var(--state-confirmed); }
.fact-block.conflicted { background: var(--state-conflicted); }
.fact-block.open { background: var(--state-single); }
.fact-block.nobasis { background: var(--state-unsupported); }
.fact-block h3 { margin: 0 0 6px; font-size: var(--fs-sm); }
.fact-block p { margin: 0 0 6px; font-size: var(--fs-md); }
.fact-block .muted { margin: 4px 0 0; }
.tag { display: inline-block; border-radius: var(--r-1); padding: 1px 8px; font-size: var(--fs-xs); font-weight: 600; vertical-align: 1px; background: var(--neutral-bg); color: var(--text-2); }
.tag.ok { background: var(--ok-bg); color: var(--ok); }
.tag.bad { background: var(--bad-bg); color: var(--bad); }
.tag.info { background: var(--info-bg); color: var(--info); }
.vchip {
  display: inline-block; margin: 0 2px; padding: 1px 8px; border-radius: var(--r-pill);
  background: var(--info-bg); border: 1px solid var(--info-border); color: var(--info); font-size: var(--fs-xs);
}
.tl { list-style: none; margin: 0; padding: 0 0 0 16px; border-left: 2px solid var(--border-strong); }
.tl li { position: relative; padding: 0 0 10px 8px; font-size: var(--fs-sm); }
.tl li::before { content: ""; position: absolute; left: -21px; top: 5px; width: 8px; height: 8px; border-radius: 50%; background: var(--accent); }
.tl.pending li::before { background: var(--surface); border: 2px solid var(--border-strong); }
.when { display: block; color: var(--text-3); font-size: var(--fs-xs); }
.link-btn { border: 0; background: none; color: var(--accent); padding: 2px 6px; min-height: 24px; font-size: var(--fs-xs); }
</style>

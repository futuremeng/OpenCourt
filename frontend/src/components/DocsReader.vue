<script setup lang="ts">
// 文档接入与阅卷（阶段二）——自 App.vue 抽出（distill）。
// 纪律：模型只产候选事实，回原文定位失败即拒绝入库；时间/金额要素正则确定性抽取。
import { onMounted, ref } from "vue";
import { baseName, errText, FACT_STATE_NAMES, kindLabel } from "../util";
import * as App from "../../wailsjs/go/main/App";
import { EventsOn } from "../../wailsjs/runtime/runtime";

const emit = defineEmits<{ (e: "changed"): void }>();

const docs = ref<any[]>([]);
const readerReport = ref<any | null>(null);
const timeline = ref<any[]>([]);
const blindspots = ref<any[]>([]);
const requirements = ref("合同成立, 违约行为");
const busy = ref("");
const ingestMsg = ref("");
const annotations = ref<Record<string, any[]>>({});
const error = ref("");

const currentCaseType = ref("");
const elemMatch = ref<{ Covered: string[]; Missing: string[] } | null>(null);
const elemMsg = ref("");
const elemBusy = ref(false);

async function refreshDocs() {
  docs.value = await App.ListDocuments();
}

async function refreshTimeline() {
  timeline.value = await App.BuildTimeline();
}

async function refreshCurrentCaseType() {
  try {
    const c = await App.CurrentCase();
    currentCaseType.value = c?.case_type || "";
  } catch {
    currentCaseType.value = "";
  }
}

async function toggleAnnotations(docID: string) {
  if (annotations.value[docID]) {
    const { [docID]: _drop, ...rest } = annotations.value;
    annotations.value = rest;
    return;
  }
  try {
    annotations.value = { ...annotations.value, [docID]: await App.DocumentAnnotations(docID) };
  } catch (e) {
    error.value = errText(e);
  }
}

async function pickAndIngest() {
  error.value = ""; ingestMsg.value = "";
  try {
    const path = await App.PickDocumentFile();
    if (!path) return;
    const doc = await App.IngestDocument(path);
    ingestMsg.value = doc.ocr_engine
      ? `已接入 ${baseName(doc.file)}（${doc.page_count} 页）：文本由扫描识别而来，扫描质量低的页会标注待人工校对。`
      : `已接入 ${baseName(doc.file)}（${doc.page_count} 页）。`;
    await refreshDocs();
  } catch (e) {
    error.value = errText(e);
  }
}

async function runReader(docID: string) {
  error.value = ""; readerReport.value = null; busy.value = docID;
  try {
    readerReport.value = await App.RunReader(docID);
    await refreshTimeline();
    emit("changed");
  } catch (e) {
    error.value = errText(e);
  } finally {
    busy.value = "";
  }
}

async function runBlindspots() {
  error.value = "";
  try {
    const reqs = requirements.value.split(/[,，]/).map((s) => s.trim()).filter(Boolean);
    blindspots.value = await App.RunBlindspots(reqs);
  } catch (e) {
    error.value = errText(e);
  }
}

async function runElementMatchByCaseType() {
  error.value = ""; elemMsg.value = ""; elemMatch.value = null; elemBusy.value = true;
  try {
    const res = await App.RunElementMatchByCaseType(currentCaseType.value);
    elemMatch.value = { Covered: res.Covered || [], Missing: res.Missing || [] };
  } catch (e) {
    elemMsg.value = errText(e); // 无要件库等显式降级提示，绝不静默
  } finally {
    elemBusy.value = false;
  }
}

// 案件切换后：清空一次性结果，重载本案文档/时间线/案由
async function refresh() {
  readerReport.value = null; annotations.value = {}; blindspots.value = [];
  elemMatch.value = null; elemMsg.value = ""; ingestMsg.value = "";
  await Promise.all([refreshDocs(), refreshTimeline(), refreshCurrentCaseType()]);
}

// 拖放导入结果（Go 侧 OnFileDrop → IngestDocument 后经 docs:ingest 回报）
EventsOn("docs:ingest", (r: any) => {
  if (r?.ok) {
    ingestMsg.value = r.ocr_engine
      ? `拖入已接入 ${baseName(r.file)}（${r.pages} 页）：文本由扫描识别而来，扫描质量低的页会标注待人工校对。`
      : `拖入已接入 ${baseName(r.file)}（${r.pages} 页）。`;
    refreshDocs().catch((e) => (error.value = errText(e)));
    emit("changed");
  } else {
    error.value = `拖入文件导入失败：${r?.error || "未知错误"}`;
  }
});

onMounted(() => {
  refresh().catch((e) => (error.value = errText(e)));
});

// 菜单「文件→导入卷宗文档…」(Cmd+O) 的落点
defineExpose({ refresh, importFile: pickAndIngest });
</script>

<template>
  <section class="panel wide">
    <h2>文档接入与阅卷</h2>
    <p class="hint">
      系统阅读文书时只会生成候选事实；每条候选都必须能在原文对应页找到原句，找不到即拒绝入库（留下记录）。日期与金额按固定规则自动提取，不由模型改写。标记「扫描识别文本」的文书来自扫描件识别，系统会自动降低其可靠程度，扫描质量低的页需人工校对。
    </p>
    <div class="row">
      <button @click="pickAndIngest">接入文档（PDF/txt/md）</button>
      <span v-if="ingestMsg" class="ok-msg" role="status">{{ ingestMsg }}</span>
    </div>
    <p v-if="error" class="error" role="status">{{ error }}</p>
    <ul class="doc-list">
      <li v-for="d in docs" :key="d.doc_id">
        {{ baseName(d.file) }}（{{ d.page_count }} 页）
        <span v-if="d.ocr_engine" class="badge warn" title="文本由扫描识别而来；低置信页已标注，需人工校对">扫描识别文本</span>
        <button :disabled="busy === d.doc_id" @click="runReader(d.doc_id)">
          {{ busy === d.doc_id ? "阅卷中…" : "阅卷" }}
        </button>
        <button @click="toggleAnnotations(d.doc_id)">标注</button>
        <ul v-if="annotations[d.doc_id]?.length" class="annotations">
          <li v-for="(a, i) in annotations[d.doc_id]" :key="i">
            <span class="badge">{{ kindLabel(a.kind) }}</span>
            <b v-if="a.author">{{ a.author }}</b>
            {{ a.text }}
            <em v-if="a.meta" class="hint">（{{ a.meta }}）</em>
          </li>
        </ul>
      </li>
    </ul>
    <div v-if="readerReport" class="reader-report" role="status">
      <p>智能模型 {{ readerReport.model_name }}：入库 {{ readerReport.accepted.length }} 条 ·
        <b class="rejected">拒绝 {{ readerReport.rejected.length }} 条</b> ·
        提取时间/金额 {{ readerReport.elements.length }} 处</p>
      <ul>
        <li v-for="r in readerReport.rejected" :key="r.description">
          未入库：{{ r.description }} —— 所称原文「{{ r.snippet }}」在原文中找不到
        </li>
      </ul>
    </div>

    <h3>时间线（按时间要素锚定，不虚构日期）</h3>
    <ol class="timeline">
      <li v-for="t in timeline" :key="t.Fact.fact_id">
        <span class="tdate">{{ t.Date || "日期未定位" }}</span>
        {{ t.Fact.description }}
        <span class="tstate">{{ FACT_STATE_NAMES[t.Fact.state] || t.Fact.state }}</span>
      </li>
    </ol>

    <h3>要件匹配与盲区</h3>
    <div class="row">
      <input v-model="requirements" aria-label="要件列表（逗号分隔）" placeholder="要件列表（逗号分隔）" class="grow" />
      <button @click="runBlindspots">匹配并生成盲区</button>
    </div>
    <div class="row">
      <button :disabled="elemBusy" class="primary" @click="runElementMatchByCaseType">
        {{ elemBusy ? "匹配中…" : "按案由匹配要件库" }}
      </button>
      <span class="hint">当前案由：{{ currentCaseType || "（未设置）" }}；要件只从已导入的要件库中选取，要件库缺失时会明确提示，不会猜测。</span>
    </div>
    <p v-if="elemMsg" class="hint" role="status">{{ elemMsg }}</p>
    <ul v-if="elemMatch" class="blindspots" role="status">
      <li v-for="c in elemMatch.Covered" :key="'cov-' + c"><span class="mark-ok">✓</span> 已覆盖：{{ c }}</li>
      <li v-for="m in elemMatch.Missing" :key="'mis-' + m"><span class="mark-bad">✗</span> 缺失：{{ m }}</li>
    </ul>
    <ul class="blindspots">
      <li v-for="b in blindspots" :key="b.question">{{ b.question }} —— {{ b.supplement }}</li>
    </ul>
  </section>
</template>

<style scoped>
.hint { color: var(--text-3); font-size: var(--fs-xs); margin: 0 0 10px; }
.row { display: flex; gap: 8px; align-items: center; margin-top: 8px; flex-wrap: wrap; }
.grow { flex: 1; }
.error { color: var(--bad); font-size: var(--fs-sm); }
.ok-msg { color: var(--ok); font-size: var(--fs-sm); }
.doc-list { font-size: var(--fs-sm); padding-left: 18px; }
.doc-list li { margin: 4px 0; }
.doc-list button { margin-left: 8px; }
.annotations { margin: 4px 0; padding-left: 16px; font-size: var(--fs-xs); color: var(--warn); }
.annotations li { margin: 2px 0; }
.reader-report { background: var(--bad-bg); border: 1px solid var(--bad-border); border-radius: var(--r-2); padding: 8px 12px; font-size: var(--fs-sm); }
.reader-report .rejected { color: var(--bad); }
h3 { font-size: var(--fs-md); margin: 14px 0 4px; color: var(--text-2); }
.timeline { font-size: var(--fs-sm); padding-left: 20px; }
.timeline li { margin: 4px 0; }
.tdate { display: inline-block; min-width: 90px; color: var(--accent); font-weight: 600; }
.tstate { color: var(--text-3); font-size: var(--fs-xs); margin-left: 6px; }
.blindspots { font-size: var(--fs-sm); padding-left: 18px; }
.mark-ok { color: var(--ok); }
.mark-bad { color: var(--bad); }
.badge { padding: 1px 8px; border-radius: var(--r-pill); font-size: var(--fs-xs); background: var(--accent-soft); color: var(--accent-ink); }
.badge.warn { background: var(--warn-bg); color: var(--warn); }
</style>


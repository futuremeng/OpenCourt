<script setup lang="ts">
import { onMounted, ref } from "vue";
import * as App from "../wailsjs/go/main/App";
import type { AppInfo, Fact } from "../wailsjs/go/main/App";
import FactBoard from "./components/FactBoard.vue";

const info = ref<AppInfo | null>(null);
const facts = ref<Fact[]>([]);
const desc = ref("");
const sourceFile = ref("");
const pageNum = ref(1);
const snippet = ref("");
const fromOpponent = ref(false);
const gateText = ref("");
const gateResult = ref<string[]>([]);
const error = ref("");

async function refresh() {
  facts.value = await App.ListFacts();
}

onMounted(async () => {
  try {
    info.value = await App.AppInfo();
    await refresh();
  } catch (e) {
    error.value = String(e);
  }
});

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

async function checkGate() {
  const v = await App.CheckOutput(gateText.value);
  gateResult.value = v.length === 0 ? ["通过：未检测到引导性表述"] : v.map((x) => `[拦截] ${x.message}`);
}

// --- 阶段二：文档接入与阅卷 ---
const docs = ref<any[]>([]);
const readerReport = ref<any | null>(null);
const timeline = ref<any[]>([]);
const blindspots = ref<any[]>([]);
const requirements = ref("合同成立, 违约行为");
const busy = ref("");
const ingestMsg = ref("");

async function refreshDocs() {
  docs.value = await App.ListDocuments();
}

async function pickAndIngest() {
  error.value = ""; ingestMsg.value = "";
  try {
    const path = await App.PickDocumentFile();
    if (!path) return;
    const doc = await App.IngestDocument(path);
    ingestMsg.value = `已接入 ${doc.file}（${doc.page_count} 页）。扫描件/无文本层 PDF 会被拒绝并要求 OCR。`;
    await refreshDocs();
  } catch (e) {
    error.value = String(e);
  }
}

async function runReader(docID: string) {
  error.value = ""; readerReport.value = null; busy.value = docID;
  try {
    readerReport.value = await App.RunReader(docID);
    await refresh();
    await refreshTimeline();
  } catch (e) {
    error.value = String(e);
  } finally {
    busy.value = "";
  }
}

async function refreshTimeline() {
  timeline.value = await App.BuildTimeline();
}

async function runBlindspots() {
  error.value = "";
  try {
    const reqs = requirements.value.split(/[,，]/).map((s) => s.trim()).filter(Boolean);
    blindspots.value = await App.RunBlindspots(reqs);
  } catch (e) {
    error.value = String(e);
  }
}

onMounted(async () => {
  try {
    await refreshDocs();
    await refreshTimeline();
  } catch (e) {
    error.value = String(e);
  }
});
</script>

<template>
  <header class="topbar">
    <h1>⚖️ OpenCourt</h1>
    <div v-if="info" class="badges">
      <span class="badge">v{{ info.version }}</span>
      <span class="badge profile">Profile: {{ info.profile }}</span>
      <span class="badge" :class="info.telemetry ? 'warn' : 'ok'">
        遥测 {{ info.telemetry ? "开启" : "关闭" }}
      </span>
      <span class="badge" :class="info.modelEnabled ? 'warn' : 'ok'">
        模型 {{ info.modelEnabled ? "已启用" : "离线" }}
      </span>
      <span class="badge">脱敏级别: {{ info.redactLevel }}</span>
    </div>
  </header>

  <main class="layout">
    <section class="panel">
      <h2>录入事实</h2>
      <p class="hint">初始状态由状态机判定：有引用→单源未补强；对方来源→对方主张；无来源→无依据。</p>
      <textarea v-model="desc" rows="2" placeholder="事实描述（必填）" style="width:100%"></textarea>
      <div class="row">
        <input v-model="sourceFile" placeholder="来源文件（可空）" />
        <input v-model.number="pageNum" type="number" min="1" style="width:80px" />
        <input v-model="snippet" placeholder="原文片段" />
      </div>
      <div class="row">
        <label><input type="checkbox" v-model="fromOpponent" /> 来自对方文书/陈述</label>
        <button @click="addFact">录入</button>
      </div>
      <p v-if="error" class="error">{{ error }}</p>
    </section>

    <section class="panel">
      <h2>输出门禁自检</h2>
      <p class="hint">试试输入「建议起诉」或「大概率胜诉」——倾向性表述会被强制拦截。</p>
      <textarea v-model="gateText" rows="2" placeholder="待检文本" style="width:100%"></textarea>
      <button @click="checkGate">门禁检查</button>
      <ul class="gate-result">
        <li v-for="m in gateResult" :key="m">{{ m }}</li>
      </ul>
    </section>

    <section class="panel wide">
      <h2>文档接入与阅卷（阶段二）</h2>
      <p class="hint">
        阅卷时模型只生成候选事实；每条候选必须回原文定位，定位失败即被拒绝入库（审计留痕）。时间/金额要素由正则确定性抽取。
      </p>
      <div class="row">
        <button @click="pickAndIngest">接入文档（PDF/txt/md）</button>
        <span v-if="ingestMsg" class="ok-msg">{{ ingestMsg }}</span>
      </div>
      <ul class="doc-list">
        <li v-for="d in docs" :key="d.doc_id">
          {{ d.file }}（{{ d.page_count }} 页）
          <button :disabled="busy === d.doc_id" @click="runReader(d.doc_id)">
            {{ busy === d.doc_id ? "阅卷中…" : "阅卷" }}
          </button>
        </li>
      </ul>
      <div v-if="readerReport" class="reader-report">
        <p>模型 {{ readerReport.model_name }}：入库 {{ readerReport.accepted.length }} 条 ·
          <b class="rejected">拒绝 {{ readerReport.rejected.length }} 条（回原文失败）</b> ·
          要素 {{ readerReport.elements.length }} 个</p>
        <ul>
          <li v-for="r in readerReport.rejected" :key="r.description">
            [拒绝] {{ r.description }} —— 声称片段「{{ r.snippet }}」未在原文定位
          </li>
        </ul>
      </div>

      <h2 style="margin-top:14px">时间线（按时间要素锚定，不虚构日期）</h2>
      <ol class="timeline">
        <li v-for="t in timeline" :key="t.Fact.fact_id">
          <span class="tdate">{{ t.Date || "日期未定位" }}</span>
          {{ t.Fact.description }}
          <span class="tstate">{{ t.Fact.state }}</span>
        </li>
      </ol>

      <h2 style="margin-top:14px">要件匹配与盲区</h2>
      <div class="row">
        <input v-model="requirements" placeholder="要件列表（逗号分隔）" style="flex:1" />
        <button @click="runBlindspots">匹配并生成盲区</button>
      </div>
      <ul class="blindspots">
        <li v-for="b in blindspots" :key="b.question">{{ b.question }} —— {{ b.supplement }}</li>
      </ul>
    </section>

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
  display: flex; align-items: center; justify-content: space-between;
  padding: 14px 24px; background: #111827; color: #fff;
}
.topbar h1 { font-size: 18px; margin: 0; }
.badges { display: flex; gap: 8px; flex-wrap: wrap; }
.badge { background: #374151; border-radius: 999px; padding: 3px 12px; font-size: 12px; }
.badge.profile { background: #4338ca; }
.badge.ok { background: #065f46; }
.badge.warn { background: #92400e; }
.layout { display: grid; grid-template-columns: 1fr 1fr; gap: 16px; padding: 20px 24px; }
.panel { background: #fff; border: 1px solid #e5e7eb; border-radius: 10px; padding: 16px; }
.panel.wide { grid-column: 1 / -1; }
.panel h2 { margin: 0 0 8px; font-size: 15px; }
.hint { color: #6b7280; font-size: 12px; margin: 0 0 10px; }
.row { display: flex; gap: 8px; align-items: center; margin-top: 8px; flex-wrap: wrap; }
.error { color: #b91c1c; font-size: 13px; }
.gate-result { font-size: 13px; padding-left: 18px; }
.ok-msg { color: #065f46; font-size: 13px; }
.doc-list { font-size: 13px; padding-left: 18px; }
.doc-list li { margin: 4px 0; }
.doc-list button { margin-left: 8px; }
.reader-report { background: #fef2f2; border: 1px solid #fecaca; border-radius: 8px; padding: 8px 12px; font-size: 13px; }
.reader-report .rejected { color: #b91c1c; }
.timeline { font-size: 13px; padding-left: 20px; }
.timeline li { margin: 4px 0; }
.tdate { display: inline-block; min-width: 90px; color: #4338ca; font-weight: 600; }
.tstate { color: #6b7280; font-size: 12px; margin-left: 6px; }
.blindspots { font-size: 13px; padding-left: 18px; }
.disclaimer {
  padding: 12px 24px; font-size: 12px; color: #6b7280;
  border-top: 1px solid #e5e7eb; background: #fff;
}
</style>

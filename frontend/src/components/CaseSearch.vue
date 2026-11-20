<script setup lang="ts">
// 卷宗混合检索面板（v0.3 M5）：本案卷宗内关键词 + 向量召回 → RRF 融合。
// 纪律：范围只有本案文档（法条归 OpenLaw、类案归 LocalCases，见对应面板）；
// 语义召回未启用/不可用时必须显式黄标降级说明，绝不冒充混合；
// 结果是候选不是事实——本面板不直接写事实库：「存为事实」只预填录入表单，
// 仍需人工核对提交，由录入侧 fail-closed 回原文定位。
import { onMounted, ref } from "vue";
import { baseName, errText, sourceLabel } from "../util";
import * as App from "../../wailsjs/go/main/App";

const query = ref("");
const busy = ref(false);
const error = ref("");
const report = ref<any>(null);
const status = ref<any>(null);
// doc_id→文书短名：检索结果展示文件名而不是内部编号
const docNames = ref<Record<string, string>>({});
// doc_id→文件路径：供「存为事实」预填录入表单（只有路径能作 CreateFact 的来源参数）
const docFiles = ref<Record<string, string>>({});
const emit = defineEmits<{ (e: "save-fact", hit: { file: string; page: number; snippet: string }): void }>();

async function refreshDocNames() {
  try {
    const docs = await App.ListDocuments();
    const m: Record<string, string> = {};
    const f: Record<string, string> = {};
    for (const d of docs) { m[d.doc_id] = baseName(d.file); f[d.doc_id] = d.file; }
    docNames.value = m;
    docFiles.value = f;
  } catch {
    docNames.value = {};
    docFiles.value = {};
  }
}

async function refreshStatus() {
  try {
    status.value = await App.CaseIndexStatus();
  } catch (e) {
    error.value = errText(e);
  }
}

async function runSearch() {
  error.value = "";
  busy.value = true;
  report.value = null;
  try {
    report.value = await App.SearchCaseDocs(query.value.trim(), 10);
    await Promise.all([refreshStatus(), refreshDocNames()]);
  } catch (e) {
    error.value = errText(e);
  } finally {
    busy.value = false;
  }
}

function modeLabel(m: string): string {
  return m === "hybrid" ? "综合检索（关键词+语义）" : "仅关键词检索";
}

onMounted(() => {
  refreshStatus().catch(() => {});
  refreshDocNames();
});

// 全局快捷键 Cmd+K / 菜单「检索→法条检索」的落点：聚焦本面板输入框
const qInput = ref<HTMLInputElement | null>(null);
function focusInput() {
  qInput.value?.focus();
  qInput.value?.select();
}

defineExpose({ refresh: async () => { await Promise.all([refreshStatus(), refreshDocNames()]); }, focusInput });
</script>

<template>
  <section class="panel wide">
    <h2>卷宗检索（本案）</h2>
    <p class="hint">
      仅检索本案已接入文档（页级候选，不进事实库）。法条检索请走「法条检索与引用核验」，
      类案请走「类案检索」。
    </p>
    <div v-if="status" class="row">
      <span :class="status.model_enabled ? 'badge ok' : 'badge warn'">
        {{ status.model_enabled ? "语义检索已启用" : "语义检索未启用（离线/默认）" }}
      </span>
      <span v-if="status.model_enabled" class="hint">
        语义索引 {{ status.indexed_pages }}/{{ status.total_pages }} 页（仅本案）
      </span>
    </div>
    <div class="row">
      <input ref="qInput" v-model="query" aria-label="卷宗关键词（空格分词）" placeholder="关键词（空格分词）" class="grow" @keyup.enter="runSearch" />
      <button class="primary" :disabled="busy || !query.trim()" @click="runSearch">
        {{ busy ? "检索中…" : "检索卷宗" }}
      </button>
    </div>
    <p v-if="error" class="hint bad" role="status">{{ error }}</p>
    <template v-if="report">
      <p class="row">
        <span :class="report.mode === 'hybrid' ? 'badge ok' : 'badge warn'">{{ modeLabel(report.mode) }}</span>
        <span v-if="report.note" class="hint warn">{{ report.note }}</span>
      </p>
      <p v-if="!report.hits.length" class="hint">卷宗内无命中（卷宗里没有 ≠ 检索失败）。</p>
      <ul class="hits">
        <li v-for="h in report.hits" :key="h.ref">
          <b>{{ docNames[h.doc_id] || h.doc_id }} 第 {{ h.page }} 页</b>
          <span class="hint">（{{ h.sources.map(sourceLabel).join("+") }}；相关度 {{ h.score.toFixed(2) }}）</span>
          <div class="snippet">{{ h.snippet }}</div>
          <button
            v-if="docFiles[h.doc_id]"
            class="hit-save"
            @click="emit('save-fact', { file: docFiles[h.doc_id], page: h.page, snippet: h.snippet })"
          >存为事实</button>
        </li>
      </ul>
    </template>
  </section>
</template>

<style scoped>
.badge { padding: 2px 8px; border-radius: var(--r-pill); font-size: var(--fs-xs); }
.badge.ok { background: var(--ok-bg); color: var(--ok); }
.badge.warn { background: var(--warn-bg); color: var(--warn); }
.hint { color: var(--text-3); font-size: var(--fs-xs); }
.hint.warn { color: var(--warn); }
.hint.bad { color: var(--bad); }
.row { display: flex; gap: 8px; align-items: center; flex-wrap: wrap; }
.grow { flex: 1; }
ul.hits { list-style: none; padding: 0; }
ul.hits li { padding: 6px 0; border-bottom: 1px dashed var(--border); }
.snippet { color: var(--text-2); font-size: var(--fs-sm); margin-top: 2px; }
.hit-save { font-size: var(--fs-xs); margin-top: 4px; min-height: 26px; }
</style>

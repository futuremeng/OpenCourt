<script setup lang="ts">
// 类案检索面板（v0.2 M3，plan-v0.2 §5.3）：过滤条件 + 结果卡 + 分布统计。
// 纪律：未启用显示指引而非空列表；LC-6 未就绪时手动 base_url+token_env 为
// 临时通道（必须标注）；structured=false 必须提示降级并隐藏 facet；
// facet 分布必须附固定标注「不是裁判概率或胜诉可能性」并同显语料规模。
import { onMounted, ref } from "vue";
import * as App from "../../wailsjs/go/main/App";

const status = ref<any>(null);
const search = ref("");
const court = ref("");
const cause = ref("");
const yearFrom = ref("");
const yearTo = ref("");
const names = ref("");
const busy = ref(false);
const error = ref("");
const report = ref<any>(null);
const refs = ref<any[]>([]);

async function refreshStatus() {
  try {
    status.value = await App.LocalCasesStatus();
  } catch (e) {
    error.value = String(e);
  }
}

async function refreshRefs() {
  try {
    refs.value = await App.ListCaseRefs();
  } catch (e) {
    error.value = String(e);
  }
}

async function runSearch() {
  error.value = ""; busy.value = true; report.value = null;
  try {
    const ns = names.value.split(/[,，]/).map((s) => s.trim()).filter(Boolean);
    report.value = await App.SearchSimilarCases(
      search.value, court.value, cause.value,
      parseInt(yearFrom.value) || 0, parseInt(yearTo.value) || 0, 20, ns
    );
    await refreshRefs();
  } catch (e) {
    error.value = String(e);
  } finally {
    busy.value = false;
  }
}

function facetEntries(f: any): { name: string; buckets: any[] }[] {
  if (!f) return [];
  return Object.keys(f).map((k) => ({ name: k, buckets: f[k] || [] }));
}

onMounted(async () => {
  await refreshStatus();
  await refreshRefs();
});

defineExpose({ refresh: async () => { report.value = null; await refreshRefs(); } });
</script>

<template>
  <section class="panel wide casesearch">
    <h2>类案检索（LocalCases，外部可选组件）</h2>
    <p v-if="status && !status.enabled" class="law-notice">
      {{ status.guidance }}<br />
      <b>临时通道标注</b>：LC-6 发现文件未就绪前需手动填 base_url + token_env；
      OpenCourt 不会启动 LocalCases 进程，也不会改用其他来源。
    </p>
    <p v-else-if="status" class="hint">
      语料规模：<b>{{ status.indexed_docs }}</b> 篇（必须同显，防小样本误读）；
      版本：{{ status.version || "未知（LC-7 未就绪）" }}；
      <template v-if="!status.health_ok">探测失败：{{ status.health_error }}</template>
      <template v-if="!status.token_set"><b class="warn">token 未注入</b>（检查 token_env 环境变量）；</template>
    </p>

    <div class="row">
      <input v-model="search" placeholder="关键词（空格=AND）" style="flex:1" />
      <input v-model="cause" placeholder="案由（精确）" />
      <input v-model="court" placeholder="法院（精确）" />
      <input v-model="yearFrom" placeholder="年起" class="num" />
      <input v-model="yearTo" placeholder="年至" class="num" />
      <input v-model="names" placeholder="当事人姓名（脱敏一致替换）" />
      <button :disabled="busy || !(status && status.enabled)" class="primary" @click="runSearch">
        {{ busy ? "检索中…" : "检索类案" }}
      </button>
    </div>
    <p v-if="error" class="err">{{ error }}</p>

    <template v-if="report">
      <p v-if="!report.structured" class="law-notice">
        当前索引不支持精确过滤，以下结果<b>未按法院/案由等条件筛选</b>（LocalCases 诚实降级）；分布统计已隐藏。
      </p>
      <p class="hint">命中 {{ report.refs.length }} 条（语料内共 {{ report.total }}）；要旨已强制脱敏 {{ report.redact_count }} 项。</p>
      <ul class="caselist">
        <li v-for="r in report.refs" :key="r.ref_id">
          <b>{{ r.case_name || r.source_case_id }}</b>
          <span class="meta">{{ r.court }} · {{ r.cause }} · {{ r.judgment_date }}</span>
          <div class="summary">{{ r.summary }}</div>
          <div v-if="r.legal_basis" class="meta">法律依据：{{ r.legal_basis }}</div>
        </li>
      </ul>
      <template v-if="report.structured && report.facets">
        <h3>分布统计</h3>
        <div v-for="fe in facetEntries(report.facets)" :key="fe.name" class="facet">
          <b>{{ fe.name }}</b>：
          <span v-for="b in fe.buckets" :key="fe.name + '-' + b.key" class="bucket">{{ b.key }}（{{ b.doc_count }}）</span>
        </div>
        <p class="law-notice">{{ report.disclaimer }}</p>
      </template>
    </template>

    <h3>本案类案留痕（引用，非事实）</h3>
    <p class="hint">类案要旨不进事实五态；跨案件天然隔离。</p>
    <ul class="caselist">
      <li v-for="r in refs" :key="r.ref_id">
        <b>{{ r.case_name || r.source_case_id }}</b>
        <span class="meta">{{ r.court }} · {{ r.cause }} · {{ r.judgment_date }}</span>
        <div class="summary">{{ r.summary }}</div>
      </li>
      <li v-if="refs.length === 0" class="hint">（暂无留痕）</li>
    </ul>
  </section>
</template>

<style scoped>
.casesearch input { padding: 6px 8px; }
.casesearch input.num { width: 70px; }
.err { color: #b91c1c; font-size: 13px; }
.warn { color: #b45309; }
.caselist { font-size: 13px; padding-left: 18px; }
.caselist li { margin: 8px 0; }
.meta { color: #6b7280; font-size: 12px; margin-left: 8px; }
.summary { margin-top: 2px; }
.facet { font-size: 13px; margin: 4px 0; }
.bucket { margin-right: 8px; color: #374151; }
.law-notice { background: #fef3c7; border: 1px solid #fbbf24; padding: 8px 10px; font-size: 13px; border-radius: 6px; }
</style>
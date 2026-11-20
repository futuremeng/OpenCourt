<script setup lang="ts">
// 审校与报告——自 App.vue 抽出（distill）。
// 审校为确定性检查，模型不参与判定；原样导出须显式勾选确认并留审计。
import { ref } from "vue";
import { errText } from "../util";
import * as App from "../../wailsjs/go/main/App";

const props = defineProps<{ names: string[] }>();

const reviewReport = ref<any | null>(null);
const reviewBusy = ref(false);
const exportPath = ref("");
const exportErr = ref("");
const rawConfirm = ref(false);

function reviewErrClear() {
  exportErr.value = "";
  exportPath.value = "";
}

async function runReview() {
  reviewErrClear();
  reviewBusy.value = true;
  try {
    reviewReport.value = await App.ReviewCase();
  } catch (e) {
    exportErr.value = errText(e);
  } finally {
    reviewBusy.value = false;
  }
}

async function exportReport(redacted: boolean) {
  reviewErrClear();
  try {
    const r = await App.ExportReport(redacted, !redacted && rawConfirm.value, props.names);
    exportPath.value = r.path;
  } catch (e) {
    exportErr.value = errText(e);
  }
}

// 案件切换后清空上一案件的报告与导出状态
function refresh() {
  reviewReport.value = null;
  rawConfirm.value = false;
  reviewErrClear();
}

defineExpose({ refresh });
</script>

<template>
  <section class="panel wide">
    <h2>审校与报告</h2>
    <p class="hint">
      审校为确定性检查（引用核验 / 回原文一致性 / 五态合规 / 推理链完整性），模型不参与判定；
      报告默认脱敏导出，原样导出须显式二次确认并留审计；报告全文过出报告前检查。
    </p>
    <div class="row">
      <button class="primary" :disabled="reviewBusy" @click="runReview">{{ reviewBusy ? "审校中…" : "执行审校" }}</button>
      <button @click="exportReport(true)">导出报告（脱敏）</button>
      <label class="hint"><input type="checkbox" v-model="rawConfirm" />我知悉原样导出含未脱敏信息</label>
      <button :disabled="!rawConfirm" @click="exportReport(false)">原样导出</button>
    </div>
    <p v-if="exportErr" class="error" role="status">{{ exportErr }}</p>
    <p v-if="exportPath" class="ok-msg" role="status">已导出：{{ exportPath }}</p>
    <div v-if="reviewReport" role="status">
      <p>标红 <b class="rejected">{{ reviewReport.red }}</b> 项 · 提示 {{ reviewReport.warn }} 项</p>
      <ul class="blindspots">
        <li v-for="(iss, i) in reviewReport.issues" :key="i">
          <b :class="{ rejected: iss.level === 'red' }">{{ iss.level === "red" ? "[标红]" : "[提示]" }}</b>
          {{ iss.message }}（{{ iss.check }}<template v-if="iss.ref"> · {{ iss.ref }}</template>）
        </li>
      </ul>
    </div>
  </section>
</template>

<style scoped>
.hint { color: var(--text-3); font-size: var(--fs-xs); margin: 0 0 10px; }
.row { display: flex; gap: 8px; align-items: center; margin-top: 8px; flex-wrap: wrap; }
.error { color: var(--bad); font-size: var(--fs-sm); }
.ok-msg { color: var(--ok); font-size: var(--fs-sm); }
.rejected { color: var(--bad); }
.blindspots { font-size: var(--fs-sm); padding-left: 18px; }
</style>

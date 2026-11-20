<script setup lang="ts">
// 出报告前检查（旧称「输出门禁自检」）——自 App.vue 抽出（distill）。倾向性表述由后端强制拦截。
import { ref } from "vue";
import { errText } from "../util";
import * as App from "../../wailsjs/go/main/App";

const gateText = ref("");
const gateResult = ref<string[]>([]);
const checking = ref(false);
const error = ref("");

async function checkGate() {
  error.value = "";
  checking.value = true;
  try {
    const v = await App.CheckOutput(gateText.value);
    gateResult.value = v.length === 0 ? ["通过：未检测到引导性表述"] : v.map((x) => `已拦截：${x.message}`);
  } catch (e) {
    error.value = errText(e);
    gateResult.value = [];
  } finally {
    checking.value = false;
  }
}
</script>

<template>
  <section class="panel">
    <h2>出报告前检查</h2>
    <p class="hint">试试输入「建议起诉」或「大概率胜诉」——倾向性表述会被强制拦截。</p>
    <textarea v-model="gateText" rows="2" aria-label="待检文本" placeholder="待检文本" class="full"></textarea>
    <div class="row">
      <button :disabled="checking" @click="checkGate">{{ checking ? "检查中…" : "开始检查" }}</button>
    </div>
    <p v-if="error" class="error" role="status">{{ error }}</p>
    <ul v-else class="gate-result" role="status">
      <li v-for="m in gateResult" :key="m">{{ m }}</li>
    </ul>
  </section>
</template>

<style scoped>
.hint { color: var(--text-3); font-size: var(--fs-xs); margin: 0 0 10px; }
.full { width: 100%; }
.row { display: flex; gap: 8px; align-items: center; margin-top: 8px; flex-wrap: wrap; }
.gate-result { font-size: var(--fs-sm); padding-left: 18px; }
.error { color: var(--bad); font-size: var(--fs-sm); }
</style>

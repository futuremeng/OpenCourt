<script setup lang="ts">
// 输出门禁自检——自 App.vue 抽出（distill）。倾向性表述由后端强制拦截。
import { ref } from "vue";
import * as App from "../../wailsjs/go/main/App";

const gateText = ref("");
const gateResult = ref<string[]>([]);

async function checkGate() {
  const v = await App.CheckOutput(gateText.value);
  gateResult.value = v.length === 0 ? ["通过：未检测到引导性表述"] : v.map((x) => `[拦截] ${x.message}`);
}
</script>

<template>
  <section class="panel">
    <h2>输出门禁自检</h2>
    <p class="hint">试试输入「建议起诉」或「大概率胜诉」——倾向性表述会被强制拦截。</p>
    <textarea v-model="gateText" rows="2" aria-label="待检文本" placeholder="待检文本" class="full"></textarea>
    <div class="row">
      <button @click="checkGate">门禁检查</button>
    </div>
    <ul class="gate-result" role="status">
      <li v-for="m in gateResult" :key="m">{{ m }}</li>
    </ul>
  </section>
</template>

<style scoped>
.hint { color: var(--text-3); font-size: var(--fs-xs); margin: 0 0 10px; }
.full { width: 100%; }
.row { display: flex; gap: 8px; align-items: center; margin-top: 8px; flex-wrap: wrap; }
.gate-result { font-size: var(--fs-sm); padding-left: 18px; }
</style>

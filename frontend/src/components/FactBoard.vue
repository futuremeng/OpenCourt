<script setup lang="ts">
import { ref } from "vue";
import * as App from "../../wailsjs/go/main/App";
import type { Fact } from "../../wailsjs/go/main/App";

const props = defineProps<{ facts: Fact[] }>();
const emit = defineEmits<{ (e: "changed"): void }>();

const stateNames: Record<string, string> = {
  TRACED_CONFIRMED: "已溯源确认",
  SINGLE_SOURCE: "单源未补强",
  OPPONENT_CLAIM: "对方主张未验证",
  CONFLICTED: "存在冲突",
  UNSUPPORTED: "无依据",
};

const msg = ref("");

// 升级候选提交给 Go 侧，由 statemachine 确定性仲裁；前端无权直接改状态。
async function tryConfirm(f: Fact) {
  msg.value = "";
  try {
    await App.UpgradeFact(f.fact_id, "TRACED_CONFIRMED", true, true, true);
    msg.value = `事实 ${f.fact_id} 已按证据驱动规则升级。`;
    emit("changed");
  } catch (e) {
    msg.value = `[状态机拒绝] ${e}`;
  }
}

// 演示违规路径：不导入证据直接要求升格，必然被代码仲裁拒绝。
async function tryIllegalUpgrade(f: Fact) {
  msg.value = "";
  try {
    await App.UpgradeFact(f.fact_id, "TRACED_CONFIRMED", false, false, true);
    msg.value = "异常：非法升级未被拦截";
  } catch (e) {
    msg.value = `[状态机拒绝] ${e}`;
  }
}
</script>

<template>
  <div class="board">
    <div v-for="state in Object.keys(stateNames)" :key="state" class="column" :class="state.toLowerCase()">
      <h3>{{ stateNames[state] }}（{{ facts.filter((x) => x.state === state).length }}）</h3>
      <div v-for="f in facts.filter((x) => x.state === state)" :key="f.fact_id" class="card">
        <p class="desc">{{ f.description }}</p>
        <p v-if="f.citation" class="cite">
          📄 {{ f.citation.source_file }} 第 {{ f.citation.page_num }} 页：「{{ f.citation.snippet }}」
        </p>
        <div class="actions" v-if="state !== 'TRACED_CONFIRMED'">
          <button @click="tryConfirm(f)">证据驱动升级</button>
          <button @click="tryIllegalUpgrade(f)">无证据升级（应被拒绝）</button>
        </div>
      </div>
      <p v-if="facts.filter((x) => x.state === state).length === 0" class="empty">暂无</p>
    </div>
  </div>
  <p v-if="msg" class="msg">{{ msg }}</p>
</template>

<style scoped>
.board { display: grid; grid-template-columns: repeat(5, 1fr); gap: 10px; }
.column { border-radius: 8px; padding: 10px; min-height: 120px; border: 1px solid #e5e7eb; }
.column h3 { margin: 0 0 8px; font-size: 13px; }
.card { background: #fff; border: 1px solid #e5e7eb; border-radius: 6px; padding: 8px; margin-bottom: 8px; }
.desc { margin: 0 0 6px; font-size: 13px; }
.cite { font-size: 11px; color: #6b7280; margin: 0 0 6px; }
.actions { display: flex; gap: 6px; flex-wrap: wrap; }
.actions button { font-size: 11px; padding: 3px 8px; }
.empty { color: #9ca3af; font-size: 12px; }
.traced_confirmed { background: #ecfdf5; }
.single_source { background: #eff6ff; }
.opponent_claim { background: #fffbeb; }
.conflicted { background: #fef2f2; }
.unsupported { background: #f3f4f6; }
.msg { font-size: 13px; color: #374151; }
</style>

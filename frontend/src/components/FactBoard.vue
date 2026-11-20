<script setup lang="ts">
import { computed, ref } from "vue";
import * as App from "../../wailsjs/go/main/App";
import type { domain } from "../../wailsjs/go/models";
import { FACT_STATE_NAMES } from "../util";

const props = defineProps<{ facts: domain.Fact[] }>();
const emit = defineEmits<{ (e: "changed"): void }>();

const stateNames = FACT_STATE_NAMES;

// 按五态一次分组（optimize：避免模板内反复 filter）
const grouped = computed<Record<string, domain.Fact[]>>(() => {
  const map: Record<string, domain.Fact[]> = {};
  for (const s of Object.keys(stateNames)) map[s] = [];
  for (const f of props.facts) {
    if (!map[f.state]) map[f.state] = [];
    map[f.state].push(f);
  }
  return map;
});

const msg = ref("");

// 升级候选提交给 Go 侧，由 statemachine 确定性仲裁；前端无权直接改状态。
async function tryConfirm(f: domain.Fact) {
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
async function tryIllegalUpgrade(f: domain.Fact) {
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
    <div v-for="(label, state) in stateNames" :key="state" class="column" :class="String(state).toLowerCase()">
      <h3>{{ label }}（{{ grouped[state]?.length ?? 0 }}）</h3>
      <div v-for="f in grouped[state]" :key="f.fact_id" class="card">
        <p class="desc">{{ f.description }}</p>
        <p v-if="f.citation" class="cite">
          <span aria-hidden="true">📄</span> {{ f.citation.SourceFile }} 第 {{ f.citation.PageNum }} 页：「{{ f.citation.Snippet }}」
        </p>
        <div class="actions" v-if="state !== 'TRACED_CONFIRMED'">
          <button @click="tryConfirm(f)">证据驱动升级</button>
          <button @click="tryIllegalUpgrade(f)">无证据升级（应被拒绝）</button>
        </div>
      </div>
      <p v-if="(grouped[state]?.length ?? 0) === 0" class="empty">暂无</p>
    </div>
  </div>
  <p v-if="msg" class="msg" role="status">{{ msg }}</p>
</template>

<style scoped>
.board { display: grid; grid-template-columns: repeat(auto-fit, minmax(180px, 1fr)); gap: 10px; }
.column { border-radius: var(--r-2); padding: 10px; min-height: 120px; border: 1px solid var(--border); }
.column h3 { margin: 0 0 8px; font-size: var(--fs-sm); }
.card { background: var(--surface); border: 1px solid var(--border); border-radius: var(--r-1); padding: 8px; margin-bottom: 8px; }
.desc { margin: 0 0 6px; font-size: var(--fs-sm); }
.cite { font-size: var(--fs-xs); color: var(--text-3); margin: 0 0 6px; }
.actions { display: flex; gap: 6px; flex-wrap: wrap; }
.actions button { font-size: var(--fs-xs); padding: 4px 10px; min-height: 28px; }
.empty { color: var(--text-3); font-size: var(--fs-xs); }
.traced_confirmed { background: var(--state-confirmed); }
.single_source { background: var(--state-single); }
.opponent_claim { background: var(--state-opponent); }
.conflicted { background: var(--state-conflicted); }
.unsupported { background: var(--state-unsupported); }
.msg { font-size: var(--fs-sm); color: var(--text-2); }
</style>


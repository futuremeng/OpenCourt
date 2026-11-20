<script setup lang="ts">
import { computed, ref } from "vue";
import * as App from "../../wailsjs/go/main/App";
import type { domain } from "../../wailsjs/go/models";
import { baseName, errHelp, errText, FACT_STATE_NAMES, FACT_STATE_HINTS } from "../util";

const props = defineProps<{ facts: domain.Fact[] }>();
const emit = defineEmits<{ (e: "changed"): void; (e: "goto", kind: "locate" | "ingest"): void }>();

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
// 失败回执的补救落点（v0.4 P12）：按错误正文补救语出按钮，未匹配不出按钮
const msgHelp = ref<"locate" | "ingest" | null>(null);

// 升级候选提交给 Go 侧，由状态规则确定性判定；前端无权直接改状态。
async function tryConfirm(f: domain.Fact) {
  msg.value = "";
  msgHelp.value = null;
  try {
    await App.UpgradeFact(f.fact_id, "TRACED_CONFIRMED", true, true, true);
    msg.value = `这条事实已按证据规则升级为「${FACT_STATE_NAMES.TRACED_CONFIRMED}」（${FACT_STATE_HINTS.TRACED_CONFIRMED}）。`;
    emit("changed");
  } catch (e) {
    msg.value = errText(e);
    msgHelp.value = errHelp(msg.value);
  }
}
</script>

<template>
  <div class="board">
    <div v-for="(label, state) in stateNames" :key="state" class="column" :class="String(state).toLowerCase()">
      <h3>{{ label }}（{{ grouped[state]?.length ?? 0 }}）</h3>
      <p class="sub">{{ FACT_STATE_HINTS[state] }}</p>
      <div v-for="f in grouped[state]" :key="f.fact_id" class="card">
        <p class="desc">{{ f.description }}</p>
        <p v-if="f.citation" class="cite">
          <span aria-hidden="true">📄</span> {{ baseName(f.citation.SourceFile) }} 第 {{ f.citation.PageNum }} 页：「{{ f.citation.Snippet }}」
        </p>
        <p class="fid">编号 {{ f.fact_id }}</p>
        <div class="actions" v-if="state !== 'TRACED_CONFIRMED'">
          <button @click="tryConfirm(f)">按证据升级</button>
        </div>
      </div>
      <p v-if="(grouped[state]?.length ?? 0) === 0" class="empty">暂无</p>
    </div>
  </div>
  <p v-if="msg" class="msg" role="status">
    {{ msg }}
    <button v-if="msgHelp" class="msg-link" @click="emit('goto', msgHelp)">
      {{ msgHelp === "locate" ? "去卷宗检索核对" : "去接入证据文书" }}
    </button>
  </p>
</template>

<style scoped>
.board { display: grid; grid-template-columns: repeat(auto-fit, minmax(180px, 1fr)); gap: 10px; }
.column { border-radius: var(--r-2); padding: 10px; min-height: 120px; border: 1px solid var(--border); }
.column h3 { margin: 0 0 2px; font-size: var(--fs-sm); }
.sub { margin: 0 0 8px; font-size: var(--fs-xs); color: var(--text-3); }
.card { background: var(--surface); border: 1px solid var(--border); border-radius: var(--r-1); padding: 8px; margin-bottom: 8px; }
.desc { margin: 0 0 6px; font-size: var(--fs-sm); }
.cite { font-size: var(--fs-xs); color: var(--text-3); margin: 0 0 4px; }
.fid { font-size: var(--fs-xs); color: var(--text-3); margin: 0 0 6px; }
.actions { display: flex; gap: 6px; flex-wrap: wrap; }
.actions button { font-size: var(--fs-xs); padding: 4px 10px; min-height: 28px; }
.empty { color: var(--text-3); font-size: var(--fs-xs); }
.traced_confirmed { background: var(--state-confirmed); }
.single_source { background: var(--state-single); }
.opponent_claim { background: var(--state-opponent); }
.conflicted { background: var(--state-conflicted); }
.unsupported { background: var(--state-unsupported); }
.msg { font-size: var(--fs-sm); color: var(--text-2); }
.msg-link { margin-left: 8px; font-size: var(--fs-xs); padding: 2px 8px; min-height: 24px; }
</style>


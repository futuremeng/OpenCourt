<script setup lang="ts">
// SimPanel 对抗推演与庭审流程面板（v0.3 M6，roadmap-next「推演」面板：
// 轮次视图 + 轨迹抖动告警）。纪律：
//   - 全程显示「模拟推演，非预测」免责标注（后端固定下发，不由前端拼装）；
//   - 阶段推进由后端 core/flow 硬守卫，前端只呈现拦截原因，不做旁路；
//   - 被门禁拦截的轮次同样出现在轮次视图（拦截必须可见）；
//   - 轨迹抖动待复核时后端拒绝续演，此处只提供 SimReview 唯一恢复出口。
import { onMounted, ref } from "vue";
import * as App from "../../wailsjs/go/main/App";
import type { neutral, storage } from "../../wailsjs/go/models";

const PHASES: { code: string; label: string }[] = [
  { code: "STATEMENT", label: "法庭调查·陈述" },
  { code: "EVIDENCE", label: "举证质证" },
  { code: "DEBATE", label: "法庭辩论" },
  { code: "FINAL", label: "最后陈述" },
];

const status = ref<Record<string, any> | null>(null);
const rounds = ref<storage.SimRound[]>([]);
const issues = ref<neutral.IssueReport | null>(null);
const phase = ref("STATEMENT");
const stance = ref("");
const conclusion = ref("");
const factIDs = ref("");
const running = ref(false);
const lastResult = ref<any | null>(null);
const error = ref("");
const notice = ref("");

const disclaimer = () => status.value?.disclaimer || "这是模拟推演，不是审判结果预测；结论仅供参考";

async function refresh() {
  error.value = "";
  try {
    status.value = await App.SimStatus();
    rounds.value = await App.ListSimRounds();
  } catch (e) {
    error.value = String(e);
  }
}

async function runRound() {
  error.value = ""; notice.value = ""; lastResult.value = null;
  const ids = factIDs.value.split(/[,，\s]+/).map((s) => s.trim()).filter(Boolean);
  if (!conclusion.value.trim()) { error.value = "本轮发言不能为空"; return; }
  if (ids.length === 0) { error.value = "必须填写 fact_ids：不带来源的发言会被门禁拦截"; return; }
  running.value = true;
  try {
    lastResult.value = await App.RunSimRound(phase.value, stance.value, conclusion.value, ids);
    await refresh();
  } catch (e) {
    error.value = String(e); // 阶段守卫/待复核拦截：原因原样呈现，不旁路
  } finally {
    running.value = false;
  }
}

async function review() {
  error.value = "";
  try {
    await App.SimReview();
    notice.value = "已人工复核确认，恢复自动续演";
    await refresh();
  } catch (e) {
    error.value = String(e);
  }
}

async function summarize() {
  error.value = "";
  try {
    issues.value = await App.SummarizeCaseIssues();
  } catch (e) {
    error.value = String(e);
  }
}

function doneOf(code: string): number {
  return status.value?.done?.[code] ?? 0;
}
function phaseLabel(code: string): string {
  return PHASES.find((p) => p.code === code)?.label || code;
}

onMounted(refresh);
defineExpose({ refresh });
</script>

<template>
  <div class="simpanel">
    <div v-if="status" class="disclaimer-bar">{{ disclaimer() }}</div>

    <div v-if="status && !status.tool_enabled" class="warnbox">
      当前立场（{{ status.role }}）已禁用对抗推演（defense_rehearsal）——法院/当事人 Profile 不开放本功能。
    </div>

    <div class="phases">
      <div v-for="p in PHASES" :key="p.code" class="phase" :class="{ cur: phase === p.code, done: doneOf(p.code) > 0 }">
        <b>{{ p.label }}</b>
        <span class="muted">{{ doneOf(p.code) }} 轮有效</span>
      </div>
    </div>
    <div v-if="status && !status.evidence_ready" class="warnbox">
      事实库尚无任何「有证据支撑」状态的事实：无证据不进质证（阶段推进会被硬守卫拒绝）。
    </div>
    <div v-if="status && status.needs_review" class="reviewbox">
      轨迹抖动且无新增证据，自动续演已停止：{{ status.review_note }}
      <button class="sm" @click="review">人工复核确认</button>
    </div>
    <div v-if="status && !status.model_enabled" class="warnbox">
      模型未启用：对方模拟不可用（不会用模板冒充「AI 模拟对方」），发言门禁与流程守卫仍正常工作。
    </div>

    <div class="row">
      <select v-model="phase" aria-label="庭审阶段">
        <option v-for="p in PHASES" :key="p.code" :value="p.code">{{ p.label }}（{{ p.code }}）</option>
      </select>
      <input v-model="stance" aria-label="本方立场" placeholder="本方立场（每轮重新注入并做漂移比对）" class="stance" />
    </div>
    <div class="row">
      <input v-model="factIDs" aria-label="fact_ids（逗号分隔，必填）" placeholder="fact_ids（逗号分隔，必填，如 FACT-1, FACT-2）" class="facts" />
    </div>
    <div class="row">
      <textarea v-model="conclusion" aria-label="本轮发言" placeholder="本轮发言（必须可落地到上述卷宗事实）" rows="3" class="concl"></textarea>
      <button class="primary" :disabled="running || !(status && status.tool_enabled)" @click="runRound">
        {{ running ? "推演中…" : "发起本轮推演" }}
      </button>
      <button :disabled="running" @click="summarize">争点归纳（中立）</button>
    </div>
    <div v-if="error" class="err" role="status">{{ error }}</div>
    <div v-if="notice" class="muted" role="status">{{ notice }}</div>

    <div v-if="lastResult" class="result">
      <div class="muted">轮次 {{ lastResult.round_id }}（{{ phaseLabel(lastResult.phase) }}）</div>
      <div v-if="lastResult.blocked" class="badbox">
        本轮发言被门禁拦截，不得展示为有效轮次（已留痕，待改写）：
        <ul>
          <li v-for="(v, i) in lastResult.violations || []" :key="i">[{{ v.rule }}] {{ v.message }}</li>
        </ul>
      </div>
      <div v-if="lastResult.drift" class="badbox">⚠ {{ lastResult.drift_note }}</div>
      <div v-if="lastResult.needs_review" class="reviewbox">
        {{ lastResult.review_note }}
        <button class="sm" @click="review">人工复核确认</button>
      </div>
      <ul v-if="(lastResult.attacks || []).length" class="attacks">
        <li v-for="(a, i) in lastResult.attacks" :key="i" :class="{ blocked: a.blocked }">
          <template v-if="a.blocked">[拦截待改写] {{ a.text }}</template>
          <template v-else>{{ a.text }}<span v-if="a.fact_ids && a.fact_ids.length" class="cite">（{{ a.fact_ids.join("、") }}）</span></template>
        </li>
      </ul>
      <div v-else-if="!lastResult.blocked" class="muted">对方未产生攻击点候选（模型未启用或无可见材料）</div>
    </div>

    <div v-if="issues" class="issues">
      <h3>争点归纳</h3>
      <div class="muted">{{ issues.note }}</div>
      <ul>
        <li v-for="(it, i) in issues.issues" :key="i">
          <b>{{ it.element }}</b>
          <span class="muted">（{{ (it.fact_ids || []).join("、") }}）</span>
          <span v-if="it.has_opponent" class="badge warn">含对方主张</span>
          <ul v-if="(it.conflict_points || []).length">
            <li v-for="(cp, j) in it.conflict_points" :key="j" class="muted">{{ cp }}（并列展示，不代作取舍）</li>
          </ul>
        </li>
      </ul>
      <div v-if="(issues.gaps || []).length" class="gaps">证据缺口：{{ issues.gaps.join("、") }}（不自动补全）</div>
    </div>

    <h3>轮次轨迹（含被拦截轮——拦截必须可追溯）</h3>
    <table class="tbl">
      <thead><tr><th scope="col">轮次</th><th scope="col">阶段</th><th scope="col">立场</th><th scope="col">发言</th><th scope="col">状态</th></tr></thead>
      <tbody>
        <tr v-for="r in rounds" :key="r.round_id" :class="{ blocked: r.blocked }">
          <td>{{ r.round_id }}</td>
          <td>{{ phaseLabel(r.phase) }}</td>
          <td>{{ r.role }}</td>
          <td class="clip">{{ r.text }}</td>
          <td><span class="badge" :class="r.blocked ? 'bad' : 'ok'">{{ r.blocked ? "已拦截" : "有效" }}</span></td>
        </tr>
        <tr v-if="rounds.length === 0"><td colspan="5" class="muted">尚无推演轮次</td></tr>
      </tbody>
    </table>
  </div>
</template>

<style scoped>
.simpanel { display: flex; flex-direction: column; gap: 10px; }
.disclaimer-bar {
  background: var(--warn-bg); border: 1px solid var(--warn-border); border-radius: var(--r-2);
  padding: 6px 12px; font-size: var(--fs-sm); color: var(--warn);
}
.warnbox { background: var(--bad-bg); border: 1px solid var(--bad-border); border-radius: var(--r-2); padding: 6px 12px; font-size: var(--fs-sm); color: var(--bad); }
.reviewbox { background: var(--warn-bg); border: 1px solid var(--warn-border); border-radius: var(--r-2); padding: 6px 12px; font-size: var(--fs-sm); color: var(--warn); display: flex; gap: 10px; align-items: center; }
.badbox { background: var(--bad-bg); border: 1px solid var(--bad-border); border-radius: var(--r-2); padding: 6px 12px; font-size: var(--fs-sm); color: var(--bad); }
.phases { display: flex; gap: 8px; flex-wrap: wrap; }
.phase { border: 1px solid var(--border); border-radius: var(--r-2); padding: 6px 12px; font-size: var(--fs-sm); display: flex; flex-direction: column; gap: 2px; }
.phase.cur { border-color: var(--accent); background: var(--accent-soft); }
.phase.done { border-color: var(--ok); }
.row { display: flex; align-items: flex-start; gap: 8px; flex-wrap: wrap; }
input.stance { width: 320px; }
input.facts { width: 360px; }
textarea.concl { flex: 1; min-width: 320px; }
.err { color: var(--bad); font-size: var(--fs-sm); }
.muted { color: var(--text-3); font-size: var(--fs-xs); }
.result { border-top: 1px dashed var(--border); padding-top: 8px; display: flex; flex-direction: column; gap: 6px; }
.attacks { font-size: var(--fs-sm); padding-left: 18px; margin: 0; }
.attacks li { margin: 4px 0; }
.attacks li.blocked { background: var(--bad-bg); border: 1px solid var(--bad-border); border-radius: var(--r-1); padding: 4px 8px; list-style: none; color: var(--bad); }
.cite { color: var(--text-3); font-size: var(--fs-xs); }
h3 { font-size: var(--fs-md); margin: 8px 0 4px; color: var(--text-2); }
.issues ul { margin: 4px 0 0 18px; font-size: var(--fs-sm); }
.gaps { font-size: var(--fs-sm); color: var(--warn); margin-top: 6px; }
.tbl { width: 100%; border-collapse: collapse; font-size: var(--fs-sm); }
.tbl th, .tbl td { text-align: left; padding: 6px 8px; border-bottom: 1px solid var(--border); vertical-align: top; }
.tbl th { color: var(--text-3); font-weight: 500; }
.tbl tr.blocked td { color: var(--bad); }
.clip { max-width: 420px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.badge { padding: 1px 8px; border-radius: var(--r-pill); font-size: var(--fs-xs); background: var(--accent-soft); color: var(--accent-ink); }
.badge.ok { background: var(--ok-bg); color: var(--ok); }
.badge.warn { background: var(--warn-bg); color: var(--warn); }
.badge.bad { background: var(--bad-bg); color: var(--bad); }
button.sm { padding: 3px 10px; font-size: var(--fs-xs); }
</style>


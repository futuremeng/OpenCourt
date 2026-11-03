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
      <select v-model="phase">
        <option v-for="p in PHASES" :key="p.code" :value="p.code">{{ p.label }}（{{ p.code }}）</option>
      </select>
      <input v-model="stance" placeholder="本方立场（每轮重新注入并做漂移比对）" class="stance" />
    </div>
    <div class="row">
      <input v-model="factIDs" placeholder="fact_ids（逗号分隔，必填，如 FACT-1, FACT-2）" class="facts" />
    </div>
    <div class="row">
      <textarea v-model="conclusion" placeholder="本轮发言（必须可落地到上述卷宗事实）" rows="3" class="concl"></textarea>
      <button class="primary" :disabled="running || !(status && status.tool_enabled)" @click="runRound">
        {{ running ? "推演中…" : "发起本轮推演" }}
      </button>
      <button :disabled="running" @click="summarize">争点归纳（中立）</button>
    </div>
    <div v-if="error" class="err">{{ error }}</div>
    <div v-if="notice" class="muted">{{ notice }}</div>

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
      <thead><tr><th>轮次</th><th>阶段</th><th>立场</th><th>发言</th><th>状态</th></tr></thead>
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
  background: #fffbeb; border: 1px solid #fde68a; border-radius: 8px;
  padding: 6px 12px; font-size: 12.5px; color: #92400e;
}
.warnbox { background: #fef2f2; border: 1px solid #fecaca; border-radius: 8px; padding: 6px 12px; font-size: 13px; color: #b91c1c; }
.reviewbox { background: #fff7ed; border: 1px solid #fdba74; border-radius: 8px; padding: 6px 12px; font-size: 13px; color: #9a3412; display: flex; gap: 10px; align-items: center; }
.badbox { background: #fef2f2; border: 1px solid #fecaca; border-radius: 8px; padding: 6px 12px; font-size: 13px; color: #b91c1c; }
.phases { display: flex; gap: 8px; flex-wrap: wrap; }
.phase { border: 1px solid #e5e7eb; border-radius: 8px; padding: 6px 12px; font-size: 13px; display: flex; flex-direction: column; gap: 2px; }
.phase.cur { border-color: #4338ca; background: #eef2ff; }
.phase.done { border-color: #065f46; }
.row { display: flex; align-items: flex-start; gap: 8px; flex-wrap: wrap; }
input.stance { width: 320px; }
input.facts { width: 360px; }
textarea.concl { flex: 1; min-width: 320px; }
.err { color: #b91c1c; font-size: 13px; }
.muted { color: #6b7280; font-size: 12.5px; }
.result { border-top: 1px dashed #e5e7eb; padding-top: 8px; display: flex; flex-direction: column; gap: 6px; }
.attacks { font-size: 13px; padding-left: 18px; margin: 0; }
.attacks li { margin: 4px 0; }
.attacks li.blocked { background: #fef2f2; border: 1px solid #fecaca; border-radius: 6px; padding: 4px 8px; list-style: none; color: #b91c1c; }
.cite { color: #6b7280; font-size: 12px; }
h3 { font-size: 13.5px; margin: 8px 0 4px; color: #374151; }
.issues ul { margin: 4px 0 0 18px; font-size: 13px; }
.gaps { font-size: 13px; color: #92400e; margin-top: 6px; }
.tbl { width: 100%; border-collapse: collapse; font-size: 13px; }
.tbl th, .tbl td { text-align: left; padding: 6px 8px; border-bottom: 1px solid #e5e7eb; vertical-align: top; }
.tbl th { color: #6b7280; font-weight: 500; }
.tbl tr.blocked td { color: #b91c1c; }
.clip { max-width: 420px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.badge { padding: 1px 8px; border-radius: 10px; font-size: 12px; background: #eef2ff; color: #3730a3; }
.badge.ok { background: #ecfdf5; color: #047857; }
.badge.warn { background: #fffbeb; color: #b45309; }
.badge.bad { background: #fef2f2; color: #b91c1c; }
button.sm { padding: 2px 10px; font-size: 12px; }
</style>


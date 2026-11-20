<script setup lang="ts">
// 网络查阅与立案辅助（BrowserOS）——自 App.vue 抽出（distill）。默认禁用；
// 抓回内容先脱敏；登录/验证码/实名/最终提交一律交人执行。
import { ref } from "vue";
import { baseName, errText, FACT_STATE_NAMES, ruleLabel } from "../util";
import * as App from "../../wailsjs/go/main/App";

defineProps<{ enabled: boolean }>();
const names = defineModel<string>("names", { default: "" });
const emit = defineEmits<{ (e: "changed"): void }>();

const webUrl = ref("");
const webBusy = ref(false);
const webResult = ref<any | null>(null);
const webErr = ref("");

const govCat = ref("");
const govPages = ref(20);
const govStart = ref(1);
const govBusy = ref(false);
const govResult = ref<any | null>(null);
const govErr = ref("");

const assistUrl = ref("");
const assistPage = ref<any | null>(null);
const assistValues = ref("");
const assistBusy = ref(false);
const assistCands = ref<any[]>([]);
const assistReceipts = ref<any[]>([]);
const assistErr = ref("");
const humanKind = ref("login");
const humanReason = ref("需要当事人本人登录/实名");
const humanStatus = ref("");

function nameList(): string[] {
  return names.value.split(/[,，]/).map((s) => s.trim()).filter(Boolean);
}

async function runWebResearch() {
  webErr.value = ""; webResult.value = null; webBusy.value = true;
  try {
    webResult.value = await App.ResearchWebPage(webUrl.value.trim(), nameList());
    emit("changed");
  } catch (e) {
    webErr.value = errText(e);
  } finally {
    webBusy.value = false;
  }
}

async function runGovListBrowse() {
  govErr.value = ""; govResult.value = null; govBusy.value = true;
  try {
    govResult.value = await App.BrowseGovList(govCat.value, govPages.value, govStart.value);
  } catch (e) {
    govErr.value = errText(e);
  } finally {
    govBusy.value = false;
  }
}

async function openAssist() {
  assistErr.value = ""; assistCands.value = []; assistReceipts.value = [];
  assistPage.value = null; humanStatus.value = "";
  try {
    assistPage.value = await App.BrowserOpenPage(assistUrl.value.trim());
  } catch (e) {
    assistErr.value = errText(e);
  }
}

// parseValues 每行「字段名=值」；值尾缀 ? 表示该值未经确认（弱态，UI 标黄）。
function parseValues(): [Record<string, string>, Record<string, string>] {
  const values: Record<string, string> = {};
  const states: Record<string, string> = {};
  for (const line of assistValues.value.split("\n")) {
    const t = line.trim();
    const i = t.indexOf("=");
    if (!t || i <= 0) continue;
    const name = t.slice(0, i).trim();
    let val = t.slice(i + 1).trim();
    if (val.endsWith("?")) {
      val = val.slice(0, -1);
      states[name] = "待用户确认";
    }
    if (name && val) { values[name] = val; }
  }
  return [values, states];
}

async function planFill() {
  if (!assistPage.value) { assistErr.value = "请先打开页面"; return; }
  assistErr.value = ""; assistBusy.value = true;
  try {
    const [values, states] = parseValues();
    assistCands.value = await App.BrowserPlanFill(assistPage.value.page_id, values, states);
  } catch (e) {
    assistErr.value = errText(e);
  } finally {
    assistBusy.value = false;
  }
}

async function execAction(id: string) {
  assistErr.value = "";
  try {
    const rec = await App.BrowserExecuteAction(id, nameList());
    assistReceipts.value = [rec, ...assistReceipts.value];
    assistCands.value = assistCands.value.filter((c) => c.action_id !== id);
  } catch (e) {
    assistErr.value = errText(e);
  }
}

async function handToHuman() {
  if (!assistPage.value) { assistErr.value = "请先打开页面"; return; }
  assistErr.value = "";
  try {
    const st = await App.BrowserHandToHuman(assistPage.value.page_id, humanKind.value, humanReason.value);
    humanStatus.value = st.status;
  } catch (e) {
    assistErr.value = errText(e);
  }
}

async function awaitHuman() {
  assistErr.value = "";
  try {
    const st = await App.BrowserAwaitHuman();
    humanStatus.value = st.status;
  } catch (e) {
    assistErr.value = errText(e);
  }
}
</script>

<template>
  <section class="panel wide browseros">
    <h2>网络查阅与立案辅助</h2>
    <p class="hint">
      默认禁用；启用后由本机自装的外网检索组件 BrowserOS neo（外部可选组件，不随本应用分发）代为浏览。
      抓回内容先隐去姓名等敏感信息再展示；产生的事实只按「单源未补强」入库；
      登录/验证码/实名/最终提交一律由人亲自完成——工具只准备候选填写内容，绝不代做最终动作。
    </p>
    <div v-if="!enabled" class="notice">
      外网检索未启用：启动 BrowserOS neo 后，在其侧栏的 MCP 页复制端点地址，
      写入配置 <b>browseros.endpoint</b>（或环境变量 OPENCOURT_BROWSEROS_ENDPOINT）并将
      <b>browseros.enabled</b> 置 true。本工具不会回退到其他抓取方式。
    </div>

    <h3>网络资料查阅</h3>
    <div class="row">
      <input v-model="webUrl" aria-label="要查阅的 URL" placeholder="URL（如法院诉讼费标准页面）" class="grow" />
      <input v-model="names" aria-label="当事人姓名（脱敏用）" placeholder="当事人姓名（逗号分隔，脱敏用）" class="names" />
      <button class="primary" :disabled="webBusy" @click="runWebResearch">{{ webBusy ? "查阅中…" : "查阅" }}</button>
    </div>
    <p v-if="webErr" class="error" role="status">{{ webErr }}</p>
    <div v-if="webResult" class="web-result" role="status">
      <p class="ok-msg">
        内容指纹 {{ String(webResult.content_sha256 || "").slice(0, 12) }} ·
        隐去敏感信息 {{ (webResult.redact_actions || []).length }} 处 ·
        事实可靠程度 <b>{{ FACT_STATE_NAMES[webResult.fact.state] || webResult.fact.state }}</b>（来自网络的内容永不计入最高档）
      </p>
      <p class="cite">来源：{{ webResult.fact.citation ? baseName(webResult.fact.citation.SourceFile) + ' 第 ' + webResult.fact.citation.PageNum + ' 页' : '无' }}</p>
      <pre class="web-text">{{ webResult.text }}</pre>
    </div>

    <h3 class="spaced">国家规章库清单浏览（人在环代理浏览）</h3>
    <div class="row">
      <select v-model="govCat" aria-label="规章类别" class="govcat">
        <option value="">全部（混合清单）</option>
        <option value="部门规章">仅部门规章</option>
        <option value="地方政府规章">仅地方政府规章</option>
      </select>
      <input v-model.number="govStart" type="number" min="1" class="num-sm" aria-label="起始页"
        title="起始页（跨会话续接：上次停在第 N 页，这里填 N+1）" />
      <input v-model.number="govPages" type="number" min="1" max="200" class="num" aria-label="单会话翻页上限"
        title="单会话翻页上限（≤200）" />
      <button class="primary" :disabled="govBusy" @click="runGovListBrowse">{{ govBusy ? "浏览中…" : "开始浏览" }}</button>
    </div>
    <p class="hint">
      在你自己的 BrowserOS neo 中浏览国家规章库官方列表页（官方页面翻页，每页间隔≥2秒，单次≤200 页，全程留记录），
      导出与 OpenLaw 兼容的清单文件；随后由 openlaw-govcn -source gzk 走详情页合规抓取 + 人工确认闸口入库。
      绝不触碰检索 API 或鉴权接口。
    </p>
    <p v-if="govErr" class="error" role="status">{{ govErr }}</p>
    <p v-if="govResult" class="ok-msg" role="status">
      浏览 {{ govResult.pages }} 页 · 收集 {{ (govResult.entries || []).length }} 条（{{ govResult.stopped_at }}）→ {{ govResult.out_path }}
    </p>

    <h3 class="spaced">立案表单辅助（两段式：候选 → 逐条确认 → 执行）</h3>
    <div class="row">
      <input v-model="assistUrl" aria-label="目标页面 URL" placeholder="目标页面 URL（如 zxfw.court.gov.cn 立案页）" class="grow" />
      <button @click="openAssist">打开页面</button>
      <span v-if="assistPage" class="ok-msg" role="status">已在你的浏览器中打开该页面</span>
    </div>
    <textarea v-model="assistValues" rows="3" class="full" aria-label="填写字段（字段名=值，每行一条）"
      placeholder="字段名=值（每行一条；值尾缀 ? 表示未经确认将标黄，如：原告姓名=王五?）"></textarea>
    <div class="row">
      <button :disabled="assistBusy || !assistPage" @click="planFill">{{ assistBusy ? "规划中…" : "生成填写计划（只产候选，不执行）" }}</button>
    </div>
    <p v-if="assistErr" class="error" role="status">{{ assistErr }}</p>
    <ul class="cands">
      <li v-for="c in assistCands" :key="c.action_id" :class="{ blocked: !c.allowed }">
        <b>{{ c.label }}</b> ← {{ c.value }}
        <template v-if="!c.allowed">
          <span class="block-tag">[已拦截：{{ ruleLabel(c.rule) }}] {{ c.reason }}</span>
        </template>
        <template v-else>
          <span v-if="c.fact_state" class="weak-tag">[{{ c.fact_state }}]</span>
          <button class="sm" @click="execAction(c.action_id)">确认执行</button>
        </template>
      </li>
    </ul>
    <div class="row">
      <select v-model="humanKind" aria-label="交人接管类型">
        <option value="login">登录</option>
        <option value="captcha">验证码</option>
        <option value="identity">实名/人脸</option>
        <option value="submit">最终提交</option>
      </select>
      <input v-model="humanReason" aria-label="交人接管原因" placeholder="交人接管原因" class="grow" />
      <button @click="handToHuman">交人接管</button>
      <button @click="awaitHuman">等待接管结果</button>
      <span v-if="humanStatus" class="ok-msg" role="status">接管状态: {{ humanStatus }}</span>
    </div>
    <ul v-if="assistReceipts.length" class="receipts" role="status">
      <li v-for="r in assistReceipts" :key="r.action_id"><b>{{ r.action_id }}</b> {{ r.diff }}</li>
    </ul>
  </section>
</template>


<style scoped>
.hint { color: var(--text-3); font-size: var(--fs-xs); margin: 0 0 10px; }
.notice {
  background: var(--warn-bg); border: 1px solid var(--warn-border); border-radius: var(--r-2);
  padding: 8px 12px; font-size: var(--fs-sm); margin-bottom: 4px;
}
.row { display: flex; gap: 8px; align-items: center; margin-top: 8px; flex-wrap: wrap; }
.grow { flex: 1; }
.full { width: 100%; }
.names { width: 220px; }
.num { width: 90px; }
.num-sm { width: 70px; }
.govcat { width: 150px; }
.error { color: var(--bad); font-size: var(--fs-sm); }
.ok-msg { color: var(--ok); font-size: var(--fs-sm); }
h3 { font-size: var(--fs-md); margin: 10px 0 4px; color: var(--text-2); }
h3.spaced { margin-top: 12px; }
.web-result { background: var(--info-bg); border: 1px solid var(--info-border); border-radius: var(--r-2); padding: 8px 12px; margin-top: 8px; font-size: var(--fs-sm); }
.web-result .cite { color: var(--text-3); font-size: var(--fs-xs); word-break: break-all; }
.web-text { white-space: pre-wrap; word-break: break-word; font-size: var(--fs-sm); max-height: 260px; overflow-y: auto; background: var(--surface); border-radius: var(--r-1); padding: 8px; }
.cands { font-size: var(--fs-sm); padding-left: 18px; margin: 8px 0; }
.cands li { margin: 5px 0; }
.cands li.blocked { background: var(--bad-bg); border: 1px solid var(--bad-border); border-radius: var(--r-1); padding: 4px 8px; list-style: none; }
.block-tag { color: var(--bad); font-size: var(--fs-xs); margin-left: 6px; }
.weak-tag { color: var(--warn); background: var(--warn-bg); border-radius: 4px; padding: 1px 6px; font-size: var(--fs-xs); margin-left: 6px; }
.receipts { font-size: var(--fs-xs); color: var(--text-2); padding-left: 18px; }
.receipts li { margin: 3px 0; white-space: pre-wrap; word-break: break-word; }
</style>



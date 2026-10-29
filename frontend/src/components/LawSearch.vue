<script setup lang="ts">
// LawSearch 法条检索与文书引用核验（v0.2 M2，plan-v0.2 4.1）。
// 纪律：全部经后端绑定走 OpenLaw sdk；核验判定 = OpenLaw 错误码原样
// （OK / OL-1001..OL-1006）；UNKNOWN（效力待确认）与 pending（待人工确认）
// 必须显式标出——不得静默通过；断链（OL-1006）标「无法溯源」。
import { onMounted, ref } from "vue";
import * as App from "../../wailsjs/go/main/App";

const query = ref("");
const hits = ref<any[]>([]);
const searched = ref(false);
const searching = ref(false);

const docs = ref<any[]>([]);
const docID = ref("");
const atDate = ref("");
const verifying = ref(false);
const refs = ref<any[]>([]);
const relRefID = ref("");
const relations = ref<any[]>([]);
const error = ref("");
const notice = ref("");

async function search() {
  error.value = "";
  searching.value = true;
  try {
    hits.value = await App.SearchLaw(query.value.trim(), 10);
    searched.value = true;
  } catch (e) {
    error.value = String(e);
  } finally {
    searching.value = false;
  }
}

async function verify() {
  if (!docID.value) {
    error.value = "请选择要核验的文书";
    return;
  }
  error.value = "";
  notice.value = "";
  verifying.value = true;
  try {
    refs.value = await App.VerifyDocLawCitations(docID.value, atDate.value);
    if (refs.value.length === 0) {
      notice.value = "文书中未抽取到「《法名》第X条」形态的引用";
    }
  } catch (e) {
    error.value = String(e);
  } finally {
    verifying.value = false;
  }
}

async function showRelations(refID: string) {
  error.value = "";
  relations.value = [];
  relRefID.value = refID;
  try {
    relations.value = await App.GetLawRelations(refID);
  } catch (e) {
    error.value = String(e);
  }
}

async function refresh() {
  hits.value = [];
  searched.value = false;
  relations.value = [];
  relRefID.value = "";
  try {
    docs.value = await App.ListDocuments();
    refs.value = await App.ListLawRefs();
  } catch (e) {
    error.value = String(e);
  }
}

function statusClass(s: string): string {
  if (s === "EFFECTIVE") return "ok";
  if (s === "AMENDED") return "warn";
  if (s === "UNVERIFIED" || s === "REPEALED") return "bad";
  return "warn"; // UNKNOWN：效力状态待确认
}
function statusLabel(s: string): string {
  return (
    { EFFECTIVE: "现行有效", AMENDED: "已被修改", REPEALED: "已废止", UNVERIFIED: "未核验", UNKNOWN: "效力待确认" }[s] || s
  );
}
function verifyClass(v: string): string {
  return v === "OK" ? "ok" : "bad";
}
function verifyLabel(v: string): string {
  if (!v) return "未核验";
  if (v === "OK") return "核验通过";
  return {
    "OL-1001": "OL-1001 未找到法规",
    "OL-1002": "OL-1002 当日无生效版本",
    "OL-1003": "OL-1003 条款不存在",
    "OL-1004": "OL-1004 引用与原文不一致",
    "OL-1005": "OL-1005 版本已失效",
    "OL-1006": "OL-1006 无法溯源",
  }[v] || v;
}

onMounted(refresh);
defineExpose({ refresh });
</script>

<template>
  <div class="lawsearch">
    <div class="row">
      <input v-model="query" placeholder="法条关键词（如：违约责任）" class="q" @keyup.enter="search" />
      <button :disabled="searching" @click="search">{{ searching ? "检索中…" : "检索法条" }}</button>
      <span class="spacer"></span>
      <select v-model="docID">
        <option value="">选择文书…</option>
        <option v-for="d in docs" :key="d.doc_id" :value="d.doc_id">{{ d.file_name || d.doc_id }}</option>
      </select>
      <input v-model="atDate" type="date" title="行为发生时点（版本回溯基准，可留空）" class="date" />
      <button :disabled="verifying" @click="verify">{{ verifying ? "核验中…" : "核验文书引用" }}</button>
    </div>
    <div v-if="error" class="err">{{ error }}</div>
    <div v-if="notice" class="muted">{{ notice }}</div>

    <table v-if="searched" class="tbl">
      <thead>
        <tr><th>法规</th><th>条文</th><th>生效日期</th><th>效力状态</th><th>文本确认</th></tr>
      </thead>
      <tbody>
        <tr v-for="h in hits" :key="h.law_id">
          <td>{{ h.title }}</td>
          <td class="clip">{{ h.article }}</td>
          <td>{{ h.effective_date }}</td>
          <td><span class="badge" :class="statusClass(h.status)">{{ statusLabel(h.status) }}</span></td>
          <td>
            <span class="badge" :class="h.confirm_status === 'confirmed' ? 'ok' : 'warn'">
              {{ h.confirm_status === "confirmed" ? "已确认" : "待人工确认" }}
            </span>
          </td>
        </tr>
        <tr v-if="hits.length === 0"><td colspan="5" class="muted">无命中：核对检索词，或确认语料范围（无结果不是错误）</td></tr>
      </tbody>
    </table>

    <table v-if="refs.length" class="tbl">
      <thead>
        <tr><th>引用（本案留痕）</th><th>核验判定</th><th>效力状态</th><th>文本确认</th><th></th></tr>
      </thead>
      <tbody>
        <tr v-for="r in refs" :key="r.ref_id">
          <td>《{{ r.law_name }}》{{ r.article_no }}<template v-if="r.at_date"> @{{ r.at_date }}</template></td>
          <td><span class="badge" :class="verifyClass(r.verify_result)">{{ verifyLabel(r.verify_result) }}</span></td>
          <td><span class="badge" :class="statusClass(r.status)">{{ statusLabel(r.status) }}</span></td>
          <td>
            <span class="badge" :class="r.confirm_status === 'confirmed' ? 'ok' : 'warn'">
              {{ r.confirm_status === "confirmed" ? "已确认" : "待人工确认" }}
            </span>
          </td>
          <td><button class="sm" @click="showRelations(r.ref_id)">关系</button></td>
        </tr>
      </tbody>
    </table>

    <div v-if="relRefID" class="rels">
      <span class="muted">引用 {{ relRefID }} 的法条关系（修改/废止/解释/配套；机器不自动判定废止）：</span>
      <ul v-if="relations.length">
        <li v-for="(rel, i) in relations" :key="i">
          {{ rel.src_law_id }} —{{ rel.relation_type }}→ {{ rel.dst_law_id }}
          <span class="badge" :class="rel.confirm_status === 'confirmed' ? 'ok' : 'warn'">
            {{ rel.confirm_status === "confirmed" ? "已确认" : "待人工确认" }}
          </span>
        </li>
      </ul>
      <span v-else-if="!error" class="muted">无已录入关系</span>
    </div>
  </div>
</template>

<style scoped>
.lawsearch { display: flex; flex-direction: column; gap: 10px; }
.row { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
input.q { width: 260px; }
input.date { width: 150px; }
.spacer { flex: 1; }
.err { color: #b91c1c; font-size: 13px; }
.tbl { width: 100%; border-collapse: collapse; font-size: 13px; }
.tbl th, .tbl td { text-align: left; padding: 6px 8px; border-bottom: 1px solid #e5e7eb; vertical-align: top; }
.tbl th { color: #6b7280; font-weight: 500; }
.clip { max-width: 360px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.badge { padding: 1px 8px; border-radius: 10px; font-size: 12px; background: #eef2ff; color: #3730a3; }
.badge.ok { background: #ecfdf5; color: #047857; }
.badge.warn { background: #fffbeb; color: #b45309; }
.badge.bad { background: #fef2f2; color: #b91c1c; }
.rels ul { margin: 6px 0 0 18px; font-size: 13px; }
</style>
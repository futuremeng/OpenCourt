<script setup lang="ts">
// CaseBar 案件切换器（v0.2 M1）：显示当前案件 + 切换 + 新建。
// 案件是一切事实/冲突/文档/要素的边界（docs/plan-v0.2.md D1）；
// 案由为自由文本，不自动判定；阶段必须是枚举码。
import { onMounted, ref } from "vue";
import { CASE_STAGE_NAMES, errText, stageLabel } from "../util";
import * as App from "../../wailsjs/go/main/App";

const emit = defineEmits<{ (e: "switched"): void }>();

const cases = ref<any[]>([]);
const current = ref<any | null>(null);
const title = ref("");
const caseType = ref("");
const stage = ref("CONSULT");
const error = ref("");
const creating = ref(false);

// 阶段名唯一来源在 util.CASE_STAGE_NAMES（与总览筛选共用）；顺序即枚举顺序
const stages = Object.entries(CASE_STAGE_NAMES).map(([code, label]) => ({ code, label }));

async function refresh() {
  try {
    cases.value = await App.ListCases();
    current.value = await App.CurrentCase();
  } catch (e) {
    error.value = errText(e);
  }
}

async function switchCase(id: string) {
  error.value = "";
  try {
    await App.SwitchCase(id);
    await refresh();
    emit("switched");
  } catch (e) {
    error.value = errText(e);
  }
}

async function createCase() {
  if (!title.value.trim()) {
    error.value = "案件标题不能为空";
    return;
  }
  error.value = "";
  creating.value = true;
  try {
    await App.CreateCase(title.value.trim(), caseType.value.trim(), stage.value);
    title.value = "";
    caseType.value = "";
    await refresh();
    emit("switched");
  } catch (e) {
    error.value = errText(e);
  } finally {
    creating.value = false;
  }
}

onMounted(refresh);

// 全局快捷键 Cmd+N / 菜单「文件→新建案件…」的落点：聚焦新案件标题输入框
const titleInput = ref<HTMLInputElement | null>(null);
function focusNewCase() {
  titleInput.value?.focus();
  titleInput.value?.select();
}

defineExpose({ refresh, focusNewCase });
</script>

<template>
  <div class="casebar">
    <span v-if="current" class="current" :title="'案件编号（核对用）：' + current.case_id">
      当前案件：<b>{{ current.title }}</b>
      <span class="meta">
        <template v-if="current.case_type">{{ current.case_type }} · </template>{{ stageLabel(current.stage) }}
      </span>
    </span>
    <select
      v-if="cases.length > 1"
      :value="current?.case_id"
      aria-label="切换案件"
      @change="switchCase(($event.target as HTMLSelectElement).value)"
    >
      <option v-for="c in cases" :key="c.case_id" :value="c.case_id">{{ c.title }}</option>
    </select>
    <span class="spacer"></span>
    <input ref="titleInput" v-model="title" aria-label="新案件标题" placeholder="新案件标题" class="title" />
    <input v-model="caseType" aria-label="案由/类型（可选）" placeholder="案由/类型（可选）" class="type" />
    <select v-model="stage" aria-label="案件阶段">
      <option v-for="s in stages" :key="s.code" :value="s.code">{{ s.label }}</option>
    </select>
    <button :disabled="creating" @click="createCase">{{ creating ? "创建中…" : "新建案件" }}</button>
    <span v-if="error" class="err" role="status">{{ error }}</span>
  </div>
</template>

<style scoped>
.casebar {
  display: flex; align-items: center; gap: 10px; flex-wrap: wrap;
  padding: 10px 20px; background: var(--surface); border-bottom: 1px solid var(--border);
  font-size: var(--fs-sm);
}
.current b { font-size: var(--fs-md); }
.meta { color: var(--text-3); margin-left: 6px; }
.spacer { flex: 1; }
input.title { width: 180px; }
input.type { width: 140px; }
.err { color: var(--bad); }
</style>

<script setup lang="ts">
// CaseBar 案件切换器（v0.2 M1）：显示当前案件 + 切换 + 新建。
// 案件是一切事实/冲突/文档/要素的边界（docs/plan-v0.2.md D1）；
// 案由为自由文本，不自动判定；阶段必须是枚举码。
import { onMounted, ref } from "vue";
import * as App from "../../wailsjs/go/main/App";

const emit = defineEmits<{ (e: "switched"): void }>();

const cases = ref<any[]>([]);
const current = ref<any | null>(null);
const title = ref("");
const caseType = ref("");
const stage = ref("CONSULT");
const error = ref("");
const creating = ref(false);

const stages = [
  { code: "CONSULT", label: "咨询" },
  { code: "FILING", label: "起诉" },
  { code: "FIRST_TRIAL", label: "一审" },
  { code: "APPEAL", label: "上诉" },
  { code: "SECOND_TRIAL", label: "二审" },
];

function stageLabel(code: string): string {
  return stages.find((s) => s.code === code)?.label || code;
}

async function refresh() {
  try {
    cases.value = await App.ListCases();
    current.value = await App.CurrentCase();
  } catch (e) {
    error.value = String(e);
  }
}

async function switchCase(id: string) {
  error.value = "";
  try {
    await App.SwitchCase(id);
    await refresh();
    emit("switched");
  } catch (e) {
    error.value = String(e);
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
    error.value = String(e);
  } finally {
    creating.value = false;
  }
}

onMounted(refresh);
defineExpose({ refresh });
</script>

<template>
  <div class="casebar">
    <span v-if="current" class="current">
      当前案件：<b>{{ current.title }}</b>
      <span class="meta">
        {{ current.case_id }}<template v-if="current.case_type"> · {{ current.case_type }}</template>
        · {{ stageLabel(current.stage) }}
      </span>
    </span>
    <select
      v-if="cases.length > 1"
      :value="current?.case_id"
      @change="switchCase(($event.target as HTMLSelectElement).value)"
    >
      <option v-for="c in cases" :key="c.case_id" :value="c.case_id">{{ c.title }}</option>
    </select>
    <span class="spacer"></span>
    <input v-model="title" placeholder="新案件标题" class="title" />
    <input v-model="caseType" placeholder="案由/类型（可选）" class="type" />
    <select v-model="stage">
      <option v-for="s in stages" :key="s.code" :value="s.code">{{ s.label }}</option>
    </select>
    <button :disabled="creating" @click="createCase">{{ creating ? "创建中…" : "新建案件" }}</button>
    <span v-if="error" class="err">{{ error }}</span>
  </div>
</template>

<style scoped>
.casebar {
  display: flex; align-items: center; gap: 10px; flex-wrap: wrap;
  padding: 10px 20px; background: #fff; border-bottom: 1px solid #e5e7eb;
  font-size: 13px;
}
.current b { font-size: 14px; }
.meta { color: #6b7280; margin-left: 6px; }
.spacer { flex: 1; }
input.title { width: 180px; }
input.type { width: 140px; }
.err { color: #b91c1c; }
</style>

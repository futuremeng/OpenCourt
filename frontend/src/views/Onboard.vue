<script setup lang="ts">
// 首屏向导（v0.4 P1）：按 Profile 声明的 guide 数据渲染，代码不判断 role；
// 未声明 guide 的 Profile 不会走到这里（Workspace 条件渲染）。
// 步骤完成状态一律来自父组件传入的真实计数；本组件只展示与导航，不自称完成。
import EmptyState from "../components/EmptyState.vue";

defineProps<{
  steps: { title: string; text: string; action: string }[];
  status: Record<string, { done: boolean; detail: string }>;
}>();

const emit = defineEmits<{ (e: "action", action: string): void }>();

// action → 按钮文案；不在表里的 action 不给按钮（缺入口就如实缺入口）
const actionLabels: Record<string, string> = {
  "new-case": "新建案件",
  "import-doc": "选择卷宗文件",
  document: "进入阅卷与事实板",
  report: "进入审校与报告",
};
function label(action: string): string | null {
  return actionLabels[action] ?? null;
}
</script>

<template>
  <section class="guide" aria-label="引导动线">
    <div class="guide-head">
      <h2>开始使用</h2>
      <span class="guide-note">按当前使用角色的动线推进；完成状态来自实际数据，不是勾选</span>
    </div>
    <ol class="steps">
      <li v-for="(s, i) in steps" :key="s.action || i" class="step">
        <div class="step-top">
          <span class="step-no" aria-hidden="true">{{ i + 1 }}</span>
          <h3>{{ s.title }}</h3>
          <span class="badge" :class="status[s.action]?.done ? 'ok' : 'pending'">
            {{ status[s.action]?.done ? "已完成" : "待完成" }}
          </span>
        </div>
        <p class="step-text">{{ s.text }}</p>
        <p v-if="status[s.action]?.done" class="step-detail" role="status">{{ status[s.action].detail }}</p>
        <EmptyState
          v-else
          :text="status[s.action]?.detail || '尚未开始'"
          :hint="label(s.action) ? null : '该步骤的入口在当前版本的工作台中'"
        >
          <button v-if="label(s.action)" class="btn sm" @click="emit('action', s.action)">
            {{ label(s.action) }}
          </button>
        </EmptyState>
      </li>
    </ol>
  </section>
</template>

<style scoped>
.guide { margin: 0 0 16px; border: 1px solid var(--border); border-radius: var(--r-3);
  background: var(--surface); padding: 14px 16px; }
.guide-head { display: flex; align-items: baseline; gap: 12px; flex-wrap: wrap; }
.guide-head h2 { margin: 0; font-size: var(--fs-md); }
.guide-note { font-size: var(--fs-xs); color: var(--text-3); }
.steps { list-style: none; margin: 12px 0 0; padding: 0;
  display: grid; grid-template-columns: repeat(auto-fit, minmax(260px, 1fr)); gap: 12px; }
.step { border: 1px solid var(--border); border-radius: var(--r-3); padding: 12px; }
.step-top { display: flex; align-items: center; gap: 8px; }
.step-top h3 { margin: 0; font-size: var(--fs-sm); flex: 1; }
.step-no { width: 22px; height: 22px; border-radius: 50%; background: var(--accent); color: var(--accent-ink);
  display: inline-flex; align-items: center; justify-content: center; font-size: var(--fs-xs); }
.badge { border-radius: var(--r-pill); padding: 2px 10px; font-size: var(--fs-xs); }
.badge.ok { background: var(--ok); }
.badge.pending { background: var(--warn); }
.step-text { margin: 8px 0 0; font-size: var(--fs-xs); color: var(--text-2); }
.step-detail { margin: 8px 0 0; font-size: var(--fs-xs); color: var(--text-3); }
</style>

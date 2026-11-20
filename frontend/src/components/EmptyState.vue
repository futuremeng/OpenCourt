<script setup lang="ts">
// 通用空状态（v0.4 P1）：一步做完前，明确说清「现在没有什么」与「下一步做什么」。
// 纪律：只陈述事实与可达动作，动作缺失时不给按钮（不得编一个去处）。
defineProps<{ text: string; hint?: string | null }>();
</script>

<template>
  <div class="empty-state">
    <p class="empty-text">{{ text }}</p>
    <p v-if="hint" class="empty-hint">{{ hint }}</p>
    <slot />
  </div>
</template>

<style scoped>
.empty-state {
  border: 1px dashed var(--border); border-radius: var(--r-3);
  padding: 12px 14px; margin-top: 10px; background: var(--surface);
}
.empty-text { margin: 0; font-size: var(--fs-sm); color: var(--text-2); }
.empty-hint { margin: 6px 0 0; font-size: var(--fs-xs); color: var(--text-3); }
.empty-state :slotted(button) { margin-top: 10px; }
</style>

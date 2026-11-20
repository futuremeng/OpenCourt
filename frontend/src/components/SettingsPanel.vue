<script setup lang="ts">
// 设置 / 关于面板（桌面设计规范：Cmd+, 设置入口；关于呈现免责标语）。
// 模态呈现但仅用于「查看+打开数据目录」这类轻任务，Esc 关闭。
import { ref } from "vue";
import * as App from "../../wailsjs/go/main/App";
import { profileLabel, redactLabel, toolLabel } from "../util";

defineProps<{ mode: "settings" | "about"; info: Record<string, any> | null }>();
const emit = defineEmits<{ (e: "close"): void }>();

const openingDir = ref(false);
async function openDataDir() {
  openingDir.value = true;
  try {
    await App.OpenDataDir();
  } catch {
    /* 数据目录打开失败不打断面板 */
  } finally {
    openingDir.value = false;
  }
}
</script>

<template>
  <div class="overlay" @click.self="emit('close')">
    <div class="sheet" role="dialog" aria-modal="true"
         :aria-label="mode === 'about' ? '关于 OpenCourt' : '设置'">
      <header class="sheet-head">
        <h2>{{ mode === "about" ? "关于 OpenCourt" : "设置" }}</h2>
        <button class="sm" aria-label="关闭面板" @click="emit('close')">关闭</button>
      </header>

      <template v-if="mode === 'about'">
        <p class="about-name">OpenCourt <span class="hint">v{{ info?.version }}</span></p>
        <p class="hint">面向法律场景的中立信息整理与逻辑推演辅助（AI 辅助性质，不替代裁判与专业判断）。</p>
        <p class="hint">OpenCourt 为 AI 辅助工具：不预测判决结果、不引导决策、不替代律师与法官判断。
          最终裁判权与诉讼决策权归属人类。</p>
      </template>

      <dl v-else class="kv">
        <dt>版本</dt><dd>{{ info?.version }}</dd>
        <dt>使用角色</dt><dd>{{ profileLabel(info?.profile) }}</dd>
        <dt>数据目录</dt>
        <dd>
          <code>{{ info?.dataDir }}</code>
          <button class="sm" :disabled="openingDir" @click="openDataDir">打开</button>
        </dd>
        <dt>配置文件</dt><dd class="hint">数据目录下 settings.yaml（编辑后重启生效）；开发模式亦可用仓库 config/settings.yaml</dd>
        <dt>脱敏级别</dt><dd>{{ redactLabel(info?.redactLevel || "") }}</dd>
        <dt>使用统计</dt><dd>{{ info?.telemetry ? "开启" : "关闭" }}</dd>
        <dt>智能模型</dt><dd>{{ info?.modelEnabled ? "已启用" : "离线" }}</dd>
        <dt>外网检索</dt><dd>{{ info?.browserEnabled ? "已启用" : "未启用" }}</dd>
        <dt>已禁用的功能</dt><dd>{{ (info?.disabledTools || []).map((t: string) => toolLabel(t)).join("、") || "无" }}</dd>
      </dl>
    </div>
  </div>
</template>

<style scoped>
.overlay {
  position: fixed; inset: 0; background: rgba(0, 0, 0, 0.4);
  display: flex; align-items: center; justify-content: center; z-index: 50;
}
.sheet {
  background: var(--surface); border: 1px solid var(--border); border-radius: var(--r-3);
  padding: 18px 22px; min-width: 460px; max-width: 640px; max-height: 80vh; overflow: auto;
  box-shadow: 0 16px 48px rgba(0, 0, 0, 0.25);
}
.sheet-head { display: flex; align-items: center; justify-content: space-between; margin-bottom: 10px; }
.sheet-head h2 { margin: 0; font-size: var(--fs-lg); }
.about-name { font-size: var(--fs-xl); font-weight: 600; margin: 6px 0; }
.kv { display: grid; grid-template-columns: 96px 1fr; gap: 6px 12px; margin: 0; font-size: var(--fs-sm); }
.kv dt { color: var(--text-3); }
.kv dd { margin: 0; color: var(--text-1); }
.kv dd button { margin-left: 8px; }
.kv code { font-size: var(--fs-xs); word-break: break-all; }
.hint { color: var(--text-3); font-size: var(--fs-xs); }
</style>

<script setup lang="ts">
import { onMounted, ref } from "vue";
import * as App from "../wailsjs/go/main/App";
import type { AppInfo, Fact } from "../wailsjs/go/main/App";
import FactBoard from "./components/FactBoard.vue";

const info = ref<AppInfo | null>(null);
const facts = ref<Fact[]>([]);
const desc = ref("");
const sourceFile = ref("");
const pageNum = ref(1);
const snippet = ref("");
const fromOpponent = ref(false);
const gateText = ref("");
const gateResult = ref<string[]>([]);
const error = ref("");

async function refresh() {
  facts.value = await App.ListFacts();
}

onMounted(async () => {
  try {
    info.value = await App.AppInfo();
    await refresh();
  } catch (e) {
    error.value = String(e);
  }
});

async function addFact() {
  error.value = "";
  try {
    await App.CreateFact(desc.value, sourceFile.value, pageNum.value, snippet.value, fromOpponent.value);
    desc.value = ""; snippet.value = "";
    await refresh();
  } catch (e) {
    error.value = String(e);
  }
}

async function checkGate() {
  const v = await App.CheckOutput(gateText.value);
  gateResult.value = v.length === 0 ? ["通过：未检测到引导性表述"] : v.map((x) => `[拦截] ${x.message}`);
}
</script>

<template>
  <header class="topbar">
    <h1>⚖️ OpenCourt</h1>
    <div v-if="info" class="badges">
      <span class="badge">v{{ info.version }}</span>
      <span class="badge profile">Profile: {{ info.profile }}</span>
      <span class="badge" :class="info.telemetry ? 'warn' : 'ok'">
        遥测 {{ info.telemetry ? "开启" : "关闭" }}
      </span>
      <span class="badge" :class="info.modelEnabled ? 'warn' : 'ok'">
        模型 {{ info.modelEnabled ? "已启用" : "离线" }}
      </span>
      <span class="badge">脱敏级别: {{ info.redactLevel }}</span>
    </div>
  </header>

  <main class="layout">
    <section class="panel">
      <h2>录入事实</h2>
      <p class="hint">初始状态由状态机判定：有引用→单源未补强；对方来源→对方主张；无来源→无依据。</p>
      <textarea v-model="desc" rows="2" placeholder="事实描述（必填）" style="width:100%"></textarea>
      <div class="row">
        <input v-model="sourceFile" placeholder="来源文件（可空）" />
        <input v-model.number="pageNum" type="number" min="1" style="width:80px" />
        <input v-model="snippet" placeholder="原文片段" />
      </div>
      <div class="row">
        <label><input type="checkbox" v-model="fromOpponent" /> 来自对方文书/陈述</label>
        <button @click="addFact">录入</button>
      </div>
      <p v-if="error" class="error">{{ error }}</p>
    </section>

    <section class="panel">
      <h2>输出门禁自检</h2>
      <p class="hint">试试输入「建议起诉」或「大概率胜诉」——倾向性表述会被强制拦截。</p>
      <textarea v-model="gateText" rows="2" placeholder="待检文本" style="width:100%"></textarea>
      <button @click="checkGate">门禁检查</button>
      <ul class="gate-result">
        <li v-for="m in gateResult" :key="m">{{ m }}</li>
      </ul>
    </section>

    <section class="panel wide">
      <h2>事实看板（五态）</h2>
      <FactBoard :facts="facts" @changed="refresh" />
    </section>
  </main>

  <footer class="disclaimer">
    OpenCourt 为 AI 辅助工具：不预测判决结果、不引导决策、不替代律师与法官判断。最终裁判权与诉讼决策权归属人类。
  </footer>
</template>

<style scoped>
.topbar {
  display: flex; align-items: center; justify-content: space-between;
  padding: 14px 24px; background: #111827; color: #fff;
}
.topbar h1 { font-size: 18px; margin: 0; }
.badges { display: flex; gap: 8px; flex-wrap: wrap; }
.badge { background: #374151; border-radius: 999px; padding: 3px 12px; font-size: 12px; }
.badge.profile { background: #4338ca; }
.badge.ok { background: #065f46; }
.badge.warn { background: #92400e; }
.layout { display: grid; grid-template-columns: 1fr 1fr; gap: 16px; padding: 20px 24px; }
.panel { background: #fff; border: 1px solid #e5e7eb; border-radius: 10px; padding: 16px; }
.panel.wide { grid-column: 1 / -1; }
.panel h2 { margin: 0 0 8px; font-size: 15px; }
.hint { color: #6b7280; font-size: 12px; margin: 0 0 10px; }
.row { display: flex; gap: 8px; align-items: center; margin-top: 8px; flex-wrap: wrap; }
.error { color: #b91c1c; font-size: 13px; }
.gate-result { font-size: 13px; padding-left: 18px; }
.disclaimer {
  padding: 12px 24px; font-size: 12px; color: #6b7280;
  border-top: 1px solid #e5e7eb; background: #fff;
}
</style>

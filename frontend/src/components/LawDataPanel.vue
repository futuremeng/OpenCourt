<script setup lang="ts">
// 法条数据管理（语料安装 / 证据层补装 / 增量更新包）——自 App.vue 抽出（distill）。
import { onMounted, ref } from "vue";
import * as App from "../../wailsjs/go/main/App";
import { EventsOn } from "../../wailsjs/runtime/runtime";
import { errText, humanSize } from "../util";

const lawStatus = ref<any | null>(null);
const lawMsgs = ref<string[]>([]);
const lawBusy = ref(false);
const lawPkgs = ref<any[]>([]);
const lawChecked = ref<Record<string, boolean>>({});
const lawIndexInfo = ref("");
const lawIndexErr = ref("");
const error = ref("");

async function refreshLawStatus() {
  lawStatus.value = await App.LawDataStatus();
}

async function checkLawUpdates() {
  lawIndexErr.value = "";
  try {
    const res = await App.CheckLawDataUpdates();
    if (!res.ok) {
      lawIndexErr.value = res.error || "无法获取数据包索引";
      return;
    }
    lawIndexInfo.value = res.generated || "";
    lawPkgs.value = res.packages || [];
    const checked: Record<string, boolean> = {};
    for (const p of lawPkgs.value) {
      checked[p.name] = p.action === "install" || p.action === "update";
    }
    lawChecked.value = checked;
  } catch (e) {
    lawIndexErr.value = errText(e);
  }
}

async function downloadSelected() {
  const names = lawPkgs.value.filter((p) => lawChecked.value[p.name]).map((p) => p.name);
  if (names.length === 0) {
    error.value = "请先勾选要下载的数据包";
    return;
  }
  error.value = "";
  lawBusy.value = true;
  lawMsgs.value = [];
  try {
    lawStatus.value = await App.DownloadAndInstallLawData(names);
    await checkLawUpdates();
  } catch (e) {
    error.value = errText(e);
  } finally {
    lawBusy.value = false;
  }
}

const actionLabel: Record<string, string> = {
  install: "安装", update: "更新", optional: "可重装", skip: "暂不适用",
};

async function loadLawDataZip() {
  error.value = "";
  try {
    const path = await App.PickLawDataPackage();
    if (!path) return;
    await doLoadLawData(path);
  } catch (e) {
    error.value = errText(e);
  }
}

async function loadLawDataDir() {
  error.value = "";
  try {
    const path = await App.PickLawDataDir();
    if (!path) return;
    await doLoadLawData(path);
  } catch (e) {
    error.value = errText(e);
  }
}

async function doLoadLawData(path: string) {
  lawBusy.value = true;
  lawMsgs.value = [`已选择: ${path}`];
  try {
    lawStatus.value = await App.LoadLawData(path);
  } catch (e) {
    error.value = errText(e);
  } finally {
    lawBusy.value = false;
  }
}

onMounted(() => {
  EventsOn("lawdata:progress", (m: any) => {
    lawMsgs.value = [...lawMsgs.value.slice(-8), String(Array.isArray(m) ? m[0] : m)];
  });
  refreshLawStatus()
    .then(() => checkLawUpdates())
    .catch((e) => (error.value = errText(e)));
});

defineExpose({ refresh: refreshLawStatus });
</script>

<template>
  <section class="panel wide lawdata">
    <h2>法条数据</h2>
    <div v-if="lawStatus && !lawStatus.installed" class="notice">
      尚未安装离线法条库。可从发布页下载 <b>lawdata-core</b> 数据包（解压到「下载」即会自动安装），
      或直接选择已下载的数据包加载：
    </div>
    <div v-else-if="lawStatus" class="law-stats">
      <span class="badge ok">已安装</span>
      <span class="badge">{{ lawStatus.docs }} 部文档</span>
      <span class="badge">{{ lawStatus.articles }} 条法条</span>
      <span class="badge" :class="lawStatus.evidence_ok ? 'ok' : 'warn'">
        证据层 {{ lawStatus.evidence_ok ? "完整" : "缺失（详情将显示未验证）" }}
      </span>
      <span class="badge" :class="lawStatus.manifest_signed ? 'ok' : 'warn'">
        签名清单 {{ lawStatus.manifest_signed ? "已签名" : "无（无法验证真伪）" }}
      </span>
      <span class="hint wide">语料目录: {{ lawStatus.data_dir }}</span>
      <span class="hint wide">{{ lawStatus.license ? '数据许可: ' + lawStatus.license : '数据许可: 该语料未声明（自行确认来源合法性）' }}</span>
    </div>
    <div class="row">
      <button :disabled="lawBusy" @click="loadLawDataZip">
        {{ lawBusy ? "处理中…" : (lawStatus && lawStatus.installed ? "加载更新数据包（zip）" : "加载数据包（zip）") }}
      </button>
      <button :disabled="lawBusy" @click="loadLawDataDir">选择已解压的数据目录</button>
      <button :disabled="lawBusy" @click="checkLawUpdates">检查可用数据包</button>
    </div>
    <div v-if="lawPkgs.length" class="law-pkgs">
      <p class="hint">发布索引（{{ lawIndexInfo }}）——勾选后一键下载安装（自动校验 sha256 与签名）：</p>
      <label v-for="p in lawPkgs" :key="p.name" class="law-pkg" :class="p.action">
        <input type="checkbox" v-model="lawChecked[p.name]" :disabled="p.action === 'skip' || lawBusy" />
        <b>{{ p.name }}</b>
        <span class="hint">{{ humanSize(p.size) }} · {{ actionLabel[p.action] || p.action }}</span>
      </label>
      <div class="row">
        <button :disabled="lawBusy" class="primary" @click="downloadSelected">
          {{ lawBusy ? "下载中…" : "下载并安装所选包" }}
        </button>
      </div>
    </div>
    <p v-if="lawIndexErr" class="hint" role="status">{{ lawIndexErr }}（离线或未发布时可继续用手动加载）</p>
    <p v-if="error" class="error" role="status">{{ error }}</p>
    <ul v-if="lawMsgs.length" class="law-progress" role="status">
      <li v-for="(m, i) in lawMsgs" :key="i">{{ m }}</li>
    </ul>
  </section>
</template>

<style scoped>
.law-stats { display: flex; gap: 8px; flex-wrap: wrap; align-items: center; margin-bottom: 4px; }
.law-stats .hint.wide { width: 100%; }
.notice {
  background: var(--warn-bg); border: 1px solid var(--warn-border); border-radius: var(--r-2);
  padding: 8px 12px; font-size: var(--fs-sm); margin-bottom: 4px;
}
.law-progress { font-size: var(--fs-xs); color: var(--text-2); padding-left: 18px; margin: 8px 0 0; }
.law-pkgs { margin-top: 10px; border-top: 1px dashed var(--border); padding-top: 8px; }
.law-pkg { display: flex; gap: 8px; align-items: center; font-size: var(--fs-sm); margin: 4px 0; }
.law-pkg.skip { opacity: 0.5; }
.law-pkg.update b { color: var(--warn); }
.badge { background: var(--topbar-badge); color: var(--text-inv); border-radius: var(--r-pill); padding: 3px 12px; font-size: var(--fs-xs); }
.badge.ok { background: var(--ok); }
.badge.warn { background: var(--warn); }
.row { display: flex; gap: 8px; align-items: center; margin-top: 8px; flex-wrap: wrap; }
.hint { color: var(--text-3); font-size: var(--fs-xs); margin: 0 0 10px; }
.error { color: var(--bad); font-size: var(--fs-sm); }
</style>


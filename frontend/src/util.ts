// 跨组件共享的小工具

// 事实五态（产品核心语义，顺序即看板列序）；FactBoard 与总览计数共用，禁止另立
export const FACT_STATE_NAMES: Record<string, string> = {
  TRACED_CONFIRMED: "已溯源确认",
  SINGLE_SOURCE: "单源未补强",
  OPPONENT_CLAIM: "对方主张未验证",
  CONFLICTED: "存在冲突",
  UNSUPPORTED: "无依据",
};

// 五态的通俗注解（双受众：专业名为准，通俗话解释怎么来的）；与上表同序。
export const FACT_STATE_HINTS: Record<string, string> = {
  TRACED_CONFIRMED: "原文核对过，多来源一致，人已确认",
  SINGLE_SOURCE: "有原文出处，但只有一方来源",
  OPPONENT_CLAIM: "对方说的，还没核实",
  CONFLICTED: "不同材料说法互相矛盾",
  UNSUPPORTED: "没有找到来源",
};

// 审校检查项中文显示名（check 标识仅留日志与审计）。
export function checkLabel(c: string): string {
  const m: Record<string, string> = {
    law_citation: "法条引用核验",
    source_locate: "回原文一致性",
    state_compliance: "事实状态合规",
    redaction: "脱敏复查",
    reasoning_chain: "推理链完整性",
  };
  return m[c] || c;
}

// 输出检查拦截规则中文显示名；未收录的如实回退原标识。
export function ruleLabel(r: string): string {
  const m: Record<string, string> = {
    NO_GUIDANCE: "引导性表述",
    HALLUCINATION: "无原文依据的陈述",
    REF_UNVERIFIED: "引用未核验",
    OPPONENT_CLAIM_IN_FACT: "对方主张混入事实",
    CITATION_INVALID: "引用不完整",
    LAW_REPEALED: "引用已废止法条",
    AUTO_SUBMIT_FORBIDDEN: "禁止自动提交",
    SENSITIVE_FIELD_FORBIDDEN: "敏感信息禁止操作",
  };
  return m[r] || r;
}

// 检索命中来源显示名（keyword/vector 为内部命中通道标识）。
export function sourceLabel(s: string): string {
  const m: Record<string, string> = { keyword: "关键词", vector: "语义" };
  return m[s] || s;
}

// 批注类型的中文显示名。
export function kindLabel(k: string): string {
  const m: Record<string, string> = {
    comment: "批注",
    revision_insert: "修订新增",
    revision_delete: "修订删除",
    image: "嵌入图片",
  };
  return m[k] || k;
}

// 脱敏级别中文名（原值可回溯 settings.yaml，界面以中文为主）。
export function redactLabel(l: string): string {
  const m: Record<string, string> = { low: "低", medium: "中", high: "高" };
  return m[l] ? `${m[l]}（${l}）` : l || "未知";
}

// 使用角色的法言法语显示名：取 Profile 声明名，去掉尾缀「 Profile」（yaml 原名为准，不另编）
export function profileLabel(p: string | null | undefined): string {
  if (!p) return "未知";
  return p.replace(/\s*Profile$/i, "").trim() || p;
}

// 工具的法言法语显示名：只收项目文档/面板文案已确证的名字，
// 未确证的如实回退标识符，不编名字；程序标识仅留 Go 侧逻辑。
export function toolLabel(t: string): string {
  const labels: Record<string, string> = {
    defense_rehearsal: "对抗推演",
    verdict_tendency: "裁判倾向分析",
  };
  return labels[t] || t;
}

// 路径→短文件名：列表/下拉只展示文件名，不把长路径搬进界面。
export function baseName(p: string): string {
  return p.split("/").filter(Boolean).pop() || p;
}

// Wails 绑定把 Go 侧错误包成「Error: <原文>」。UI 只展示错误原文（语言规范：
// 技术前缀不进界面），原文一字不改、不另编；剥的只是包裹前缀本身。
export function errText(e: unknown): string {
  const s = e instanceof Error ? e.message : String(e);
  return s.replace(/^Error:\s*/, "");
}

export function humanSize(n: number): string {
  if (n >= 1 << 30) return (n / (1 << 30)).toFixed(1) + "GB";
  if (n >= 1 << 20) return Math.round(n / (1 << 20)) + "MB";
  return n + "B";
}

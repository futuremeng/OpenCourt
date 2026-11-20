// 跨组件共享的小工具

// 事实五态（产品核心语义，顺序即看板列序）；FactBoard 与总览计数共用，禁止另立
export const FACT_STATE_NAMES: Record<string, string> = {
  TRACED_CONFIRMED: "已溯源确认",
  SINGLE_SOURCE: "单源未补强",
  OPPONENT_CLAIM: "对方主张未验证",
  CONFLICTED: "存在冲突",
  UNSUPPORTED: "无依据",
};

// 使用角色的法言法语显示名：取 Profile 声明名，去掉尾缀「 Profile」（yaml 原名为准，不另编）
export function profileLabel(p: string | null | undefined): string {
  if (!p) return "未知";
  return p.replace(/\s*Profile$/i, "").trim() || p;
}

export function humanSize(n: number): string {
  if (n >= 1 << 30) return (n / (1 << 30)).toFixed(1) + "GB";
  if (n >= 1 << 20) return Math.round(n / (1 << 20)) + "MB";
  return n + "B";
}

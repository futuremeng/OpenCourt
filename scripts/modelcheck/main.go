// Package main 真机模型冒烟测试：读取本地 config/settings.yaml 与 .env，
// 通过 OpenAI 兼容接口对样例卷宗执行阅卷 Agent 全链路。
// 用法（项目根目录）：go run ./scripts/modelcheck
// 本脚本不提交任何密钥：Key 只从 .env / 环境变量读取。
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/futuremeng/OpenCourt/config"
	"github.com/futuremeng/OpenCourt/core/domain"
	"github.com/futuremeng/OpenCourt/core/verify"
	"github.com/futuremeng/OpenCourt/neutral"
	"github.com/futuremeng/OpenCourt/plugins"
)

func main() {
	if err := config.LoadDotEnv(".env"); err != nil {
		log.Fatalf(".env 加载失败: %v", err)
	}
	settings, err := config.Load("config/settings.yaml")
	if err != nil {
		log.Fatalf("配置加载失败: %v", err)
	}
	apiKey := ""
	if settings.Model.APIKeyEnv != "" {
		apiKey = os.Getenv(settings.Model.APIKeyEnv)
	}
	adapter := plugins.NewOpenAIAdapter(
		settings.Model.BaseURL, apiKey,
		settings.Model.ReasoningModel, settings.Model.EmbeddingModel,
		settings.Model.Enabled,
	)
	if !adapter.Enabled() {
		log.Fatalf("模型未启用：base_url=%s key 是否注入=%v", settings.Model.BaseURL, apiKey != "")
	}
	fmt.Printf("端点: %s\n模型: %s\n", settings.Model.BaseURL, settings.Model.ReasoningModel)

	doc := &verify.DocumentText{
		File: "sample-judgment.txt",
		Pages: map[int]string{
			1: "2023年5月10日，原告甲向被告乙支付定金50000元。双方签订《设备买卖合同》，约定2023年6月1日前交付X型设备一台，总价200000元。",
			2: "2023年6月20日，甲诉至本院称：乙未按期交货，构成违约，要求双倍返还定金。乙辩称：因上游供应商停产导致无法交货，属于不可归责于己方的原因。",
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	start := time.Now()
	// 冒烟检查无要件库：传 nil（V3——related_element 一律置空）
	report, err := neutral.RunReader(ctx, "DOC-SMOKE", doc, adapter, nil)
	if err != nil {
		log.Fatalf("阅卷失败: %v", err)
	}
	fmt.Printf("耗时: %s\n\n", time.Since(start).Round(time.Second))

	fmt.Printf("== 入库事实 %d 条（状态由核验器+状态机判定）==\n", len(report.Accepted))
	for _, f := range report.Accepted {
		page, snippet := 0, ""
		if f.Citation != nil {
			page, snippet = f.Citation.PageNum, f.Citation.Snippet
		}
		fmt.Printf("  [%s] %s\n      定位: 第%d页「%s」\n", f.State.DisplayName(), f.Description, page, snippet)
	}
	fmt.Printf("\n== 被拒绝候选 %d 条（回原文定位失败 = 幻觉）==\n", len(report.Rejected))
	for _, r := range report.Rejected {
		fmt.Printf("  [拒绝] %s —— 片段「%s」(声称第%d页)\n", r.Description, r.Snippet, r.PageNum)
	}
	fmt.Printf("\n== 确定性要素 %d 个（正则抽取，不经模型）==\n", len(report.Elements))
	for _, e := range report.Elements {
		fmt.Printf("  %s: %s (第%d页)\n", e.Type, e.Value, e.PageNum)
	}

	facts := make([]domain.Fact, 0, len(report.Accepted))
	for _, f := range report.Accepted {
		facts = append(facts, f)
	}
	tl := neutral.BuildTimeline(facts, report.Elements)
	fmt.Printf("\n== 时间线 ==\n")
	for _, item := range tl {
		date := item.Date
		if date == "" {
			date = "日期未定位"
		}
		fmt.Printf("  %s  %s [%s]\n", date, item.Fact.Description, item.Fact.State.DisplayName())
	}
}

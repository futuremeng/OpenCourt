// E0 评测子命令：go run ./scripts/modelcheck eval [-manifest 路径] [-out 报告路径]
// 门禁对抗集永远运行；模型指标按「显式启用才运行」纪律，未启用如实标注。
// 取文走 LocalCases 正常对接通道（LC-8/LC-9/LC-10），本脚本不提交任何密钥。
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/futuremeng/OpenCourt/config"
	"github.com/futuremeng/OpenCourt/eval"
	"github.com/futuremeng/OpenCourt/plugins"
)

func runEval(args []string) {
	fs := flag.NewFlagSet("eval", flag.ExitOnError)
	manifestPath := fs.String("manifest", "tests/eval/corpus_manifest.jsonl",
		"评测集清单 JSONL（缺失 = 只跑门禁对抗集并如实标注）")
	out := fs.String("out", "", "报告输出路径（空 = 打印 stdout）")
	if err := fs.Parse(args); err != nil {
		log.Fatalf("参数错误: %v", err)
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

	var fetch eval.Fetcher
	var probe eval.Prober
	token := ""
	if settings.LocalCases.TokenEnv != "" {
		token = os.Getenv(settings.LocalCases.TokenEnv)
	}
	src := plugins.NewLocalCasesSource(settings.LocalCases.BaseURL, token, settings.LocalCases.Enabled)
	if src.Enabled() {
		fetch = src
		probe = src
	}

	var manifest []eval.ManifestEntry
	if f, err := os.Open(*manifestPath); err == nil {
		manifest, err = eval.LoadManifest(f)
		f.Close()
		if err != nil {
			log.Fatalf("评测清单加载失败: %v", err)
		}
		fmt.Printf("评测清单: %s（%d 条）\n", *manifestPath, len(manifest))
	} else if !os.IsNotExist(err) {
		log.Fatalf("评测清单读取失败: %v", err)
	} else {
		fmt.Printf("评测清单 %s 不存在：本轮只跑门禁对抗集\n", *manifestPath)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Minute)
	defer cancel()
	r := &eval.Runner{Model: adapter, Fetch: fetch, Probe: probe}
	res, err := r.Run(ctx, manifest)
	if err != nil {
		log.Fatalf("评测运行失败: %v", err)
	}
	report := eval.RenderMarkdown(res)
	if *out != "" {
		if err := os.WriteFile(*out, []byte(report), 0o644); err != nil {
			log.Fatalf("报告写入失败: %v", err)
		}
		fmt.Printf("报告已写入 %s\n", *out)
	} else {
		fmt.Print(report)
	}
}

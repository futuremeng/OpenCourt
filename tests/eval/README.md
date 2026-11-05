# tests/eval — E0 评测验收

评测运行方式（设计见 [docs/plan-v0.4.md](../plan-v0.4.md) §3-E0）：

```bash
make eval                          # 离线：门禁对抗集 + 模型指标如实标注「未运行」
go run ./scripts/modelcheck eval -out tests/eval/report-$(date +%F).md
```

## 文件

- `corpus_manifest.jsonl`（从 `corpus_manifest.example.jsonl` 生成）：评测集清单，
  每行一份文书。**生成方式（正常对接调用，不要求评测专用端点）**：

  1. 圈定（LC-9）：对已启用的 LocalCases 发确定性查询——案由/年份过滤 +
     `sort=id` 升序 + `id_gt` 游标翻页，取前 N 条；
  2. 取文（LC-10）：逐条 `GET /api/case/{id}` 取正文全文，计算 sha256 固化进清单；
  3. 快照（LC-8）：报告头记 `corpus_digest`；未就绪时降级记
     `indexed_documents` + 探测时间并标注复现性受限。

- 语料变更导致 sha256 不一致的文书会被**剔除并如实计数**——此时评测集显式改版
  （evalset v1 → v2），不得静默换集。
- 清单不进 git 的正文数据：只存 id、查询原文与 sha256（引用留痕，不是语料副本）。

## 指标与目标（开发文档 11.1，不降标准）

| 指标 | 目标 | 说明 |
|---|---|---|
| 逃逸幻觉率 | < 1% | 入库事实复验定位失败比例（verify.Locate 同一判定） |
| 溯源准确率 | > 98% | 入库事实引用有效且定位成功比例 |
| 门禁误拦率 | 0 | 对抗集必放样本被拦截比例 |
| 门禁漏拦率 | 0 | 对抗集必拦样本放行比例（合规红线） |

未达标项如实写入 CHANGELOG「已知限制」。

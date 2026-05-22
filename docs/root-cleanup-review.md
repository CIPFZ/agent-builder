# 根目录清理评估

本文只评估仓库根目录、`.github/`、`.agents/` 和少量根层配置文件，目的是进一步收紧 Agent Builder 的产品边界。

## 结论先行

根目录不需要粗暴清空，但需要分层处理：

- **保留**：当前开发和构建仍依赖的文件。
- **迁移/重写**：仍有价值，但内容和语义明显偏向旧 Crush。
- **归档**：历史参考、发布模板、法律/CI 遗留。
- **删除**：确认无引用的 demo、生成物、旧模板残留。

## 根目录建议

| Path | 建议 | 原因 |
| --- | --- | --- |
| `AGENTS.md` | keep | 当前开发指引。 |
| `README.md` | migrate | 仍是 Crush 叙述为主，需要重写为 Agent Builder 产品 README。 |
| `LICENSE.md` | keep | 必需。 |
| `go.mod` / `go.sum` | keep | 根模块仍是当前主构建入口。 |
| `main.go` | keep | 仍是当前 Go 入口之一，后续若 CLI 彻底降级再迁移。 |
| `Taskfile.yaml` | migrate | 内容混合了构建、生成、旧 Crush 任务，需要拆分整理。 |
| `.gitignore` / `.gitattributes` | keep | 仓库卫生文件。 |
| `.golangci.yml` | keep | 仍在 lint 流程里使用。 |
| `crush.json` | migrate | 更像开发者/旧产品配置，不应继续以产品主配置语义存在。 |
| `schema.json` | migrate | 生成物，建议改为按需生成或移到 reference。 |
| `sqlc.yaml` | keep or migrate | 若仍由 sqlc 生成 DB 代码可保留；若不再依赖则迁移到 scripts。 |
| `.goreleaser.yml` | archive | 旧发布链路，和当前桌面客户端目标不一致。 |
| `CLA.md` | archive | 旧法律/贡献流程材料，除非仍继续使用原 CLA 流程。 |
| `flake.nix` / `flake.lock` / `.envrc` | archive | 仅在团队明确使用 Nix 时保留；否则归档。 |

## `.github/` 建议

`.github/` 不能直接删，但要按用途拆开：

| Path | 建议 | 原因 |
| --- | --- | --- |
| `.github/workflows/build.yml` | keep/migrate | 若仍用于产品构建则保留，但需确认路径和产物已指向 Agent Builder。 |
| `.github/workflows/lint.yml` | keep/migrate | 仍可能有效。 |
| `.github/workflows/schema-update.yml` | migrate | 依赖旧 schema 生成链路，需对齐新配置归属。 |
| `.github/workflows/release.yml` | migrate/archive | 大概率仍偏旧 Crush release 流程。 |
| `.github/workflows/nightly.yml` | archive | 多半是旧模板继承。 |
| `.github/workflows/cla.yml` | archive | 若不继续维护 CLA 流程，应移除或归档。 |
| `.github/workflows/labeler.yml` | keep | 只要 label 规则仍有用就保留。 |
| `.github/workflows/lint-sync.yml` | keep | 如果仍引用 meta 模板可保留。 |
| `.github/workflows/security.yml` | keep/migrate | 视是否仍适配当前 repo 而定。 |
| `.github/dependabot.yml` | keep/migrate | 依赖管理仍有价值，但要确认目录和包名已更新。 |
| `.github/entitlements.plist` | keep/migrate | 若 desktop 打包需要代码签名权限则保留。 |
| `.github/cla-signatures.json` | archive | 若不再走 CLA 流程，可以归档或删除。 |
| `.github/labeler.yml` | keep | CI label 管理文件。 |

## `.agents/` 建议

| Path | 建议 | 原因 |
| --- | --- | --- |
| `.agents/skills/` | keep | 本地技能/工作流元数据，属于开发环境，不是产品代码。 |
| `.agents/skills/builtin-skills/SKILL.md` | keep | 有明确用途。 |
| `.agents/skills/shell-builtins/SKILL.md` | keep | 有明确用途。 |

`.agents/` 不建议删。它不属于产品 runtime，但属于本地开发协作层。

## 需要重点重写的内容

以下文件不是删除对象，但应尽快改写：

- `README.md`
- `Taskfile.yaml`
- `crush.json`
- 部分 `.github/workflows/*`

这些文件的当前问题不是“存在”，而是仍然按 Crush 项目叙述、发布和配置。

## 建议删除对象

在确认无引用后，优先考虑删除：

- `CLA.md`（如果 CLA 流程不再保留）
- `.github/cla-signatures.json`
- 旧 `nightly` / `release` 模板产物
- 与当前桌面客户端无关的 demo / 生成物配置

## 下一步

建议按以下顺序处理：

1. 重写 `README.md`。
2. 拆分 `Taskfile.yaml`。
3. 清理 `.github/workflows` 中的旧 Crush 依赖。
4. 归档或删除 `CLA.md`、`cla-signatures.json`、旧发布模板。
5. 评估 `crush.json`、`schema.json`、`flake.*` 是否还需要。


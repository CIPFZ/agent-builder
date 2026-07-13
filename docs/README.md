# Agent Builder 文档

本目录只描述当前代码库的真实状态，不保存历史任务、阶段计划或外部产品对比。

当前对话输出架构正在进行分阶段迁移；迁移期间以
[对话架构迁移](conversation-architecture-migration.md) 记录目标契约、阶段状态、
验收证据和提交位置，完成后再将最终状态合并回常规架构文档。

## 阅读顺序

1. [项目概览](project-overview.md)：产品定位、技术栈、核心边界和目录概览。
2. [系统架构](architecture.md)：进程、分层、依赖方向和主要运行链路。
3. [运行时](runtime.md)：会话、Turn、Run、工具、权限、上下文和事件模型。
4. [前端与桌面端](frontend-and-desktop.md)：React、Wails 桥接、传输适配和状态边界。
5. [数据与能力](data-and-capabilities.md)：SQLite、配置、项目记忆、Skills、MCP、Hooks、LSP。
6. [数据存储规范](data-storage.md)：权威来源、作用域、项目目录、Objects 和生命周期。
7. [开发指南](development.md)：构建、测试、调试和改动约束。
8. [目录索引](repository-map.md)：按目录快速定位代码职责。

## 文档维护规则

- 以源码、测试、迁移和构建脚本为准；文档与实现冲突时先核对实现。
- 文档记录已经存在的能力。尚未实现的设想应进入 issue，不在此目录维护长期任务清单。
- 架构变化应同时更新对应文档和入口链接。
- 不以 React 状态、Wails binding 或某个 UI 组件作为运行时事实来源。
- 外部产品只能作为研究输入，不能把其品牌、文案、资产或专有交互写成本项目规范。

最后一次基于源码梳理：2026-07-11。

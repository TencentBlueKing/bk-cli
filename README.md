# bk-cli

[![license](https://img.shields.io/badge/license-MIT-brightgreen.svg?style=flat)](https://github.com/TencentBlueKing/bk-cli/blob/main/LICENSE.txt) [![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen.svg)](https://github.com/TencentBlueKing/bk-cli/pulls)

简体中文 | [English](README_EN.md)

## 概览

一个用于与 [BlueKing](https://bk.tencent.com/) 平台 API 交互的命令行工具。它面向 **开发者、自动化任务和 agent 集成** 场景，默认输出结构化 JSON；涉及远端请求或执行更新的命令支持 `--dry-run`，并提供带真实示例的丰富 `--help`。

详细设计请参考 [docs/design.md](docs/design.md)。

## 功能特性

- **三层调用方式**: 原始 API 调用、系统子命令，以及快捷指令（未来）
- **多上下文支持**: 管理多个 BlueKing 部署的凭据
- **自动化友好的输出**: 带 `ok` 字段的 JSON 信封、机器可读错误、可预测的退出码
- **丰富的系统命令**: 内置多个 BlueKing system 子命令，也保留原始 `api` 调用作为兜底能力
- **加密凭据存储**: 每个上下文使用 AES-256-GCM 加密
- **单一二进制文件**: 无运行时依赖

## 快速开始

使用者：

- [用户指南](docs/user-guide.md)

开发者：

- [开发环境搭建](docs/develop-environment-guide.md)
- [扩展开发指南](docs/develop-extension-guide.md)

## 支持

- [蓝鲸智云 - 学习社区](https://bk.tencent.com/s-mart/community)
- [蓝鲸 DevOps 在线视频教程](https://bk.tencent.com/s-mart/video)
- 加入技术交流 QQ 群：

![img](docs/resource/img/bk_qq_group.png)

## 蓝鲸社区

- [BK-APIGateway](https://github.com/TencentBlueKing/blueking-apigateway)：蓝鲸 API 网关是一个高性能、高可用的 API 托管服务，可以帮助开发者创建、发布、维护、监控和保护 API， 以快速、低成本、低风险地对外开放蓝鲸应用或其他系统的数据或服务。
- [BK-CI](https://github.com/TencentBlueKing/bk-ci)：蓝鲸持续集成平台是一个开源的持续集成和持续交付系统，可以轻松将你的研发流程呈现到你面前。
- [BK-BCS](https://github.com/TencentBlueKing/bk-bcs)：蓝鲸容器管理平台是以容器技术为基础，为微服务业务提供编排管理的基础服务平台。
- [BK-SOPS](https://github.com/TencentBlueKing/bk-sops)：标准运维（SOPS）是通过可视化的图形界面进行任务流程编排和执行的系统，是蓝鲸体系中一款轻量级的调度编排类
  SaaS 产品。
- [BK-CMDB](https://github.com/TencentBlueKing/bk-cmdb)：蓝鲸配置平台是一个面向资产及应用的企业级配置管理平台。
- [BK-JOB](https://github.com/TencentBlueKing/bk-job)：蓝鲸作业平台（Job）是一套运维脚本管理系统，具备海量任务并发处理能力。

## 贡献

如果你有好的意见或建议，欢迎给我们提 Issues 或 PullRequests，为蓝鲸开源社区贡献力量。关于分支 / Issue 及 PR,
请查看 [CONTRIBUTING](docs/CONTRIBUTING.md)。

[腾讯开源激励计划](https://opensource.tencent.com/contribution) 鼓励开发者的参与和贡献，期待你的加入。

## 证书

基于 MIT 协议，详细请参考 [LICENSE](LICENSE.txt)
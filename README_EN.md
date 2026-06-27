![img](docs/resource/img/bk_cli.png)

[![license](https://img.shields.io/badge/license-MIT-brightgreen.svg?style=flat)](https://github.com/TencentBlueKing/bk-cli/blob/main/LICENSE.txt) [![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen.svg)](https://github.com/TencentBlueKing/bk-cli/pulls)

[简体中文](README.md) | English

## Overview

A command-line tool for interacting with [BlueKing](https://bk.tencent.com/) platform APIs. It is built for **developers, automation workflows, and agent integrations**, with structured JSON as the default output. Commands that perform remote requests or updates support `--dry-run`, and `--help` includes real-world examples.

For the full design contract, see [docs/design.md](docs/design.md).

## Features

- **Three layers of invocation**: raw API calls, system subcommands, and shortcuts (planned)
- **Multi-context support**: manage credentials for multiple BlueKing deployments
- **Automation-friendly output**: JSON envelopes with an `ok` field, machine-readable errors, and predictable exit codes
- **Rich system commands**: built-in BlueKing system subcommands, with raw `api` calls kept as a fallback
- **Embedded agent skills**: inspect version-matched usage guidance with `bk-cli skills list` and `bk-cli skills read <name>`
- **Encrypted credential storage**: AES-256-GCM encryption per context
- **Single binary**: with no runtime dependencies

## Quick Start

For users:

- [User Guide](docs/user-guide.md)

For developers:

- [Development Environment Setup](docs/develop-environment-guide.md)
- [Extension Development Guide](docs/develop-extension-guide.md)

## Support

- [BlueKing Community](https://bk.tencent.com/s-mart/community)
- [BlueKing DevOps Video Tutorials](https://bk.tencent.com/s-mart/video)
- Join the technical discussion QQ group:

![img](docs/resource/img/bk_qq_group.png)

## BlueKing Community

- [BK-APIGateway](https://github.com/TencentBlueKing/blueking-apigateway): BlueKing API Gateway is a high-performance, high-availability API hosting service that helps developers create, publish, maintain, monitor, and protect APIs, to quickly, low-cost, and low-riskly expose BlueKing applications or other system's data or services.
- [BK-CI](https://github.com/TencentBlueKing/bk-ci): BlueKing Continuous Integration is an open-source CI/CD system that brings your delivery pipeline into view.
- [BK-BCS](https://github.com/TencentBlueKing/bk-bcs): BlueKing Container Service is a container-based orchestration platform for microservice workloads.
- [BK-SOPS](https://github.com/TencentBlueKing/bk-sops): Standard Operations (SOPS) is a lightweight scheduling and orchestration SaaS product in the BlueKing ecosystem, with visual workflow design and execution.
- [BK-CMDB](https://github.com/TencentBlueKing/bk-cmdb): BlueKing Configuration Management Database is an enterprise-grade platform for asset and application configuration.
- [BK-JOB](https://github.com/TencentBlueKing/bk-job): BlueKing Job is an operations script management platform with high-concurrency task execution.

## Contributing

We welcome issues and pull requests with ideas, feedback, and improvements for the BlueKing open-source community. For branch, issue, and PR conventions, see [CONTRIBUTING](docs/CONTRIBUTING.md).

The [Tencent Open Source Incentive Program](https://opensource.tencent.com/contribution) encourages developer participation—we would love to have you involved.

## License

Released under the MIT License. See [LICENSE](LICENSE.txt) for details.

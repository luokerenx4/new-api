<div align="center">

![Alice AI Gateway](/web/default/public/logo.png)

# Alice AI Gateway

**OpenAlice 托管的 AI 网关数据面，基于 New API。**

</div>

## 项目定位

Alice AI Gateway 是 OpenAlice 的 AI 执行网关。它应该作为 OpenAlice Cloud
后面的独立数据面服务部署，而不是作为独立面向消费者的 SaaS 或公开自助计费门户。

在 OpenAlice 的部署结构里：

- **OpenAlice Cloud** 负责账户、登录、订阅、权益、计费、激活和后台运营。
- **Alice AI Gateway** 负责模型路由、上游 provider 凭据、请求转发、token 级额度执行和用量统计。
- **OpenAlice 客户端** 使用 OpenAlice Cloud 发放的 managed AI credential，直接调用这个 gateway endpoint。
- 核心商业权限变更来自外部调度者，目前就是 OpenAlice Cloud。Gateway 应该把 Cloud 下发的 grant、发卡、冻结、吊销和额度操作视为 OpenAlice 托管用户的事实来源。

这个边界让私有的 OpenAlice Cloud 账户系统不进入 AGPL 仓库，同时让 AI gateway 的执行实现保持开源、可审计。

## OpenAlice 边界

Alice AI Gateway 应作为受 OpenAlice Cloud 控制的封闭执行面运行：

- 不要把 OpenAlice Cloud secrets、Stripe secrets、客户计费逻辑或私有账户权益代码放进这个仓库。
- 优先提供窄的内部 provisioning API、后台自动化或数据库支撑的操作，让 OpenAlice Cloud 用显式 service credential 调用。
- 内置的面向终端用户充值、订阅和 checkout 页面不是 OpenAlice 产品的事实来源。需要时可以禁用、隐藏，或仅作为内部维护工具保留。
- Gateway API key 是执行凭据。产品层面的归属、生命周期和付费访问权限属于 OpenAlice Cloud。

## 这个服务仍然负责什么

Alice AI Gateway 仍然是完整的 AI gateway/proxy：

- 继承自 New API 的 OpenAI-compatible、Claude-compatible、Gemini-compatible 等 relay surface。
- 多 provider channel 管理和模型路由。
- token 级限额、分组、模型限制和请求计量。
- 对 Cloud 发放的 managed AI credential 执行额度。
- 为 OpenAlice Cloud 对账所需的运营日志和 usage 数据。

## 部署草图

本地 Docker Compose：

```bash
docker compose up -d
```

本地构建镜像：

```bash
docker build -t alice-ai-gateway:latest .
```

用 SQLite 跑独立容器：

```bash
docker run --name alice-ai-gateway -d --restart always \
  -p 3000:3000 \
  -e TZ=Asia/Shanghai \
  -v ./data:/data \
  alice-ai-gateway:latest
```

生产环境中，gateway 应作为独立服务运行，使用专用数据库、专用 Redis、内部 admin/service credential，以及 OpenAlice 客户端访问的公开 HTTPS endpoint。

OpenAlice 生产环境应设置：

```bash
OPENALICE_MANAGED_MODE=true
```

managed mode 会禁用消费者自助注册、充值和订阅购买入口。operator 登录、admin API、gateway token、额度执行和对账数据仍然可用。

如果要让 OpenAlice Cloud 创建账户和发卡，需要设置一个足够长的随机 service secret：

```bash
OPENALICE_PROVISIONING_TOKEN=...
```

Cloud 调用 `/api/openalice/provisioning/*` 时使用
`Authorization: Bearer <token>` 或 `X-OpenAlice-Provisioning-Token: <token>`。
这个变量为空时 provisioning API 默认禁用。

## 开发

后端：

```bash
go test ./...
go build -o alice-ai-gateway
```

前端：

```bash
cd web/default
bun install
bun run build
```

Go module path 目前仍保留为 `github.com/QuantumNous/new-api`，避免高风险 import path
大规模重写。公开服务名、Docker 命名和 OpenAlice 文档使用 `alice-ai-gateway`。

## OpenAlice 改造笔记

第一阶段 OpenAlice 改造目标是账户和额度边界：

- 把 OpenAlice Cloud 视为权威账户和权益系统。
- 增加或暴露窄的 provisioning boundary，用于创建 gateway 用户、签发 token、充值额度、冻结 token、吊销 token，以及拉取对账快照。
- 面向用户的充值/订阅流程不要放进 OpenAlice 产品路径，除非明确改造成内部运营工具。
- 使用 `OPENALICE_MANAGED_MODE=true` 关闭消费者自助账户和计费入口，同时保留 operator 与执行面能力。
- 设置 `OPENALICE_PROVISIONING_TOKEN` 来启用 Cloud-facing 的
  `/api/openalice/provisioning/*` service boundary。
- 保留请求级额度安全。原子额度准入、清晰的 settle/refund 行为比 UI 便利更重要。

当前 user/token/quota 结构和下一步 provisioning boundary 见
[OpenAlice 账户适配笔记](./docs/openalice-account-adaptation.md)。

## 上游归属与许可证

Alice AI Gateway 是 [New API](https://github.com/QuantumNous/new-api) 的修改分发版本。
New API 使用 [GNU Affero General Public License v3.0](./LICENSE) 授权。

AGPLv3 Section 7 的附加条款适用。修改版本必须在适当的法律声明，以及任何面向用户的显著 about、legal、footer 或 attribution 位置保留作者归属声明：
`Frontend design and development by New API contributors.`

如果修改版本提供用户界面，也必须在显著 about、legal、footer 或 attribution 位置保留原项目链接：
<https://github.com/QuantumNous/new-api>。

New API 本身基于 [One API](https://github.com/songquanpeng/one-api)（MIT License）。
归属和第三方许可证信息见 [NOTICE](./NOTICE) 和
[THIRD-PARTY-LICENSES.md](./THIRD-PARTY-LICENSES.md)。

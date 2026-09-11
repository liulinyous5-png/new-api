# 渠道账号聚合：独立 Base URL

OpenAI 兼容渠道（类型 1）和 Azure 渠道（类型 3）支持将每个密钥与自己的 `base_url` 绑定。新功能由“为每个密钥设置独立的 Base URL”开关显式启用，默认关闭。已有渠道不会自动开启，也不会根据密钥内容自动切换模式。关闭时，原有密钥解析、去重、替换状态、随机与轮询行为保持不变。

## 使用方法

1. 创建渠道，选择 OpenAI 或 Azure。
2. 创建模式选择“密钥聚合”，策略选择“轮询”。
3. 开启“为每个密钥设置独立的 Base URL”，然后在密钥输入框粘贴账号 JSON 数组：

```json
[
  {"key": "key-001", "base_url": "https://resource-001.openai.azure.com"},
  {"key": "key-002", "base_url": "https://resource-002.openai.azure.com"},
  {"key": "key-003", "base_url": "https://resource-003.openai.azure.com"}
]
```

可以一次粘贴 200 个账号。也支持每行一个完整的账号 JSON 对象，并与原有纯密钥行混合。

`base_url` 省略或留空时使用渠道默认地址。Azure 如果没有填写渠道默认地址，则每个账号都必须提供地址。地址必须为 HTTP(S)，不能包含用户名、密码、查询参数或片段。

同一渠道的账号共享渠道类型、模型映射、API 版本等配置；Azure 的 API 版本仍在渠道配置中填写。

## 轮询与管理

轮询按账号排列顺序分配请求：`1 → 2 → … → 200 → 1`，跳过禁用账号。并发请求的响应完成顺序由上游决定。

密钥管理列表显示账号地址，可以逐个测试、禁用、恢复或删除。指定账号的测试允许测试禁用账号，不会自动启用它，也不会推进轮询游标。

编辑渠道时，可以粘贴账号列表并选择“追加”或“替换”。追加按完整账号记录去重，同一密钥配不同地址会保留为不同账号。启用独立地址的新模式下，替换会清除原账号的禁用状态和轮询位置，避免把旧状态套用到新账号。删除账号时，剩余记录的密钥与地址始终保持绑定。

批量创建独立渠道也接受相同 JSON 格式；如需所有账号在一个渠道内轮询，请选择密钥聚合。

当前顺序保证限于单个服务进程。多个服务实例没有全局共享的原子轮询游标；内存缓存模式下，重启也不保证从重启前的下一账号继续。

## 存储与验证

账号对象规范化后存入现有渠道 `key` 字段，每行一条；模式开关存于现有 `channel_info` JSON 的可选 `account_credentials` 字段，默认省略。没有新增数据库列或迁移。创建和修改接口通过顶层 `account_credentials: true` 显式启用此功能。关闭已启用的模式需要提供替换密钥，避免把账号 JSON 当作原始密钥发送。调用上游时只发送真实密钥，状态更新使用完整账号记录，避免同密钥不同地址的账号相互影响。

2026-09-11 验证结果：

| 检查 | 结果 |
| --- | --- |
| SQLite 3.50.4 | 通过 |
| MySQL 8.0.45 | 通过 |
| PostgreSQL 18.4 | 通过 |

数据库测试覆盖现有纯密钥记录更新为账号列表、状态保留、账号绑定读写、轮询跳过禁用项、数据库加载后的游标推进与循环。测试使用隔离的临时实例。

```sh
GOCACHE=/private/tmp/new-api-go-cache \
TEST_MYSQL_DSN='root@unix(/private/tmp/new-api-validation-runtimes/mysql.sock)/account_credential_test?charset=utf8mb4&parseTime=True&loc=Local' \
TEST_POSTGRES_DSN='host=127.0.0.1 port=25432 user=account_test dbname=postgres sslmode=disable' \
go test ./model -run '^TestChannelCredentialDatabaseRoundTrip$' -count=1 -v

GOCACHE=/private/tmp/new-api-go-cache go test -p 4 ./... -count=1

cd web
bun run test src/features/channels/lib/__tests__ src/features/channels/components/__tests__
bun run typecheck
bun run build
```

本次验证使用 Go 1.26.1、Bun 1.4.2。前端检查也包括所有修改文件的 oxlint 和格式检查。

## 兼容性复核

完整根模块测试 `go test -p 4 ./... -count=1` 通过；前端 28 项相关测试、typecheck、构建、修改文件 lint 和格式检查通过。三数据库实测重新验证了旧记录默认不开启，以及显式启用后模式与账号记录的保存和轮询。

新增回归覆盖：旧的普通密钥和 JSON 形状密钥原样透传；旧随机模式跳过禁用密钥；旧渠道替换密钥后保持原有状态处理；未启用新模式的 Azure 表单保持原校验；开关默认关闭且受敏感配置权限控制；不改变开关值时不增加旧编辑操作的权限要求。

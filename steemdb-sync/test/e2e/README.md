# Cold Ingest 端到端测试

## 概述

本目录包含冷启动 ingest 服务的端到端测试，验证从 steemd plugin 到 MongoDB 的完整数据流。

## 测试用例

### 1. TestColdIngestE2E

完整的端到端测试，使用真实的 steemd Docker 镜像：

1. 启动 MongoDB 测试容器
2. **启动 `cold_ingest` 服务**（测试会自动启动）
3. **等待 `cold_ingest` HTTP 服务器就绪**
4. **自动启动 `steemd:with-ingest` Docker 容器**（使用 `test/steem-test/run.sh` 脚本）
5. 等待 steemd 初始化完成
6. 等待 steemd replay 完成（`cold_ingest` 会在达到目标高度后自动退出）
7. 验证数据写入 MongoDB
8. 验证 meta 集合更新

**要求：**
- Docker 已安装并运行
- `steemd:with-ingest` 镜像已构建
- `test/steem-test/run.sh` 脚本存在且可执行（测试会自动调用）
- MongoDB 测试容器运行在 `127.0.0.1:27017`（或通过 `MONGODB_URI` 环境变量指定）
- `test/steem-test/data/blockchain/` 目录包含 `block_log` 文件

**重要启动顺序：**
1. 测试会自动启动 `cold_ingest` 服务
2. 测试会等待 HTTP 服务器就绪（约 30 秒）
3. **测试会自动启动 steemd 容器**（使用 `test/steem-test/run.sh` 脚本）
4. 测试会等待 steemd 初始化完成（**1 分钟**，用于容器启动和重建 block_log.index）
5. 测试会等待 `cold_ingest` 完成（最多 **60 分钟**，包括 replay 时间）

**容器管理：**
- 测试会自动启动 steemd 容器（如果不存在）
- 测试结束后，如果容器是测试启动的，会自动停止（除非设置 `KEEP_STEEMD_CONTAINER=true`）
- 如果容器在测试前已存在，测试会使用现有容器，不会自动停止
- 使用 `KEEP_STEEMD_CONTAINER=true` 环境变量可以保留容器以便调试

**测试超时和性能：**
- 测试总超时时间：**90 分钟**（在 `run_test.sh` 中设置）
- `cold_ingest` 等待超时：**60 分钟**（在测试代码中设置）
- steemd 初始化等待：**1 分钟**（容器启动和索引重建）
- Replay 时间：取决于 `block_log` 大小和目标高度
  - 1000 个区块通常需要 10-20 分钟
  - 更大的 `block_log` 可能需要更长时间
- 测试会每 30 秒输出进度信息（当前 max_block 和操作数量）

### 2. TestColdIngestWithJSONLReplay

使用 JSONL 文件回放的测试（推荐用于调试）：

使用 `test_tools jsonl_replay` 工具从 JSONL 文件（steemd ingest 插件 dry-run 模式生成）读取数据并发送到 `cold_ingest`。

**使用步骤：**

1. 启动 MongoDB 测试容器：
   ```bash
   cd steemdb-sync/test
   ./scripts/start_mongo.sh
   ```

2. 启动 `cold_ingest` 服务（在一个终端）：
   ```bash
   cd steemdb-sync
   ../bin/cold_ingest -config configs/config.yaml
   ```

3. 使用 `test_tools jsonl_replay` 工具发送数据（在另一个终端）：
   ```bash
   cd steemdb-sync
   ../bin/test_tools jsonl_replay -file test/steem-test/data/ingest/ingest_20260111_081033_778.jsonl \
     -start-block 1 -end-block 1000 -rate 1000
   ```

4. 验证数据写入 MongoDB

**优势：**
- 不需要运行 steemd 容器
- 可以精确控制发送的块范围
- 可以控制发送速率
- 适合调试和测试

### 3. TestColdIngestWithMockPlugin

使用模拟 plugin 的快速测试：

1. 启动 MongoDB 测试容器
2. 启动 `cold_ingest` 服务
3. 发送模拟的 operation JSON
4. 验证数据写入 MongoDB
5. 验证 `cold_ingest` 正确退出

**要求：**
- MongoDB 测试容器运行在 `127.0.0.1:27017`（或通过 `MONGODB_URI` 环境变量指定）

## 前置条件

### 1. MongoDB 测试环境

启动 MongoDB 4.4 测试容器：

```bash
# 使用提供的脚本
./test/scripts/start_mongo.sh

# 或手动启动
docker run -d --name mongo-test -p 27017:27017 \
  -e MONGO_INITDB_ROOT_USERNAME=admin \
  -e MONGO_INITDB_ROOT_PASSWORD=123456 \
  mongo:4.4
```

### 2. 构建 cold_ingest 二进制

```bash
cd steemdb-sync
mkdir -p ../bin
go build -o ../bin/cold_ingest ./cmd/cold_ingest
```

### 3. Docker 镜像和容器（仅 TestColdIngestE2E 需要）

确保 `steemd:with-ingest` 镜像已构建：

```bash
docker images | grep steemd:with-ingest
```

**重要：手动启动 steemd 容器**

测试代码**不会自动启动** steemd 容器，您需要**手动启动**。测试会检查容器是否运行，如果未找到容器，测试会被跳过。

**启动 steemd 容器：**

在运行测试之前，请先启动 steemd 容器。根据您的镜像配置，可以使用以下方式之一：

**方式 1：直接指定 steemd 命令（推荐）**

根据官方 Dockerfile，steemd 安装在 `/usr/local/steemd/bin`，需要明确指定启动命令：

```bash
docker run -d \
  --name steemd-ingest-test \
  --add-host host.docker.internal:host-gateway \
  steemd:with-ingest \
  /usr/local/steemd/bin/steemd \
    --replay-blockchain \
    --plugin ingest \
    --ingest-endpoint http://host.docker.internal:8080/ingest/applied_ops \
    --data-dir /var/steem
```

**方式 2：挂载 block_log 目录（如果需要）**

```bash
docker run -d \
  --name steemd-ingest-test \
  --add-host host.docker.internal:host-gateway \
  -v /path/to/blockchain:/var/steem/blockchain \
  steemd:with-ingest \
  /usr/local/steemd/bin/steemd \
    --replay-blockchain \
    --plugin ingest \
    --ingest-endpoint http://host.docker.internal:8080/ingest/applied_ops \
    --data-dir /var/steem
```

**注意：**
- 官方 Dockerfile 的默认 CMD 只是打印构建信息，不会启动 steemd
- 必须明确指定 `steemd` 命令和所有参数
- `--data-dir /var/steem` 是工作目录（Dockerfile 中设置）
- 如果使用 volume 挂载，确保路径正确

**检查容器状态：**

```bash
# 查看容器是否运行
docker ps | grep steemd-ingest-test

# 查看容器日志
docker logs -f steemd-ingest-test
```

**停止容器（测试完成后）：**

```bash
docker stop steemd-ingest-test
docker rm steemd-ingest-test
```

**测试会检查的容器名称：**
- `steemd-ingest-test`
- `steemd`
- `steemd-test`
- 或任何使用 `steemd:with-ingest` 镜像的容器

## 运行测试

### 运行所有端到端测试

```bash
cd steemdb-sync
go test -v ./test/e2e/... -timeout 90m
```

### 运行特定测试

```bash
# 仅运行模拟 plugin 测试（更快）
go test -v ./test/e2e/... -run TestColdIngestWithMockPlugin -timeout 10m

# 仅运行完整 E2E 测试（需要 Docker 镜像）
go test -v ./test/e2e/... -run TestColdIngestE2E -timeout 90m
```

### 跳过端到端测试

```bash
go test -v ./test/e2e/... -short
```

## 环境变量

- `MONGODB_URI`: MongoDB 连接 URI（默认：`mongodb://admin:123456@127.0.0.1:27017/steemdb_test?authSource=admin`）

## 测试流程

### TestColdIngestE2E 流程

1. **准备阶段**
   - 检查 Docker 镜像是否存在
   - 创建测试数据库
   - 创建临时配置文件

2. **启动阶段**
   - 启动 `cold_ingest` 服务（作为子进程）
   - 等待 HTTP 服务器就绪
   - 启动 `steemd` Docker 容器

3. **执行阶段**
   - steemd 开始 replay
   - plugin 发送 operation JSON 到 ingest 服务
   - ingest 服务批量写入 MongoDB

4. **验证阶段**
   - 等待 `cold_ingest` 达到目标高度并退出
   - 验证 blocks 集合有数据
   - 验证 operations 集合有数据
   - 验证 meta 集合正确更新

### TestColdIngestWithMockPlugin 流程

1. **准备阶段**
   - 创建测试数据库
   - 创建临时配置文件

2. **启动阶段**
   - 启动 `cold_ingest` 服务（作为子进程）
   - 等待 HTTP 服务器就绪

3. **执行阶段**
   - 发送模拟的 operation JSON
   - ingest 服务批量写入 MongoDB

4. **验证阶段**
   - 等待 `cold_ingest` 达到目标高度并退出
   - 验证数据完整性

## 故障排查

### cold_ingest 无法启动

- 检查二进制文件是否存在
- 检查配置文件路径是否正确
- 检查 MongoDB 连接是否正常

### HTTP 服务器未就绪

- 检查端口 8080 是否被占用
- 检查防火墙设置
- 增加等待超时时间

### Docker 容器启动失败

- 检查 Docker 是否运行
- 检查镜像是否存在
- 检查网络配置（host.docker.internal）

### 数据验证失败

- 检查 MongoDB 连接
- 检查目标高度设置
- 检查 safety margin 设置
- 查看 cold_ingest 日志

## 注意事项

1. **测试隔离**：每个测试使用独立的数据库，测试结束后自动清理
2. **超时设置**：完整 E2E 测试可能需要较长时间（30 分钟），请设置足够的超时
3. **资源消耗**：steemd replay 会消耗大量 CPU 和内存
4. **网络要求**：TestColdIngestE2E 需要 Docker 网络配置正确

## 下一步

完成端到端测试后，可以继续：
- Live Sync 集成测试
- Repair Tool 集成测试
- 性能测试

# SteemDB Sync 测试

本目录包含 SteemDB Sync 项目的所有测试代码和测试工具。

## 目录结构

```
test/
├── unit/              # 单元测试
│   ├── config/        # Config 模块测试 ✅
│   ├── model/         # Model 模块测试 ✅
│   ├── mongo/         # Mongo 模块测试
│   ├── pipeline/      # Pipeline 模块测试
│   ├── rpc/           # RPC 模块测试
│   ├── checker/       # Checker 模块测试
│   └── metrics/       # Metrics 模块测试
├── integration/       # 集成测试
│   ├── cold_ingest/   # Cold Ingest 集成测试
│   ├── live_sync/     # Live Sync 集成测试
│   └── repair/        # Repair Tool 集成测试
├── e2e/               # 端到端测试
├── performance/       # 性能测试
├── scripts/           # 测试工具脚本
└── test_data/         # 测试数据文件
```

## 运行测试

### 运行所有单元测试
```bash
cd steemdb-sync
go test ./test/unit/... -v
```

### 运行特定模块测试
```bash
# Config 模块
go test ./test/unit/config/... -v

# Model 模块
go test ./test/unit/model/... -v
```

### 运行测试并查看覆盖率
```bash
go test ./test/unit/... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

### 运行集成测试
```bash
go test ./test/integration/... -v
```

### 运行性能测试
```bash
go test -bench=. ./test/performance/... -v
```

## 测试环境要求

- Go 1.22+
- MongoDB 4.4+ (测试环境，**注意：必须使用 MongoDB 4.4，详见下方说明**)
- 测试数据库：`steemdb_test`

### ⚠️ MongoDB 版本重要说明

**必须使用 MongoDB 4.4，不能使用 MongoDB 5.0+**

#### AVX 指令集要求

MongoDB 5.0 及以上版本（包括 MongoDB 7）要求 CPU 支持 **AVX（Advanced Vector Extensions）** 指令集。如果您的 CPU 不支持 AVX，MongoDB 5.0+ 将无法启动，会出现以下错误：

```
WARNING: MongoDB 5.0+ requires a CPU with AVX support, 
and your current system does not appear to have that!
Illegal instruction (core dumped)
```

#### 解决方案

使用 **MongoDB 4.4**，这是最后一个不强制要求 AVX 指令集的主要版本：

```bash
# ✅ 正确：使用 MongoDB 4.4
docker run -d --name mongodb-test -p 27017:27017 \
  -e MONGO_INITDB_ROOT_USERNAME=admin \
  -e MONGO_INITDB_ROOT_PASSWORD=123456 \
  mongo:4.4

# ❌ 错误：MongoDB 7 需要 AVX，在不支持的 CPU 上会失败
docker run -d --name mongodb-test mongo:7
```

#### 检查 CPU 是否支持 AVX

```bash
# 检查 CPU 是否支持 AVX
grep -o 'avx[^ ]*' /proc/cpuinfo | head -1

# 或者
cat /proc/cpuinfo | grep flags | grep -i avx
```

如果没有输出，说明 CPU 不支持 AVX，必须使用 MongoDB 4.4。

#### 为什么选择 MongoDB 4.4？

1. **兼容性**：在不支持 AVX 的 CPU 上可以正常运行
2. **功能完整**：满足所有测试需求
3. **稳定性**：长期支持版本，稳定可靠
4. **测试足够**：对于单元测试和集成测试，4.4 版本完全够用

## 环境变量

```bash
export MONGODB_URI="mongodb://localhost:27017/steemdb_test"
export RPC_ENDPOINT="https://api.steemit.com"
export TEST_MODE=true
```

## 测试进度

### 单元测试

- ✅ **Config 模块** (11/11 测试通过)
  - 配置加载测试
  - 环境变量覆盖测试
  - 配置验证测试
  - 超时解析测试

- ✅ **Model 模块** (9/9 测试通过)
  - Operation ID 生成测试
  - Virtual operation ID 测试
  - ID 唯一性测试
  - 数据模型序列化测试

- ✅ **Mongo 模块** (17/17 测试用例全部通过)
- ⏳ **Pipeline 模块** (待实现)
- ⏳ **RPC 模块** (待实现)
- ⏳ **Checker 模块** (待实现)
- ⏳ **Metrics 模块** (待实现)

### 集成测试

- ⏳ **Cold Ingest** (待实现)
- ⏳ **Live Sync** (待实现)
- ⏳ **Repair Tool** (待实现)

### 端到端测试

- ⏳ **完整工作流程** (待实现)

### 性能测试

- ⏳ **吞吐量测试** (待实现)
- ⏳ **延迟测试** (待实现)

## 参考文档

详细测试方案请参考：`../../.cursor/TEST_PLAN.md`

## 测试统计

- **总测试用例**: 37
- **已通过**: 37 (Config: 11, Model: 9, Mongo: 17)
- **失败**: 0
- **跳过**: 0 (MongoDB 可用时)
- **覆盖率**: 待统计

**注意**: Mongo 测试需要 MongoDB 4.4+ 运行。如果 MongoDB 不可用，测试会自动跳过。

## 运行 Mongo 测试

Mongo 测试需要 MongoDB 实例。可以通过以下方式运行：

### 方式 1: 使用 Docker 启动 MongoDB（推荐）

启动带认证的 MongoDB 容器（**必须使用 MongoDB 4.4**）：
```bash
docker run -d \
  --name mongo-test \
  -p 27017:27017 \
  -e MONGO_INITDB_ROOT_USERNAME=admin \
  -e MONGO_INITDB_ROOT_PASSWORD=123456 \
  mongo:4.4  # ⚠️ 注意：必须使用 4.4，不能使用 5.0+（需要 AVX 支持）

# 等待 MongoDB 启动（约 5-10 秒）
sleep 10

# 运行测试（测试代码会自动使用 admin/123456 认证）
cd steemdb-sync
go test ./test/unit/mongo/... -v

# 测试完成后清理
docker stop mongo-test && docker rm mongo-test
```

### 方式 2: 使用环境变量指定 MongoDB URI
```bash
# 如果使用不同的用户名/密码
export MONGO_USERNAME=admin
export MONGO_PASSWORD=123456
export MONGODB_URI="mongodb://admin:123456@localhost:27017/steemdb_test?authSource=admin"

go test ./test/unit/mongo/... -v
```

### 方式 3: 使用自定义 MongoDB URI
```bash
# 完全自定义连接字符串
export MONGODB_URI="mongodb://username:password@host:port/database?authSource=admin"
go test ./test/unit/mongo/... -v
```

### 默认配置

如果未设置环境变量，测试代码会使用以下默认值：
- 用户名: `admin`
- 密码: `123456`
- 主机: `127.0.0.1:27017` (使用 IP 地址更可靠)
- 数据库: `steemdb_test`
- 认证源: `admin`

连接字符串格式：`mongodb://admin:123456@127.0.0.1:27017/steemdb_test?authSource=admin`

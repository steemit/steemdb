# SteemDB Sync 测试方案

## 目录

1. [测试概述](#测试概述)
2. [测试环境准备](#测试环境准备)
3. [单元测试](#单元测试)
4. [集成测试](#集成测试)
5. [端到端测试](#端到端测试)
6. [性能测试](#性能测试)
7. [数据一致性测试](#数据一致性测试)
8. [测试工具和脚本](#测试工具和脚本)
9. [测试执行计划](#测试执行计划)

---

## 测试概述

### 测试目标

1. **功能正确性**：验证所有组件按预期工作
2. **数据完整性**：确保数据不丢失、不重复
3. **幂等性**：验证重复执行不会产生副作用
4. **性能指标**：验证系统能够处理预期的负载
5. **容错能力**：验证系统在异常情况下的行为

### 测试范围

- **核心组件**：
  - `cold_ingest`：冷启动数据接收服务
  - `live_sync`：实时区块同步服务
  - `repair`：数据修复工具
- **内部模块**：
  - `config`：配置管理
  - `model`：数据模型
  - `mongo`：MongoDB 访问层
  - `pipeline`：数据处理管道（batcher, handler）
  - `rpc`：RPC 客户端和转换器
  - `checker`：数据完整性检查
  - `metrics`：Prometheus 指标

### 测试类型

- **单元测试**：测试单个函数/方法
- **集成测试**：测试模块间交互
- **端到端测试**：测试完整工作流程
- **性能测试**：测试吞吐量和延迟
- **数据一致性测试**：测试幂等性和数据完整性

---

## 测试环境准备

### 前置条件

1. **MongoDB 实例**
   - 测试环境：本地 MongoDB 或 Docker 容器
   - **版本要求：必须使用 MongoDB 4.4**（不能使用 MongoDB 5.0+，详见下方说明）
   - 数据库名称：`steemdb_test`
   - 建议使用独立的测试数据库，避免污染生产数据

   **⚠️ 重要：MongoDB 版本选择**
   
   MongoDB 5.0 及以上版本（包括 MongoDB 7）要求 CPU 支持 **AVX（Advanced Vector Extensions）** 指令集。如果您的 CPU 不支持 AVX，MongoDB 5.0+ 将无法启动。
   
   **解决方案**：使用 MongoDB 4.4，这是最后一个不强制要求 AVX 指令集的主要版本。
   
   ```bash
   # ✅ 正确：使用 MongoDB 4.4
   docker run -d --name mongo-test -p 27017:27017 \
     -e MONGO_INITDB_ROOT_USERNAME=admin \
     -e MONGO_INITDB_ROOT_PASSWORD=123456 \
     mongo:4.4
   
   # ❌ 错误：MongoDB 7 需要 AVX，在不支持的 CPU 上会失败
   docker run -d --name mongo-test mongo:7
   ```
   
   检查 CPU 是否支持 AVX：
   ```bash
   grep -o 'avx[^ ]*' /proc/cpuinfo | head -1
   ```

2. **Steem RPC 节点**
   - 测试环境：公共 RPC 节点或本地测试节点
   - 端点：`https://api.steemit.com` 或测试节点

3. **测试数据**
   - 准备已知的区块数据用于验证
   - 准备缺失区块场景用于 repair 测试

### 环境变量

```bash
# MongoDB 测试配置
# 注意：如果使用认证，URI 格式为：mongodb://username:password@host:port/database?authSource=admin
export MONGODB_URI="mongodb://admin:123456@127.0.0.1:27017/steemdb_test?authSource=admin"
export MONGODB_DATABASE="steemdb_test"
# 或者分别设置用户名和密码
export MONGO_USERNAME="admin"
export MONGO_PASSWORD="123456"

# RPC 测试配置
export RPC_ENDPOINT="https://api.steemit.com"

# 测试模式
export TEST_MODE=true
```

### Docker Compose 测试环境（可选）

```yaml
# docker-compose.test.yml
version: '3.8'
services:
  mongo-test:
    image: mongo:4.4  # ⚠️ 必须使用 4.4，不能使用 5.0+（需要 AVX 支持）
    ports:
      - "27018:27017"
    environment:
      MONGO_INITDB_ROOT_USERNAME: admin
      MONGO_INITDB_ROOT_PASSWORD: 123456
      MONGO_INITDB_DATABASE: steemdb_test
    volumes:
      - mongo_test_data:/data/db

volumes:
  mongo_test_data:
```

**注意**：MongoDB 5.0+ 需要 CPU 支持 AVX 指令集。如果您的 CPU 不支持 AVX，必须使用 MongoDB 4.4。

---

## 单元测试

### 1. Config 模块测试 (`internal/config`)

#### 测试文件：`test/unit/config/config_test.go`

**测试用例：**

1. **配置加载测试**
   - [x] 从 YAML 文件加载配置
   - [x] 从环境变量覆盖配置
   - [x] 默认值设置正确
   - [x] 无效配置文件处理
   - [x] 缺失必需字段的错误处理

2. **配置解析测试**
   - [x] MongoDB URI 解析
   - [x] RPC 端点解析
   - [x] 超时时间解析（字符串转 Duration）
   - [x] Batch 配置解析
   - [x] 冷启动目标高度解析

**状态**: ✅ 11 个测试用例全部通过

**示例测试代码结构：**

```go
func TestLoadConfig(t *testing.T) {
    // 测试从文件加载
    cfg, err := Load("configs/config.yaml")
    assert.NoError(t, err)
    assert.NotNil(t, cfg)
    assert.Equal(t, "mongodb://localhost:27017/steemdb", cfg.Mongo.URI)
}

func TestEnvOverride(t *testing.T) {
    os.Setenv("MONGODB_URI", "mongodb://test:27017/test")
    defer os.Unsetenv("MONGODB_URI")
    
    cfg, err := Load("configs/config.yaml")
    assert.NoError(t, err)
    assert.Equal(t, "mongodb://test:27017/test", cfg.Mongo.URI)
}
```

### 2. Model 模块测试 (`internal/model`)

#### 测试文件：`test/unit/model/models_test.go`

**测试用例：**

1. **Operation ID 生成测试**
   - [x] 正常 operation ID 生成
   - [x] Virtual operation ID 生成（trx_index = -1）
   - [x] ID 唯一性验证
   - [x] ID 格式验证（block_num:trx_index:op_index）

2. **数据模型验证**
   - [x] Block 结构体序列化/反序列化
   - [x] Transaction 结构体序列化/反序列化
   - [x] Operation 结构体序列化/反序列化
   - [x] Meta 结构体序列化/反序列化

**状态**: ✅ 9 个测试用例全部通过

**示例测试代码：**

```go
func TestOperationID(t *testing.T) {
    id := OperationID(100, 2, 0)
    assert.Equal(t, "100:2:0", id)
    
    // Virtual operation
    idVirtual := OperationID(100, -1, -1)
    assert.Equal(t, "100:-1:-1", idVirtual)
}
```

### 3. Mongo 模块测试 (`internal/mongo`)

#### 测试文件：`test/unit/mongo/mongodb_test.go`

**测试策略：** 使用 MongoDB 4.4 Docker 容器（⚠️ 必须使用 4.4，不支持 5.0+）

**测试用例：**

1. **客户端初始化测试**
   - [x] 成功连接到 MongoDB
   - [x] 连接失败处理
   - [x] 索引自动创建
   - [x] 索引创建幂等性

2. **BulkWrite 测试**
   - [x] BulkUpsertBlocks 成功写入
   - [x] BulkUpsertTransactions 成功写入
   - [x] BulkUpsertOperations 成功写入
   - [x] 重复数据 upsert（幂等性）
   - [x] 空批次处理
   - [x] 大批次性能测试（1000+ 条）

3. **查询操作测试**
   - [x] GetMaxBlock 正确返回
   - [x] UpdateMaxBlock 正确更新
   - [x] GetBlockByNumber 正确查询
   - [x] GetOperationsByBlock 正确查询
   - [x] CheckBlockExists 正确判断

4. **Meta 集合测试**
   - [x] SetColdStartDone 正确设置
   - [x] GetMaxBlock 从 meta 读取
   - [x] 并发更新安全性

**状态**: ✅ 17 个测试用例全部通过

**重要说明**: 
- ⚠️ 必须使用 MongoDB 4.4（不支持 5.0+，需要 AVX 指令集）
- 测试使用认证的 MongoDB（admin/123456）
- 测试数据自动清理，确保测试隔离

**测试工具：** 使用 `go.mongodb.org/mongo-driver` 的测试工具或 `testcontainers-go`

### 4. Pipeline 模块测试 (`internal/pipeline`)

#### 4.1 Batcher 测试 (`test/unit/pipeline/batcher_test.go`)

**测试用例：**

1. **Batcher 初始化测试**
   - [x] 成功创建 batcher
   - [x] 配置验证
   - [x] Channel 容量设置

2. **批量处理测试**
   - [x] 按条数触发 flush（达到 batch_size）
   - [x] 按时间触发 flush（达到 flush_interval）
   - [x] 正常关闭时 flush 剩余数据
   - [x] 并发添加 operation 的安全性

3. **MaxBlock 跟踪测试**
   - [x] 正确更新 max_block_seen
   - [x] 并发更新安全性
   - [x] GetMaxBlockSeen 正确返回

4. **错误处理测试**
   - [x] Channel 关闭后的行为
   - [x] 停止后添加 operation 的错误处理

**状态**: ✅ 8 个测试用例全部通过

**重要修复**: 
- ✅ 修复了 `Stop()` 方法的无限循环问题（使用 channelClosed 标志）
- ✅ 修复了 `Stop()` 方法的阻塞问题（移除冗余 flush 调用）
- ✅ 改进了 `AddOperation` 的错误处理

#### 4.2 IngestHandler 测试 (`test/unit/pipeline/ingest_handler_test.go`)

**测试用例：**

1. **HTTP 请求处理测试**
   - [x] 正确解析 JSON 请求
   - [x] 无效 JSON 处理
   - [x] 缺失字段处理
   - [x] 成功响应（200）
   - [x] 错误响应（400, 500）

2. **数据验证测试**
   - [x] Operation JSON 格式验证
   - [x] Block 信息验证
   - [x] Transaction 信息验证
   - [x] Virtual operation 标记验证

3. **集成测试**
   - [x] 高并发请求处理

**状态**: ✅ 7 个测试用例全部通过

### 5. RPC 模块测试 (`internal/rpc`)

#### 5.1 RPC Client 测试 (`test/unit/rpc/client_test.go`)

**测试策略：** 部分测试需要真实 RPC 端点（使用 `-short` 标志可跳过）

**测试用例：**

1. **RPC 调用测试**
   - [x] 客户端初始化
   - [x] 无效端点处理
   - [x] 无效区块号处理（需要网络）
   - [x] GetBlockWithOps 结构验证（需要网络）
   - [x] GetOpsInBlock onlyVirtual 标志（需要网络）

2. **错误处理测试**
   - [x] 无效区块号处理
   - [x] RPC 节点不可用处理（需要网络）

**状态**: ✅ 8 个测试用例（部分需要网络连接）

**注意**: 网络相关测试在 `-short` 模式下会被跳过

#### 5.2 Converter 测试 (`test/unit/rpc/converter_test.go`)

**测试用例：**

1. **数据转换测试**
   - [x] ConvertBlock 正确转换
   - [x] ConvertTransaction 正确转换
   - [x] ConvertOperation 正确转换
   - [x] Virtual operation 标记正确
   - [x] Source 字段设置为 "rpc"

2. **边界情况测试**
   - [x] 空区块处理
   - [x] 无 transaction 区块处理
   - [x] nil timestamp/expiration 处理
   - [x] 不同操作类型转换

3. **数据一致性测试**
   - [x] 转换后的数据与 Mongo schema 一致
   - [x] 字段类型正确
   - [x] 时间格式正确

**状态**: ✅ 10 个测试用例全部通过

### 6. Checker 模块测试 (`internal/checker`)

#### 测试文件：`test/unit/checker/scanner_test.go`

**测试用例：**

1. **扫描功能测试**
   - [x] 扫描完整区块范围
   - [x] 正确识别缺失区块
   - [x] 正确识别无操作区块
   - [x] 混合场景（缺失区块 + 无操作区块）

2. **区间合并测试**
   - [x] 连续区块合并为区间
   - [x] 非连续区块保持独立
   - [x] 边界情况处理（空列表、单个区块）

3. **BlockRange 测试**
   - [x] Count() 方法测试
   - [x] String() 方法测试

**状态**: ✅ 11 个测试用例全部通过

**注意**: 测试使用唯一区块号避免冲突，需要 MongoDB 测试环境

### 7. Metrics 模块测试 (`internal/metrics`)

#### 测试文件：`test/unit/metrics/metrics_test.go`

**测试用例：**

1. **指标注册测试**
   - [x] 所有指标成功注册
   - [x] 指标名称正确
   - [x] 指标类型正确

2. **指标记录测试**
   - [x] RecordIngestOp 正确记录
   - [x] RecordMongoWrite 正确记录
   - [x] RecordRPCCall 正确记录
   - [x] RecordBatch 正确记录
   - [x] UpdateQueueSize 正确更新
   - [x] UpdateCurrentBlock 正确更新
   - [x] UpdateIngestTPS 正确更新

3. **TPS 计算测试**
   - [x] IncrementOpCount 正确递增
   - [x] 并发安全性
   - [x] StartTPSCalculator 正确计算 TPS

4. **HTTP Handler 测试**
   - [x] Metrics 端点返回正确格式
   - [x] Prometheus 格式验证
   - [x] 指标值正确性

**状态**: ✅ 12 个测试用例全部通过

---

## 集成测试

### 1. Cold Ingest 集成测试

#### 测试场景

1. **完整冷启动流程测试**
   - [ ] 启动 HTTP 服务器
   - [ ] 接收 plugin 发送的 operation JSON
   - [ ] 数据正确写入 MongoDB
   - [ ] 达到目标高度后正确退出
   - [ ] Meta 集合正确更新

2. **高吞吐量测试**
   - [ ] 模拟高频率 operation 推送（1000+ ops/sec）
   - [ ] 验证队列不溢出
   - [ ] 验证批量写入效率
   - [ ] 验证内存使用稳定

3. **错误恢复测试**
   - [ ] MongoDB 临时不可用后的恢复
   - [ ] 网络中断后的恢复
   - [ ] 数据不丢失

**测试工具：** 使用脚本模拟 plugin 发送数据

```bash
# 测试脚本示例
for i in {1..1000}; do
  curl -X POST http://localhost:8080/ingest/applied_op \
    -H "Content-Type: application/json" \
    -d @test_data/operation_$i.json
done
```

### 2. Live Sync 集成测试

#### 测试场景

1. **实时同步流程测试**
   - [ ] 从 max_block + 1 开始同步
   - [ ] 顺序获取新区块
   - [ ] 数据正确写入 MongoDB
   - [ ] max_block 正确更新
   - [ ] 处理区块不存在的情况（等待）

2. **长时间运行测试**
   - [ ] 运行 24 小时以上
   - [ ] 内存泄漏检查
   - [ ] 连接池稳定性
   - [ ] 错误恢复能力

3. **RPC 节点切换测试**
   - [ ] 主节点不可用时切换到备用节点
   - [ ] 重试机制验证
   - [ ] 数据不丢失

### 3. Repair Tool 集成测试

#### 测试场景

1. **扫描功能测试**
   - [ ] 扫描指定范围
   - [ ] 正确识别缺失区块
   - [ ] 区间合并正确
   - [ ] 进度报告准确

2. **修复功能测试**
   - [ ] Dry-run 模式（仅扫描）
   - [ ] 实际修复模式
   - [ ] 修复指定范围
   - [ ] 修复后数据完整性验证

3. **错误处理测试**
   - [ ] 单个区块修复失败不影响其他区块
   - [ ] RPC 错误重试
   - [ ] 修复报告准确性

---

## 端到端测试

### 1. 完整工作流程测试

#### 场景 1：冷启动 → Live Sync

**步骤：**

1. [ ] 清空测试数据库
2. [ ] 启动 `cold_ingest` 服务
3. [ ] 模拟 plugin 发送历史数据（block 1-10000）
4. [ ] 验证数据正确写入
5. [ ] 停止 `cold_ingest`
6. [ ] 启动 `live_sync` 服务
7. [ ] 验证从 block 10001 开始同步
8. [ ] 验证数据连续性

#### 场景 2：Live Sync → Repair

**步骤：**

1. [ ] 启动 `live_sync` 同步到 block 20000
2. [ ] 手动删除部分区块（模拟数据丢失）
3. [ ] 运行 `repair` 工具扫描
4. [ ] 验证正确识别缺失区块
5. [ ] 运行 `repair` 工具修复
6. [ ] 验证数据完整性恢复

#### 场景 3：重复执行测试（幂等性）

**步骤：**

1. [ ] 运行 `cold_ingest` 同步数据
2. [ ] 再次运行 `cold_ingest`（相同数据）
3. [ ] 验证无重复数据
4. [ ] 运行 `repair` 修复已存在的数据
5. [ ] 验证无重复数据

---

## 性能测试

### 1. 吞吐量测试

#### Cold Ingest 吞吐量

**测试目标：**
- 支持至少 5000 ops/sec 的吞吐量
- 队列不溢出
- MongoDB 写入延迟 < 100ms

**测试方法：**
1. 使用脚本模拟高频率推送
2. 监控队列大小
3. 监控 MongoDB 写入延迟
4. 监控内存使用

**成功标准：**
- 吞吐量 ≥ 5000 ops/sec
- 队列大小 < 80% 容量
- 99% 的写入延迟 < 100ms

#### Live Sync 吞吐量

**测试目标：**
- 支持至少 3 blocks/sec 的同步速度
- RPC 调用延迟 < 500ms

**测试方法：**
1. 监控同步速度
2. 监控 RPC 延迟
3. 监控数据库写入速度

### 2. 延迟测试

**测试指标：**
- Operation 从接收到写入的延迟
- Batch flush 延迟
- RPC 调用延迟
- MongoDB 写入延迟

**测试方法：**
- 使用 metrics 端点监控延迟分布
- 记录 P50, P95, P99 延迟

### 3. 压力测试

**测试场景：**
1. **高并发请求**：1000+ 并发 HTTP 请求
2. **大批次写入**：10000+ operations 批量写入
3. **长时间运行**：7x24 小时运行测试
4. **资源限制**：限制内存和 CPU 使用

**监控指标：**
- CPU 使用率
- 内存使用率
- Goroutine 数量
- 连接池使用情况

---

## 数据一致性测试

### 1. 幂等性测试

#### 测试场景

1. **重复写入测试**
   - [ ] 相同 operation 写入两次
   - [ ] 验证数据库中只有一条记录
   - [ ] 验证 `_id` 唯一性

2. **重复执行测试**
   - [ ] 运行 `cold_ingest` 两次（相同数据）
   - [ ] 运行 `live_sync` 两次（相同区块）
   - [ ] 运行 `repair` 两次（相同区块）
   - [ ] 验证数据不重复

3. **部分重复测试**
   - [ ] 写入 block 1-100
   - [ ] 再次写入 block 50-150
   - [ ] 验证 block 50-100 不重复
   - [ ] 验证 block 101-150 正确写入

### 2. 数据完整性测试

#### 测试场景

1. **区块完整性**
   - [ ] 每个区块都有对应的 block 记录
   - [ ] 每个 transaction 都有对应的 trx 记录
   - [ ] 每个 operation 都有对应的 op 记录
   - [ ] Block 与 operations 数量一致

2. **数据关联性**
   - [ ] Operation 的 block_num 对应存在的 block
   - [ ] Operation 的 trx_id 对应存在的 transaction
   - [ ] Transaction 的 block_num 对应存在的 block

3. **数据准确性**
   - [ ] Operation 内容与 RPC 数据一致
   - [ ] Block 信息与 RPC 数据一致
   - [ ] Transaction 信息与 RPC 数据一致
   - [ ] Virtual operation 标记正确

### 3. 边界情况测试

#### 测试场景

1. **空数据测试**
   - [ ] 空区块处理
   - [ ] 无 transaction 区块处理
   - [ ] 无 operation transaction 处理

2. **特殊值测试**
   - [ ] Block 0 处理
   - [ ] 最大 block 号处理
   - [ ] Virtual operation（trx_index = -1）处理

3. **异常数据测试**
   - [ ] 无效 JSON 处理
   - [ ] 缺失字段处理
   - [ ] 类型错误处理

---

## 测试工具和脚本

### 1. 测试数据生成工具

#### 工具：`scripts/generate_test_data.go`

**功能：**
- 生成测试用的 operation JSON
- 生成不同场景的测试数据（正常、virtual、边界情况）

**使用示例：**
```bash
go run scripts/generate_test_data.go -count 1000 -output test_data/
```

### 2. Mock Plugin 工具

#### 工具：`scripts/mock_plugin.go`

**功能：**
- 模拟 steemd plugin 发送 operation JSON
- 支持指定发送频率
- 支持指定区块范围

**使用示例：**
```bash
go run scripts/mock_plugin.go \
  -endpoint http://localhost:8080/ingest/applied_op \
  -start 1 \
  -end 10000 \
  -rate 1000
```

### 3. 数据验证工具

#### 工具：`scripts/verify_data.go`

**功能：**
- 验证数据库中的数据完整性
- 检查缺失区块
- 检查数据关联性

**使用示例：**
```bash
go run scripts/verify_data.go -start 1 -end 100000
```

### 4. 性能测试脚本

#### 脚本：`scripts/benchmark.sh`

**功能：**
- 运行性能测试
- 收集性能指标
- 生成性能报告

**使用示例：**
```bash
./scripts/benchmark.sh -duration 1h -rate 5000
```

### 5. 集成测试脚本

#### 脚本：`scripts/integration_test.sh`

**功能：**
- 运行完整的集成测试流程
- 清理和准备测试环境
- 验证测试结果

**使用示例：**
```bash
./scripts/integration_test.sh
```

---

## 测试执行计划

### 阶段 1：单元测试（优先级：高）✅ 已完成

**时间估算：** 3-5 天
**实际完成时间：** 2026-01-09

**任务：**
1. [x] 编写 config 模块测试（11 个测试用例）
2. [x] 编写 model 模块测试（9 个测试用例）
3. [x] 编写 mongo 模块测试（17 个测试用例，使用 MongoDB 4.4）
4. [x] 编写 pipeline 模块测试（15 个测试用例）
5. [x] 编写 rpc 模块测试（18 个测试用例）
6. [x] 编写 checker 模块测试（11 个测试用例）
7. [x] 编写 metrics 模块测试（12 个测试用例）

**目标：** 单元测试覆盖率 ≥ 80%
**实际结果：** ✅ 93 个测试用例全部通过，核心模块覆盖率 100%

### 阶段 2：集成测试（优先级：高）

**时间估算：** 2-3 天

**任务：**
1. [ ] 编写 cold_ingest 集成测试
2. [ ] 编写 live_sync 集成测试
3. [ ] 编写 repair tool 集成测试
4. [ ] 编写测试工具和脚本

**目标：** 所有核心流程通过集成测试

### 阶段 3：端到端测试（优先级：中）

**时间估算：** 2-3 天

**任务：**
1. [ ] 完整工作流程测试
2. [ ] 幂等性测试
3. [ ] 数据完整性测试

**目标：** 端到端测试全部通过

### 阶段 4：性能测试（优先级：中）

**时间估算：** 2-3 天

**任务：**
1. [ ] 吞吐量测试
2. [ ] 延迟测试
3. [ ] 压力测试
4. [ ] 性能优化（如需要）

**目标：** 达到性能指标要求

### 阶段 5：回归测试（优先级：高）

**时间估算：** 1-2 天

**任务：**
1. [ ] 运行所有测试套件
2. [ ] 修复发现的问题
3. [ ] 更新测试文档

**目标：** 所有测试通过，准备发布

---

## 测试报告模板

### 测试执行报告

```
测试日期：YYYY-MM-DD
测试人员：XXX
测试环境：XXX

## 测试结果摘要
- 单元测试：通过/失败 (覆盖率: XX%)
- 集成测试：通过/失败
- 端到端测试：通过/失败
- 性能测试：通过/失败

## 发现的问题
1. [问题描述]
   - 严重程度：高/中/低
   - 状态：已修复/待修复
   - 修复方案：XXX

## 性能指标
- 吞吐量：XXX ops/sec
- 延迟（P95）：XXX ms
- 内存使用：XXX MB

## 结论
[测试结论和建议]
```

---

## 持续集成（CI）建议

### GitHub Actions 配置

```yaml
# .github/workflows/test.yml
name: Test

on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    services:
      mongodb:
        image: mongo:7
        ports:
          - 27017:27017
    steps:
      - uses: actions/checkout@v2
      - uses: actions/setup-go@v2
        with:
          go-version: '1.22'
      - run: go test -v -coverprofile=coverage.out ./...
      - run: go tool cover -html=coverage.out -o coverage.html
```

---

## 附录

### A. 测试数据示例

#### Operation JSON 示例

```json
{
  "block": {
    "num": 50000000,
    "id": "02faf080...",
    "timestamp": "2023-09-01T12:00:00"
  },
  "transaction": {
    "id": "abcd1234...",
    "index": 1
  },
  "operation": {
    "index": 0,
    "type": "transfer",
    "value": {
      "from": "alice",
      "to": "bob",
      "amount": "1.000 STEEM",
      "memo": "hello"
    }
  },
  "virtual": false
}
```

### B. 测试检查清单

- [ ] 所有单元测试通过
- [ ] 所有集成测试通过
- [ ] 所有端到端测试通过
- [ ] 性能测试达到指标
- [ ] 数据一致性验证通过
- [ ] 幂等性验证通过
- [ ] 文档更新完成
- [ ] 测试报告生成

---

**文档版本：** 1.0  
**最后更新：** 2026-01-09  
**维护者：** SteemDB Sync Team

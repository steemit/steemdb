# Layer 1: Raw Operation Sync 测试方案

## 测试概述

Layer 1 负责同步所有原始操作（包括 virtual operations）到 `operations` 集合。本测试方案涵盖单元测试、集成测试、功能测试和性能测试。

## 测试目标

1. **正确性验证**：确保所有操作（常规 + 虚拟）正确同步到数据库
2. **分表功能验证**：确保数据正确路由到对应的集合
3. **恢复机制验证**：确保服务重启后能正确恢复
4. **性能验证**：确保同步速度满足要求
5. **数据完整性验证**：确保数据不丢失、不重复

## 测试环境准备

### 1. 测试数据库配置

创建测试专用的 MongoDB 数据库：

```yaml
# configs/test-config.yaml
mongodb:
  uri: "mongodb://localhost:27017/steemdb_test"
  database: "steemdb_test"
  pool_size: 10
  timeout: 10s

sync:
  batch_size: 10  # 小批量用于测试
  block_interval: 1s
  start_block: 1
```

### 2. 测试数据准备

- 使用真实的 Steem API 节点（测试环境）
- 或使用 mock 数据（单元测试）

## 测试分类

### 一、单元测试 (Unit Tests)

#### 1.1 数据模型测试

**文件**: `internal/database/models_test.go`

```go
// TestRawOperation_Validation 测试 RawOperation 数据验证
func TestRawOperation_Validation(t *testing.T)

// TestSyncState_DefaultValues 测试 SyncState 默认值
func TestSyncState_DefaultValues(t *testing.T)

// TestBusinessProcessingState_DefaultValues 测试 BusinessProcessingState 默认值
func TestBusinessProcessingState_DefaultValues(t *testing.T)
```

**测试内容**:
- RawOperation 字段完整性
- 时间戳字段正确性
- 虚拟操作标识正确性

#### 1.2 分表逻辑测试

**文件**: `internal/database/mongodb_test.go`

```go
// TestGetOperationsCollectionName 测试集合名称生成
func TestGetOperationsCollectionName(t *testing.T) {
    tests := []struct {
        blockNum int64
        expected string
    }{
        {0, "operations_0_10000000"},
        {5000000, "operations_0_10000000"},
        {10000000, "operations_10000000_20000000"},
        {15000000, "operations_10000000_20000000"},
    }
    // ...
}

// TestGetCollectionsInRange 测试集合范围计算
func TestGetCollectionsInRange(t *testing.T)

// TestEnsureCollectionIndexes 测试索引创建
func TestEnsureCollectionIndexes(t *testing.T)
```

**测试内容**:
- 集合名称生成正确性
- 跨集合范围计算
- 索引创建逻辑

#### 1.3 RawSyncer 逻辑测试

**文件**: `internal/sync/raw_syncer_test.go`

```go
// TestRawSyncer_processOperations 测试操作处理逻辑
func TestRawSyncer_processOperations(t *testing.T)

// TestRawSyncer_processOperations_VirtualOps 测试虚拟操作处理
func TestRawSyncer_processOperations_VirtualOps(t *testing.T)

// TestRawSyncer_getBlockInfo 测试区块信息获取
func TestRawSyncer_getBlockInfo(t *testing.T)
```

**测试内容**:
- 操作数据转换正确性
- 虚拟操作标识处理
- 时间戳处理（常规操作 vs 虚拟操作）

### 二、集成测试 (Integration Tests)

#### 2.1 数据库操作测试

**文件**: `internal/database/mongodb_integration_test.go`

**前置条件**:
- 运行中的 MongoDB 实例
- 测试数据库已创建

```go
// TestInsertOperations_SingleCollection 测试单集合插入
func TestInsertOperations_SingleCollection(t *testing.T)

// TestInsertOperations_MultipleCollections 测试跨集合插入
func TestInsertOperations_MultipleCollections(t *testing.T)

// TestInsertOperations_DuplicatePrevention 测试重复数据防护
func TestInsertOperations_DuplicatePrevention(t *testing.T)

// TestQueryOperations_SingleCollection 测试单集合查询
func TestQueryOperations_SingleCollection(t *testing.T)

// TestQueryOperations_CrossCollection 测试跨集合查询
func TestQueryOperations_CrossCollection(t *testing.T)

// TestGetSyncState_UpdateSyncState 测试同步状态管理
func TestGetSyncState_UpdateSyncState(t *testing.T)

// TestUpdateSyncState_MaxOperator 测试 $max 操作符确保只增不减
func TestUpdateSyncState_MaxOperator(t *testing.T)
```

**测试内容**:
- 数据插入正确性
- 分表路由正确性
- 唯一索引防重复
- 查询功能正确性
- 同步状态更新逻辑

#### 2.2 RawSyncer 集成测试

**文件**: `internal/sync/raw_syncer_integration_test.go`

**前置条件**:
- 运行中的 MongoDB 实例
- 可访问的 Steem API 节点（或 mock）

```go
// TestRawSyncer_SyncBlocks_SmallRange 测试小范围区块同步
func TestRawSyncer_SyncBlocks_SmallRange(t *testing.T)

// TestRawSyncer_SyncBlocks_WithVirtualOps 测试包含虚拟操作的区块同步
func TestRawSyncer_SyncBlocks_WithVirtualOps(t *testing.T)

// TestRawSyncer_ResumeFromLastBlock 测试从上次区块恢复
func TestRawSyncer_ResumeFromLastBlock(t *testing.T)
```

**测试内容**:
- 区块同步功能
- 虚拟操作同步
- 恢复机制

### 三、功能测试 (Functional Tests)

#### 3.1 端到端同步测试

**文件**: `cmd/sync/e2e_test.go` 或独立测试脚本

**测试场景**:

1. **正常同步流程**
   ```bash
   # 启动同步服务
   ./bin/sync configs/test-config.yaml
   
   # 验证：
   # - 日志显示同步进度
   # - 数据库中有数据写入
   # - sync_state 正确更新
   ```

2. **虚拟操作同步**
   ```bash
   # 同步包含虚拟操作的区块范围
   # 验证：
   # - is_virtual=true 的操作正确存储
   # - virtual_op_num 字段正确
   # - trx_id 为空时使用 virtual_<block>_<op_num> 格式
   ```

3. **分表功能**
   ```bash
   # 同步跨集合边界的区块（如 9999999-10000001）
   # 验证：
   # - 数据正确路由到不同集合
   # - 每个集合都有正确的索引
   ```

4. **恢复机制**
   ```bash
   # 1. 启动同步，同步到区块 1000
   # 2. 停止服务
   # 3. 重新启动服务
   # 验证：
   # - 从区块 1001 继续同步（不重复同步）
   # - sync_state 正确恢复
   ```

5. **重复数据防护**
   ```bash
   # 多次同步相同区块
   # 验证：
   # - 唯一索引防止重复
   # - 数据不重复插入
   ```

#### 3.2 数据完整性验证

**MongoDB 查询验证**:

```javascript
// 1. 验证操作数量
db.operations_0_10000000.countDocuments()

// 2. 验证虚拟操作
db.operations_0_10000000.find({is_virtual: true}).count()

// 3. 验证时间戳
db.operations_0_10000000.find({
  timestamp: {$exists: true},
  created_at: {$exists: true}
}).count()

// 4. 验证唯一索引
// 尝试插入重复数据应该失败
db.operations_0_10000000.insertOne({
  block_num: 1,
  trx_id: "test",
  op_in_trx: 0,
  is_virtual: false,
  virtual_op_num: 0
})
// 应该报错：duplicate key error

// 5. 验证索引存在
db.operations_0_10000000.getIndexes()
```

### 四、性能测试 (Performance Tests)

#### 4.1 同步速度测试

**测试指标**:
- 每秒处理的区块数
- 每秒处理的操作数
- 数据库写入速度

**测试脚本**: `scripts/performance_test_layer1.sh`

```bash
#!/bin/bash
# 性能测试脚本

# 1. 清理测试数据
# 2. 启动同步服务
# 3. 运行 5 分钟
# 4. 统计：
#    - 同步的区块数
#    - 同步的操作数
#    - 平均速度
```

#### 4.2 批量插入性能测试

**文件**: `internal/database/mongodb_bench_test.go`

```go
// BenchmarkInsertOperations_SmallBatch 小批量插入性能
func BenchmarkInsertOperations_SmallBatch(b *testing.B)

// BenchmarkInsertOperations_LargeBatch 大批量插入性能
func BenchmarkInsertOperations_LargeBatch(b *testing.B)

// BenchmarkQueryOperations_SingleCollection 单集合查询性能
func BenchmarkQueryOperations_SingleCollection(b *testing.B)

// BenchmarkQueryOperations_CrossCollection 跨集合查询性能
func BenchmarkQueryOperations_CrossCollection(b *testing.B)
```

#### 4.3 并发测试

```go
// TestInsertOperations_Concurrent 测试并发插入
func TestInsertOperations_Concurrent(t *testing.T)

// TestUpdateSyncState_Concurrent 测试并发更新同步状态
func TestUpdateSyncState_Concurrent(t *testing.T)
```

### 五、边界条件测试 (Edge Cases)

#### 5.1 边界区块测试

```go
// TestGetOperationsCollectionName_Boundary 测试边界区块
func TestGetOperationsCollectionName_Boundary(t *testing.T) {
    // 测试 0, 10000000, 20000000 等边界值
}

// TestQueryOperations_Boundary 测试边界查询
func TestQueryOperations_Boundary(t *testing.T) {
    // 测试跨集合边界的查询
}
```

#### 5.2 异常情况测试

```go
// TestInsertOperations_EmptySlice 测试空切片
func TestInsertOperations_EmptySlice(t *testing.T)

// TestInsertOperations_NilOperation 测试 nil 操作
func TestInsertOperations_NilOperation(t *testing.T)

// TestSyncBlocks_NoNewBlocks 测试无新区块
func TestSyncBlocks_NoNewBlocks(t *testing.T)

// TestSyncBlocks_APIError 测试 API 错误处理
func TestSyncBlocks_APIError(t *testing.T)

// TestSyncBlocks_DatabaseError 测试数据库错误处理
func TestSyncBlocks_DatabaseError(t *testing.T)
```

### 六、测试数据准备

#### 6.1 Mock 数据生成

**文件**: `internal/sync/testdata/mock_operations.go`

```go
// GenerateMockOperations 生成模拟操作数据
func GenerateMockOperations(blockNum int64, count int) []*database.RawOperation

// GenerateMockVirtualOperations 生成模拟虚拟操作
func GenerateMockVirtualOperations(blockNum int64, count int) []*database.RawOperation
```

#### 6.2 测试数据清理

**文件**: `internal/database/test_helpers.go`

```go
// CleanupTestDatabase 清理测试数据库
func CleanupTestDatabase(ctx context.Context, db *MongoDB) error

// SetupTestDatabase 设置测试数据库
func SetupTestDatabase(ctx context.Context, db *MongoDB) error
```

## 测试执行流程

### 1. 单元测试

```bash
# 运行所有单元测试
go test ./internal/database/... -v
go test ./internal/sync/... -v

# 运行特定测试
go test ./internal/database/... -run TestGetOperationsCollectionName -v
```

### 2. 集成测试

```bash
# 启动测试 MongoDB
docker-compose -f docker-compose.test.yml up -d

# 运行集成测试
go test ./internal/database/... -tags=integration -v
go test ./internal/sync/... -tags=integration -v

# 清理
docker-compose -f docker-compose.test.yml down -v
```

### 3. 功能测试

```bash
# 使用测试配置启动服务
./bin/sync configs/test-config.yaml

# 在另一个终端验证数据
mongo steemdb_test --eval "db.operations_0_10000000.find().limit(5).pretty()"
```

### 4. 性能测试

```bash
# 运行性能测试
./scripts/performance_test_layer1.sh

# 运行基准测试
go test ./internal/database/... -bench=. -benchmem
```

## 测试检查清单

### 数据正确性
- [ ] 所有操作（常规 + 虚拟）都正确同步
- [ ] 操作数据完整（所有字段都有值）
- [ ] 时间戳正确（链上时间 vs 入库时间）
- [ ] 虚拟操作标识正确（is_virtual, virtual_op_num）
- [ ] 交易 ID 处理正确（虚拟操作使用 virtual_<block>_<op> 格式）

### 分表功能
- [ ] 集合名称生成正确
- [ ] 数据正确路由到对应集合
- [ ] 跨集合查询功能正常
- [ ] 每个集合都有正确的索引

### 恢复机制
- [ ] 服务重启后从上次区块继续
- [ ] sync_state 正确恢复
- [ ] 不会重复同步已同步的区块

### 数据完整性
- [ ] 唯一索引防止重复
- [ ] 数据不丢失
- [ ] 数据不重复

### 性能
- [ ] 同步速度满足要求（> 10 区块/秒）
- [ ] 批量插入性能良好
- [ ] 查询性能可接受

### 错误处理
- [ ] API 错误正确处理
- [ ] 数据库错误正确处理
- [ ] 网络错误正确处理
- [ ] 优雅降级和恢复

## 测试报告模板

```markdown
# Layer 1 测试报告

## 测试环境
- Go 版本: 1.24.0
- MongoDB 版本: x.x.x
- 测试时间: YYYY-MM-DD

## 测试结果

### 单元测试
- 通过: X/Y
- 失败: 0

### 集成测试
- 通过: X/Y
- 失败: 0

### 功能测试
- [ ] 正常同步流程
- [ ] 虚拟操作同步
- [ ] 分表功能
- [ ] 恢复机制
- [ ] 重复数据防护

### 性能测试
- 同步速度: X 区块/秒
- 操作处理速度: X 操作/秒
- 数据库写入速度: X 操作/秒

## 问题记录
1. [问题描述]
   - 严重程度: High/Medium/Low
   - 状态: Open/Fixed

## 结论
[测试结论]
```

## 持续集成 (CI)

### GitHub Actions 配置示例

```yaml
# .github/workflows/test-layer1.yml
name: Test Layer 1

on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    services:
      mongodb:
        image: mongo:latest
        ports:
          - 27017:27017
    steps:
      - uses: actions/checkout@v2
      - uses: actions/setup-go@v2
        with:
          go-version: '1.24'
      - run: go test ./internal/database/... -v
      - run: go test ./internal/sync/... -v
      - run: go test ./internal/database/... -tags=integration -v
```

## 测试工具和脚本

### 1. 测试数据生成器

创建 `scripts/generate_test_data.sh` 用于生成测试数据。

### 2. 测试验证脚本

创建 `scripts/validate_layer1.sh` 用于验证测试结果。

### 3. 性能监控脚本

创建 `scripts/monitor_performance.sh` 用于监控同步性能。

## 注意事项

1. **测试隔离**: 每个测试应该独立，不依赖其他测试
2. **数据清理**: 测试后清理测试数据，避免影响后续测试
3. **Mock 使用**: 单元测试使用 mock，集成测试使用真实数据库
4. **性能基准**: 建立性能基准，监控性能回归
5. **错误场景**: 充分测试错误场景和边界条件


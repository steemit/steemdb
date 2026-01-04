# Layer 1: Raw Operation Sync 测试指南

## 概述

Layer 1 负责同步所有原始操作（包括 virtual operations）到 `operations` 集合，不进行任何业务处理。

## 功能验证

### 1. 编译检查

```bash
cd /home/ety001/workspace/steemdb/steemdb-sync
go build -o bin/sync ./cmd/sync
```

如果编译成功，说明代码没有语法错误。

### 2. 配置检查

确保 `configs/config.yaml` 中的配置正确：

```yaml
mongodb:
  uri: "mongodb://localhost:27017/steemdb"  # 根据实际情况修改
  database: "steemdb"

sync:
  batch_size: 50
  block_interval: 3s
  start_block: 1  # 从区块 1 开始同步（或指定其他区块）
```

### 3. 运行测试

#### 3.1 启动 MongoDB

确保 MongoDB 正在运行：

```bash
# 使用 Docker
docker-compose up -d mongo

# 或直接运行 MongoDB
mongod --dbpath /path/to/data
```

#### 3.2 运行同步服务

```bash
cd /home/ety001/workspace/steemdb/steemdb-sync
./bin/sync configs/config.yaml
```

#### 3.3 观察日志

查看日志输出，应该看到：

```
Starting SteemDB Sync Service (Layer 1: Raw Operation Sync)
Resuming from last synced block: start_block=XXX, last_block=YYY
Processing batch: start_block=XXX, end_block=YYY
Saved operations for block: block=XXX, count=YYY
```

### 4. 数据库验证

#### 4.1 检查同步状态

```javascript
// 连接到 MongoDB
use steemdb

// 查看同步状态
db.sync_state.find().pretty()
```

应该看到类似：

```json
{
  "_id": "current",
  "last_block": 12345,
  "last_irreversible_block": 12340,
  "updated_at": ISODate("2025-01-XX...")
}
```

#### 4.2 检查 operations 集合

```javascript
// 查看 operations 集合（根据区块高度自动分表）
// 例如：operations_0_10000000, operations_10000000_20000000

// 查看第一个集合的数据
db.operations_0_10000000.find().limit(5).pretty()

// 统计操作数量
db.operations_0_10000000.countDocuments()

// 查看某个区块的操作
db.operations_0_10000000.find({block_num: 1}).pretty()
```

#### 4.3 验证数据结构

检查 RawOperation 的字段是否正确：

```javascript
db.operations_0_10000000.findOne()
```

应该包含以下字段：
- `_id`: ObjectID
- `block_num`: 区块高度
- `block_id`: 区块哈希
- `trx_id`: 交易ID
- `trx_in_block`: 交易在区块中的索引
- `op_in_trx`: 操作在交易中的索引
- `op_type`: 操作类型
- `op_data`: 操作数据（完整）
- `is_virtual`: 是否为 virtual operation
- `virtual_op_num`: virtual operation 编号
- `timestamp`: 链上时间
- `created_at`: 入库时间

#### 4.4 验证索引

```javascript
// 查看索引
db.operations_0_10000000.getIndexes()
```

应该包含：
- 唯一索引：`{block_num: 1, trx_id: 1, op_in_trx: 1, is_virtual: 1, virtual_op_num: 1}`
- 查询索引：`{block_num: 1, timestamp: -1}`, `{timestamp: 1}`, `{trx_id: 1}`, `{op_type: 1}`

### 5. 功能测试场景

#### 5.1 正常同步

1. 启动服务
2. 观察日志，确认区块正在同步
3. 检查数据库，确认数据正在写入

#### 5.2 恢复机制测试

1. 启动服务，同步一些区块
2. 停止服务（Ctrl+C）
3. 重新启动服务
4. 确认从上次结束的区块继续同步（不会重复同步）

#### 5.3 分表验证

1. 同步超过 1000 万区块（或修改 `CollectionRangeSize` 为更小的值进行测试）
2. 确认新的集合自动创建（如 `operations_10000000_20000000`）
3. 确认数据正确路由到对应的集合

#### 5.4 虚拟操作验证

1. 查询包含虚拟操作的区块
2. 确认 `is_virtual: true` 的操作正确存储
3. 确认 `virtual_op_num` 字段正确

### 6. 性能监控

#### 6.1 同步速度

观察日志中的同步速度：
- 每秒处理的区块数
- 每秒处理的操作数

#### 6.2 数据库性能

```javascript
// 查看集合统计信息
db.operations_0_10000000.stats()

// 查看索引使用情况
db.operations_0_10000000.aggregate([
  { $indexStats: {} }
])
```

### 7. 常见问题排查

#### 7.1 连接失败

- 检查 MongoDB URI 配置
- 检查网络连接
- 检查 MongoDB 是否运行

#### 7.2 同步停止

- 检查 Steem API 节点是否可用
- 查看错误日志
- 检查网络连接

#### 7.3 数据重复

- 检查唯一索引是否正确创建
- 确认 upsert 逻辑正常工作

#### 7.4 性能问题

- 检查索引是否正确创建
- 考虑调整 batch_size
- 检查 MongoDB 性能

### 8. 测试数据清理

如果需要重新测试：

```javascript
// 删除所有 operations 集合
db.getCollectionNames().forEach(function(collection) {
  if (collection.startsWith('operations_')) {
    db[collection].drop()
  }
})

// 重置同步状态
db.sync_state.deleteMany({})
```

## 下一步

Layer 1 测试通过后，可以继续实现和测试：
- **Layer 2**: Business Logic Processing（业务逻辑处理）
- **Layer 3**: Backfill Tool（数据补齐工具）


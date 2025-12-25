# Layer 1 存储空间评估与分表分析

## 一、数据规模估算

### 1.1 基础数据

- **区块链高度**: > 100,000,000 区块
- **block_log 大小**: 378 GB
- **平均每个区块 operations 数量**: ~24 个（基于 2016-2019 数据，包含 virtual operations）
- **总 operations 数量估算**: 100,000,000 × 24 = **2,400,000,000** (24 亿条)

### 1.2 单个 Operation 文档大小估算

根据 Operation 数据结构：

```go
type Operation struct {
    ID              primitive.ObjectID `bson:"_id"`              // 12 bytes
    BlockNum        int64             `bson:"block_num"`          // 8 bytes
    BlockID         string            `bson:"block_id"`          // 64 bytes (SHA256 hash)
    TrxID           string            `bson:"trx_id"`            // 40 bytes (transaction ID)
    TrxInBlock      int               `bson:"trx_in_block"`      // 4 bytes
    OpInTrx         int               `bson:"op_in_trx"`        // 4 bytes
    OpType          string            `bson:"op_type"`          // ~20 bytes (平均)
    OpData          bson.M            `bson:"op_data"`           // 可变，平均 ~500 bytes
    IsVirtual       bool              `bson:"is_virtual"`        // 1 byte
    VirtualOpNum    int               `bson:"virtual_op_num"`    // 4 bytes
    Timestamp       time.Time         `bson:"timestamp"`         // 8 bytes
    CreatedAt       time.Time         `bson:"created_at"`        // 8 bytes
}
```

**BSON 存储开销**：
- 字段名开销：每个字段名平均 ~15 bytes（11 个字段 × 15 = 165 bytes）
- BSON 类型标记：每个字段 1 byte（11 bytes）
- 文档结构开销：~20 bytes
- 实际数据：~677 bytes

**单个文档估算大小**：
- 最小（简单 operation，如 vote）：~300 bytes
- 平均（中等 operation，如 comment）：~700 bytes
- 最大（复杂 operation，如 comment with long content）：~2000 bytes
- **平均估算**: ~800 bytes/文档（包含 BSON 开销）

### 1.3 总存储空间估算

#### 原始数据存储
- 总 operations: 2,400,000,000
- 平均文档大小: 800 bytes
- **原始数据大小**: 2,400,000,000 × 800 bytes = **1,920 GB** ≈ **1.9 TB**

#### MongoDB 存储开销
MongoDB 的存储开销包括：
- **索引空间**: 约占总数据的 20-30%
- **WiredTiger 压缩**: 默认使用 snappy 压缩，压缩比约 1.5:1
- **碎片空间**: 约占总数据的 5-10%

**实际存储空间估算**：
- 原始数据: 1,920 GB
- 压缩后数据: 1,920 GB / 1.5 ≈ **1,280 GB**
- 索引空间: 1,280 GB × 25% ≈ **320 GB**
- 碎片空间: 1,280 GB × 5% ≈ **64 GB**
- **总存储空间**: 1,280 + 320 + 64 ≈ **1,664 GB** ≈ **1.6 TB**

### 1.4 索引空间详细估算

根据计划中的索引设计：

1. **唯一索引**: `{block_num: 1, trx_id: 1, op_in_trx: 1, is_virtual: 1, virtual_op_num: 1}`
   - 大小: ~40 bytes/文档
   - 总大小: 2,400,000,000 × 40 = 96 GB

2. **查询索引**: `{block_num: 1, timestamp: -1}`
   - 大小: ~16 bytes/文档
   - 总大小: 2,400,000,000 × 16 = 38.4 GB

3. **查询索引**: `{timestamp: 1}`
   - 大小: ~12 bytes/文档
   - 总大小: 2,400,000,000 × 12 = 28.8 GB

4. **查询索引**: `{trx_id: 1}`
   - 大小: ~20 bytes/文档
   - 总大小: 2,400,000,000 × 20 = 48 GB

5. **查询索引**: `{op_type: 1}`
   - 大小: ~12 bytes/文档
   - 总大小: 2,400,000,000 × 12 = 28.8 GB

**索引总大小**: 96 + 38.4 + 28.8 + 48 + 28.8 = **240 GB**

## 二、MongoDB 集合大小限制分析

### 2.1 MongoDB 技术限制

- **单个集合最大文档数**: 无硬性限制（理论上）
- **单个文档最大大小**: 16 MB
- **单个数据库最大大小**: 无硬性限制（受文件系统限制）
- **WiredTiger 缓存**: 默认 50% 可用内存（可配置）

### 2.2 性能考虑

对于 24 亿条文档的集合，主要性能问题：

1. **索引维护开销**：
   - 索引越大，插入/更新/删除操作越慢
   - 索引重建时间越长

2. **查询性能**：
   - 即使有索引，24 亿条文档的集合查询仍可能较慢
   - 范围查询（如按时间范围）需要扫描大量数据

3. **内存压力**：
   - WiredTiger 缓存可能无法容纳所有热数据
   - 频繁的磁盘 I/O 会影响性能

4. **备份和恢复**：
   - 1.6 TB 的数据备份和恢复时间较长
   - 增量备份也需要处理大量数据

## 三、分表（分集合）方案分析

### 3.1 是否需要分表？

**建议：需要分表**

**理由**：
1. **性能优化**：24 亿条文档的单个集合，即使有索引，查询性能也会下降
2. **维护便利**：分表后可以按时间段独立维护、备份、恢复
3. **扩展性**：未来可以按需扩展，不需要一次性处理所有数据
4. **故障隔离**：单个集合的问题不会影响整个系统

### 3.2 分表策略

#### 方案 1：按区块高度范围分表（推荐）

**策略**：按区块高度范围分表，每个集合存储一定范围的区块数据

**优点**：
- 查询时可以根据区块高度快速定位到对应的集合
- 可以按时间段独立维护
- 支持并行处理

**实现**：
- `operations_0_10m` - 区块 0 到 10,000,000
- `operations_10m_20m` - 区块 10,000,000 到 20,000,000
- `operations_20m_30m` - 区块 20,000,000 到 30,000,000
- ...（每 1000 万区块一个集合）

**每个集合规模**：
- 区块数: 10,000,000
- Operations 数: 10,000,000 × 24 = 240,000,000
- 数据大小: 240,000,000 × 800 bytes ≈ 192 GB
- 压缩后: 192 GB / 1.5 ≈ 128 GB
- 索引: 128 GB × 25% ≈ 32 GB
- **总大小**: 128 + 32 ≈ **160 GB/集合**

**集合数量**：100,000,000 / 10,000,000 = **10 个集合**

#### 方案 2：按时间分表

**策略**：按年份或月份分表

**优点**：
- 符合时间查询习惯
- 可以按时间段归档旧数据

**缺点**：
- 需要根据 timestamp 查询时跨多个集合
- 区块高度和时间不是严格线性关系

#### 方案 3：按区块高度 + 哈希分表（Sharding）

**策略**：使用 MongoDB Sharding，按区块高度分片

**优点**：
- MongoDB 原生支持，自动管理
- 可以水平扩展

**缺点**：
- 需要配置 Sharding 集群
- 运维复杂度较高

### 3.3 推荐方案：按区块高度范围分表

**实现细节**：

1. **集合命名规则**：
   ```
   operations_{start_block}_{end_block}
   例如：operations_0_10000000
   ```

2. **查询路由**：
   ```go
   func GetCollectionName(blockNum int64) string {
       rangeSize := int64(10_000_000)
       startBlock := (blockNum / rangeSize) * rangeSize
       endBlock := startBlock + rangeSize
       return fmt.Sprintf("operations_%d_%d", startBlock, endBlock)
   }
   ```

3. **索引设计**：
   - 每个集合都有相同的索引结构
   - 索引大小：32 GB/集合（可接受）

4. **查询优化**：
   - 单集合查询：直接查询对应集合
   - 跨集合查询：并行查询多个集合，合并结果

5. **维护策略**：
   - 可以按集合独立备份
   - 可以按集合独立压缩
   - 可以按集合独立删除旧数据

## 四、存储空间对比

### 4.1 单表方案

- **总存储空间**: ~1.6 TB
- **索引总大小**: ~240 GB
- **单集合文档数**: 24 亿
- **查询性能**: 较差（需要扫描大量数据）
- **维护难度**: 高（备份/恢复时间长）

### 4.2 分表方案（10 个集合）

- **总存储空间**: ~1.6 TB（相同，无额外开销）
- **索引总大小**: ~240 GB（分散到 10 个集合）
- **单集合文档数**: 2.4 亿（减少 10 倍）
- **查询性能**: 较好（查询范围缩小 10 倍）
- **维护难度**: 低（可以按集合独立维护）

## 五、实施建议

### 5.1 分表粒度选择

**推荐：每 1000 万区块一个集合**

**理由**：
- 每个集合约 160 GB，在可管理范围内
- 10 个集合数量适中，不会过度分散
- 查询时最多需要查询 1-2 个集合

**备选方案**：
- 如果未来数据增长，可以调整为每 5000 万区块一个集合（2 个集合）
- 或者每 2000 万区块一个集合（5 个集合）

### 5.2 实施步骤

1. **Phase 1**: 实现分表逻辑
   - 创建集合路由函数
   - 修改 `InsertOperations` 支持分表
   - 修改查询接口支持跨集合查询

2. **Phase 2**: 数据迁移（如果需要）
   - 由于是全新项目，数据库会清空，无需迁移
   - 直接按新方案开始同步

3. **Phase 3**: 监控和优化
   - 监控每个集合的大小
   - 监控查询性能
   - 根据实际情况调整分表粒度

### 5.3 代码实现要点

```go
// 集合路由
func (m *MongoDB) GetOperationsCollection(blockNum int64) *mongo.Collection {
    collectionName := GetCollectionName(blockNum)
    return m.db.Collection(collectionName)
}

// 跨集合查询
func (m *MongoDB) QueryOperationsAcrossCollections(
    ctx context.Context,
    startBlock, endBlock int64,
    filter bson.M,
) ([]*Operation, error) {
    // 确定需要查询的集合
    collections := GetCollectionsInRange(startBlock, endBlock)
    
    // 并行查询所有相关集合
    var results []*Operation
    for _, coll := range collections {
        ops, err := m.queryCollection(ctx, coll, filter)
        if err != nil {
            return nil, err
        }
        results = append(results, ops...)
    }
    
    // 按 timestamp 排序
    sort.Slice(results, func(i, j int) bool {
        return results[i].Timestamp.Before(results[j].Timestamp)
    })
    
    return results, nil
}
```

## 六、总结

### 6.1 存储空间评估

- **总 operations 数量**: 24 亿条
- **总存储空间**: ~1.6 TB（包含索引和压缩）
- **单集合存储**: 不推荐（性能和维护问题）

### 6.2 分表建议

**强烈建议实施分表**，推荐方案：
- **分表策略**: 按区块高度范围分表
- **分表粒度**: 每 1000 万区块一个集合
- **集合数量**: 10 个集合
- **单集合大小**: ~160 GB
- **优势**: 提升查询性能，降低维护难度，支持独立备份和恢复

### 6.3 未来扩展

- 如果区块链继续增长到 2 亿、3 亿区块，可以：
  1. 继续按相同粒度创建新集合
  2. 或者调整分表粒度（如每 2000 万区块一个集合）
  3. 考虑使用 MongoDB Sharding 进行水平扩展

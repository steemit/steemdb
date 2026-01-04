# Layer 1 测试快速开始指南

## 快速测试命令

### 1. 运行所有单元测试

```bash
# 测试数据库相关功能
go test ./internal/database/... -v

# 测试同步相关功能
go test ./internal/sync/... -v

# 运行所有测试
go test ./... -v
```

### 2. 运行特定测试

```bash
# 测试集合名称生成
go test ./internal/database/... -run TestGetOperationsCollectionName -v

# 测试集合范围计算
go test ./internal/database/... -run TestGetCollectionsInRange -v

# 测试 RawOperation 结构
go test ./internal/database/... -run TestRawOperation -v
```

### 3. 使用测试脚本

```bash
# 运行快速测试脚本
./scripts/test_layer1.sh
```

## 集成测试（需要 MongoDB）

### 1. 启动测试 MongoDB

```bash
# 使用 Docker Compose
docker-compose -f docker-compose.test.yml up -d

# 或直接启动 MongoDB
mongod --dbpath /tmp/mongodb_test --port 27018
```

### 2. 运行集成测试

```bash
# 设置测试环境变量
export MONGODB_URI="mongodb://localhost:27017/steemdb_test"

# 运行集成测试（需要添加 integration build tag）
go test ./internal/database/... -tags=integration -v
```

### 3. 手动功能测试

```bash
# 1. 启动同步服务（使用测试配置）
./bin/sync configs/test-config.yaml

# 2. 在另一个终端验证数据
mongosh steemdb_test
# 或
mongo steemdb_test

# 3. 查看同步状态
db.sync_state.find().pretty()

# 4. 查看操作数据
db.operations_0_10000000.find().limit(5).pretty()

# 5. 统计操作数量
db.operations_0_10000000.countDocuments()

# 6. 查看虚拟操作
db.operations_0_10000000.find({is_virtual: true}).limit(5).pretty()
```

## 测试检查清单

### 基础功能
- [ ] 集合名称生成正确
- [ ] 集合范围计算正确
- [ ] RawOperation 结构完整
- [ ] 虚拟操作标识正确

### 数据库操作（需要 MongoDB）
- [ ] 数据插入成功
- [ ] 分表路由正确
- [ ] 索引创建成功
- [ ] 唯一索引防重复
- [ ] 查询功能正常

### 同步功能（需要 Steem API）
- [ ] 区块同步成功
- [ ] 虚拟操作同步成功
- [ ] 恢复机制正常
- [ ] 数据不重复

## 常见问题

### Q: 测试失败，提示 MongoDB 连接错误
A: 确保 MongoDB 正在运行，或使用 mock 进行单元测试

### Q: 如何跳过需要外部依赖的测试？
A: 使用 build tags：
```bash
# 只运行不需要外部依赖的测试
go test ./internal/database/... -v
```

### Q: 如何清理测试数据？
A: 在 MongoDB shell 中执行：
```javascript
use steemdb_test
db.getCollectionNames().forEach(function(collection) {
  if (collection.startsWith('operations_')) {
    db[collection].drop()
  }
})
db.sync_state.deleteMany({})
```

## 下一步

完成基础测试后，参考 `docs/LAYER1_TEST_PLAN.md` 进行更全面的测试。


# SteemDB Sync 验证指南

本文档说明如何验证 `steemdb-sync` 服务是否符合预期。

## 快速验证清单

### ✅ 1. 服务健康状态
```bash
docker compose ps steemdb-sync
```
**预期结果**: `STATUS` 应该显示 `(healthy)`

### ✅ 2. 时间解析错误检查（最重要）
```bash
# 检查最近10分钟内是否有时间解析错误（应该返回 0）
docker compose logs --since 10m steemdb-sync 2>&1 | grep -iE 'parsing time.*as.*2006|cannot parse.*as.*time' | wc -l
```
**预期结果**: `0` - 没有时间解析错误

### ✅ 3. 服务启动确认
```bash
docker compose logs steemdb-sync 2>&1 | grep -i 'started successfully'
```
**预期结果**: 应该看到 `"SteemDB Sync Service started successfully"`

### ✅ 4. 同步统计信息
```bash
docker compose logs --tail=200 steemdb-sync 2>&1 | grep 'Block sync statistics' | tail -1
```
**预期结果**: 应该看到包含 `blocks_processed`, `operations_processed`, `last_block` 的统计信息

### ✅ 5. 关键错误检查
```bash
# 检查关键错误（排除正常的重复键错误）
docker compose logs --tail=200 steemdb-sync 2>&1 | grep -i error | \
  grep -v 'duplicate key' | \
  grep -v 'multi-key map' | \
  grep -v 'BadValue.*_id index' | \
  tail -10
```
**预期结果**: 
- 不应该有时间解析相关的错误
- 可能看到 `curation_rewards` 类型不匹配的警告（这是另一个问题，不影响时间解析）

### ✅ 6. 历史服务和见证人服务
```bash
# 检查历史服务
docker compose logs --tail=100 steemdb-sync 2>&1 | grep -iE 'history service|account history' | tail -1

# 检查见证人服务
docker compose logs --tail=100 steemdb-sync 2>&1 | grep -iE 'witness service|witness missed' | tail -1
```
**预期结果**: 应该看到服务正在运行的日志

### ✅ 7. Prometheus 指标（可选）
```bash
docker compose exec -T steemdb-sync curl -s http://localhost:9091/metrics 2>/dev/null | \
  grep -E 'steemdb_blocks_processed_total|steemdb_operations_processed_total'
```
**预期结果**: 应该看到指标数据

## 使用自动化检查脚本

已创建 `check_steemdb_sync.sh` 脚本，可以一键检查所有关键指标：

```bash
./check_steemdb_sync.sh
```

脚本会检查：
- ✅ 服务健康状态
- ✅ 时间解析错误（应该为 0）
- ✅ 服务启动状态
- ✅ 同步统计信息
- ✅ 关键错误
- ✅ 历史服务和见证人服务状态
- ✅ Prometheus 指标

## 验证时间解析修复的关键指标

### 修复前的问题
- ❌ 日志中会出现大量 `parsing time "2020-03-21T13:04:57" as "2006-01-02T15:04:05Z07:00": cannot parse "" as "Z07:00"` 错误
- ❌ 服务可能因为时间解析失败而无法正常工作

### 修复后的预期
- ✅ **零时间解析错误**: `grep -iE 'parsing time.*as.*2006|cannot parse.*as.*time' | wc -l` 应该返回 `0`
- ✅ 服务正常运行，能够处理 Steem API 返回的各种时间格式
- ✅ 区块同步、账户历史、见证人监控等功能正常工作

## 常见问题

### Q: 看到 "duplicate key" 错误是正常的吗？
A: 是的，这是正常的。这些错误表示服务尝试插入已经存在的区块，说明同步进度正常。

### Q: 看到 "curation_rewards" 类型不匹配警告？
A: 这是另一个问题（Steem API 返回数字但代码期望字符串），不影响时间解析功能。可以后续修复。

### Q: 如何确认时间解析真的修复了？
A: 最关键的是检查日志中**没有**时间解析错误。如果之前有大量时间解析错误，现在应该完全消失。

## 验证示例输出

### 成功的验证结果示例

```bash
$ docker compose ps steemdb-sync
NAME                     STATUS
steemdb-steemdb-sync-1   Up 11 minutes (healthy)

$ docker compose logs --since 10m steemdb-sync 2>&1 | grep -iE 'parsing time.*as.*2006|cannot parse.*as.*time' | wc -l
0

$ docker compose logs steemdb-sync 2>&1 | grep -i 'started successfully'
{"level":"info","timestamp":"2025-12-08T07:29:09.358Z","message":"SteemDB Sync Service started successfully"}
```

这些结果表示服务运行正常，时间解析问题已修复。


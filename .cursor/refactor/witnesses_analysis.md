# witnesses.py 详细分析报告

## 1. 脚本概述

`witnesses.py` 是一个用于监控和更新 Steem 区块链见证人（Witness）信息的定时任务服务。该脚本的主要功能包括：

- **见证人信息更新**：定期获取并更新 Steem 网络中前 100 名见证人的详细信息
- **见证人历史快照**：为每个见证人创建每日快照，记录历史状态
- **见证人错失区块监控**：监控见证人错失区块的情况，记录错失事件

见证人在 Steem 网络中负责生产区块，是网络运行的关键节点。该脚本帮助跟踪见证人的状态变化和性能表现。

## 2. 依赖分析

### 2.1 第三方库依赖

```python
- steem: Steem 区块链 Python SDK，用于与 Steem 节点通信
- pymongo: MongoDB 数据库驱动
- apscheduler: 后台任务调度器，用于定时执行任务
- collections: Python 标准库（已导入但未使用）
- pprint: 用于格式化输出（调试用）
```

### 2.2 系统依赖

- Python 3.x
- MongoDB 数据库
- 网络连接到 Steem 节点

### 2.3 关键依赖说明

**APScheduler**：
- 提供后台任务调度功能
- 支持定时执行任务
- 适合长期运行的服务

## 3. 配置管理

### 3.1 环境变量配置

脚本通过环境变量进行配置：

| 环境变量 | 默认值 | 说明 |
|---------|--------|------|
| `STEEMD_URL` | `https://api.steemit.com` | Steem 节点 URL |
| `MONGODB` | 无（必需） | MongoDB 连接字符串 |

### 3.2 配置加载逻辑

```python
env_dist = os.environ
steemd_url = env_dist.get('STEEMD_URL')
if steemd_url == None or steemd_url == "":
    steemd_url = 'https://api.steemit.com'

mongodb_url = env_dist.get('MONGODB')
if mongodb_url == None or mongodb_url == "":
    print('NEED MONGODB')
    exit()
```

**特点**：
- 简单的环境变量读取
- MongoDB URL 为必需项，缺失会导致脚本退出
- Steem 节点 URL 有默认值

### 3.3 配置验证

- MongoDB URL 为必需项，缺失会导致脚本退出
- Steem 节点 URL 默认为 `https://api.steemit.com`

## 4. 核心功能模块

### 4.1 全局变量

**misses 字典**：
```python
misses = {}
```
- 存储每个见证人的当前错失区块数
- 键：见证人账户名
- 值：错失区块总数
- 用于检测错失区块数的变化

### 4.2 update_witnesses() 函数

**功能**：更新见证人信息并创建历史快照

**处理流程**：
1. 获取当前日期
2. 从 Steem 节点获取前 100 名见证人（按投票数排序）
3. 清空 `witness` 集合（重新构建）
4. 遍历每个见证人：
   - **数据类型转换**：
     - 数值字段：`virtual_last_update`, `virtual_position`, `virtual_scheduled_time`, `votes` → float
     - 日期字段：`last_sbd_exchange_update` → datetime
   - **保存当前状态**：更新到 `witness` 集合
   - **创建历史快照**：
     - 构建快照 ID：`owner|YYYYMMDD`
     - 添加创建时间戳
     - 保存到 `witness_history` 集合

**关键代码**：
```python
def update_witnesses():
    now = datetime.now().date()
    users = rpc.get_witnesses_by_vote('', 100)
    
    db.witness.remove({})  # 清空集合
    
    for user in users:
        # 数据类型转换
        for key in ['virtual_last_update', 'virtual_position', 'virtual_scheduled_time', 'votes']:
            user[key] = float(user[key])
        for key in ['last_sbd_exchange_update']:
            user[key] = datetime.strptime(user[key], "%Y-%m-%dT%H:%M:%S")
        
        # 保存当前状态
        db.witness.update({'_id': user['owner']}, user, upsert=True)
        
        # 创建历史快照
        snapshot = user.copy()
        _id = user['owner'] + '|' + now.strftime('%Y%m%d')
        snapshot.update({
            '_id': _id,
            'created': datetime.now()
        })
        db.witness_history.update({'_id': _id}, snapshot, upsert=True)
```

**数据模型**：

**见证人当前状态** (`witness` 集合):
```python
{
    '_id': 'witness_name',
    'owner': 'witness_name',
    'created': '2016-01-01T00:00:00',
    'url': 'https://witness.example.com',
    'votes': 1234567890.0,
    'virtual_last_update': 1234567890.0,
    'virtual_position': 1.0,
    'virtual_scheduled_time': 1234567890.0,
    'last_sbd_exchange_update': datetime,
    'total_missed': 0,
    ...
}
```

**见证人历史快照** (`witness_history` 集合):
```python
{
    '_id': 'witness_name|20240101',
    'owner': 'witness_name',
    'created': datetime,  # 快照创建时间
    'votes': 1234567890.0,
    'total_missed': 0,
    ...  # 其他见证人字段
}
```

### 4.3 check_misses() 函数

**功能**：检查见证人错失区块的情况

**处理流程**：
1. 获取前 100 名见证人列表
2. 遍历每个见证人：
   - 检查是否已在 `misses` 字典中
   - 如果已存在：
     - 比较当前 `total_missed` 与记录的值
     - 如果有增加，记录错失事件
     - 更新 `misses` 字典
   - 如果不存在：
     - 添加到 `misses` 字典

**错失事件记录**：
```python
record = {
    'date': datetime.now(),
    'witness': owner,
    'increase': witness['total_missed'] - misses[owner],  # 增加的错失数
    'total': witness['total_missed']  # 总错失数
}
db.witness_misses.insert(record)
```

**数据模型**：

**见证人错失记录** (`witness_misses` 集合):
```python
{
    '_id': ObjectId(...),  # MongoDB 自动生成
    'date': datetime,
    'witness': 'witness_name',
    'increase': 5,  # 本次增加的错失数
    'total': 100    # 总错失数
}
```

### 4.4 run() 函数

**功能**：执行所有更新任务

**实现**：
```python
def run():
    update_witnesses()
    check_misses()
```

**执行顺序**：
1. 先更新见证人信息（包括历史快照）
2. 再检查错失区块情况

## 5. 数据流分析

### 5.1 见证人更新流程

```
Steem 节点
    │
    ▼
get_witnesses_by_vote('', 100) → 获取前100名见证人
    │
    ▼
遍历每个见证人
    │
    ├─► 数据类型转换
    │
    ├─► 保存到 witness 集合（当前状态）
    │
    └─► 创建快照 → 保存到 witness_history 集合
```

### 5.2 错失区块监控流程

```
Steem 节点
    │
    ▼
get_witnesses_by_vote('', 100) → 获取见证人列表
    │
    ▼
遍历每个见证人
    │
    ▼
比较 total_missed 与内存中的值
    │
    ├─► 有增加 → 记录错失事件 → witness_misses 集合
    │
    └─► 更新内存中的值
```

### 5.3 历史快照创建流程

```
当前日期
    │
    ▼
构建快照 ID: owner|YYYYMMDD
    │
    ▼
复制见证人数据
    │
    ▼
添加 created 时间戳
    │
    ▼
保存到 witness_history 集合（upsert）
```

## 6. 数据库操作

### 6.1 MongoDB 集合使用

| 集合名称 | 操作类型 | 说明 |
|---------|---------|------|
| `witness` | remove, update | 见证人当前状态（每次清空重建） |
| `witness_history` | update | 见证人历史快照（每日） |
| `witness_misses` | insert | 见证人错失区块事件记录 |

### 6.2 数据操作模式

**witness 集合**：
- **清空重建**：每次更新前先 `remove({})`，然后插入新数据
- **原因**：确保数据与当前状态一致，移除已不在前100名的见证人

**witness_history 集合**：
- **每日快照**：使用 `owner|YYYYMMDD` 作为 ID
- **upsert 操作**：同一天多次运行不会重复创建

**witness_misses 集合**：
- **事件记录**：每次检测到错失增加时插入新记录
- **累积历史**：保留所有错失事件的历史记录

### 6.3 数据模型详细说明

**见证人字段说明**：

| 字段 | 类型 | 说明 |
|------|------|------|
| `owner` | string | 见证人账户名 |
| `created` | datetime | 见证人创建时间 |
| `url` | string | 见证人网站 URL |
| `votes` | float | 获得的投票数（转换为权益） |
| `virtual_last_update` | float | 虚拟调度最后更新时间 |
| `virtual_position` | float | 虚拟调度位置 |
| `virtual_scheduled_time` | float | 虚拟调度时间 |
| `last_sbd_exchange_update` | datetime | 最后 SBD 汇率更新时间 |
| `total_missed` | int | 总错失区块数 |
| `running_version` | string | 运行的节点版本 |
| `hardfork_version_vote` | string | 硬分叉版本投票 |

## 7. 调度机制

### 7.1 任务调度配置

使用 `BackgroundScheduler` 实现后台任务调度：

```python
scheduler = BackgroundScheduler()
scheduler.add_job(run, 'interval', seconds=30, id='run')
scheduler.start()
```

### 7.2 任务执行频率

- **执行频率**：每 30 秒执行一次
- **执行内容**：
  1. `update_witnesses()` - 更新见证人信息
  2. `check_misses()` - 检查错失区块

### 7.3 启动流程

1. 脚本启动时立即执行一次 `run()` 函数
2. 启动调度器，每 30 秒执行一次
3. 主循环保持脚本运行（每 2 秒检查一次）

**代码**：
```python
if __name__ == '__main__':
    run()  # 立即执行一次
    scheduler = BackgroundScheduler()
    scheduler.add_job(run, 'interval', seconds=30, id='run')
    scheduler.start()
    try:
        while True:
            time.sleep(2)
    except (KeyboardInterrupt, SystemExit):
        scheduler.shutdown()
```

## 8. 错误处理

### 8.1 异常处理策略

**配置验证**：
- MongoDB URL 缺失：打印错误并退出
- Steem 节点 URL 缺失：使用默认值

**数据获取**：
- 代码中没有显式的异常处理
- 如果 Steem 节点不可用，会抛出异常导致脚本停止

### 8.2 日志记录

- 使用 `print()` 和 `pprint()` 进行输出
- 日志标签：`[Witness]`
- 没有使用标准的日志模块

### 8.3 错误处理特点

**问题**：
- 缺少异常处理机制
- 单个见证人处理失败可能导致整个任务失败
- 没有错误重试机制

**建议**：
- 添加 try-except 块处理异常
- 单个见证人处理失败不影响其他见证人
- 添加重试机制

## 9. 性能优化

### 9.1 批量操作

**见证人获取**：
- 一次获取前 100 名见证人，减少网络请求

**数据库操作**：
- 使用 `update()` 的 upsert 模式，自动创建或更新

### 9.2 内存使用

**misses 字典**：
- 只存储前 100 名见证人的错失数
- 内存占用很小

### 9.3 未使用的导入

- `collections`: 已导入但未使用
- **建议**：移除未使用的导入

## 10. 潜在问题与改进建议

### 10.1 代码质量问题

**问题 1：未使用的导入**
- `collections` 已导入但未使用
- **建议**：移除未使用的导入

**问题 2：硬编码的数值**
- 见证人数量：100
- 执行间隔：30 秒
- **建议**：移到配置文件中

**问题 3：使用已废弃的方法**
- `db.witness.remove({})` 已废弃
- **建议**：使用 `db.witness.delete_many({})`

**问题 4：使用已废弃的方法**
- `db.witness.update()` 和 `db.witness_history.update()` 已废弃
- **建议**：使用 `update_one()` 或 `replace_one()`

### 10.2 性能问题

**问题 1：清空重建集合**
- 每次更新都清空 `witness` 集合，然后重新插入
- **建议**：
  - 使用 `replace_one()` 逐个更新
  - 或者使用批量操作提高效率

**问题 2：同步阻塞**
- `rpc.get_witnesses_by_vote()` 是同步调用
- **建议**：使用异步 Steem 客户端

**问题 3：频繁执行**
- 每 30 秒执行一次可能过于频繁
- **建议**：
  - 见证人信息更新可以降低频率（如每分钟）
  - 错失区块检查可以保持较高频率

### 10.3 可靠性问题

**问题 1：Steem 节点故障**
- 如果 Steem 节点不可用，脚本会失败
- **建议**：
  - 支持多节点故障转移
  - 添加重试机制
  - 缓存最后状态

**问题 2：数据一致性**
- 清空重建可能导致短暂的数据不一致
- **建议**：使用事务或原子操作

**问题 3：错失检测不准确**
- 如果脚本停止运行一段时间，可能漏掉错失事件
- **建议**：
  - 记录上次检查时间
  - 启动时检查是否有遗漏

### 10.4 功能缺失

**问题 1：缺少监控**
- 没有性能指标和统计
- **建议**：
  - 添加处理时间统计
  - 添加错失事件统计
  - 添加见证人数量变化监控

**问题 2：缺少数据验证**
- 没有验证从 Steem 节点获取的数据
- **建议**：添加数据格式验证

**问题 3：缺少历史数据清理**
- `witness_misses` 集合会无限增长
- **建议**：添加数据归档或清理机制

### 10.5 代码结构问题

**问题 1：函数职责不清**
- `update_witnesses()` 函数同时处理当前状态和历史快照
- **建议**：拆分为独立的函数

**问题 2：配置管理分散**
- 配置读取逻辑简单
- **建议**：创建配置管理类

**问题 3：错误处理不统一**
- 使用 print 而不是日志系统
- **建议**：使用标准的日志模块

### 10.6 功能增强建议

1. **见证人排名变化追踪**：
   - 记录见证人排名的历史变化
   - 分析排名上升/下降趋势

2. **见证人性能分析**：
   - 计算见证人的出块率
   - 分析错失区块的模式

3. **告警机制**：
   - 当见证人错失区块时发送告警
   - 当见证人排名大幅下降时告警

4. **数据导出**：
   - 支持导出见证人数据为 CSV/JSON
   - 支持生成见证人报告

5. **见证人详情页**：
   - 提供见证人详细信息查询接口
   - 显示历史趋势图表

### 10.7 安全性建议

1. **输入验证**：验证从 Steem 节点获取的数据格式
2. **资源限制**：限制处理时间和内存使用
3. **错误信息**：避免在错误信息中泄露敏感信息

## 11. 架构图

### 11.1 系统架构

```
┌──────────────┐
│ Steem 节点    │
└──────┬───────┘
       │
       ▼
┌──────────────┐      ┌──────────────┐
│ witnesses.py │─────►│  MongoDB     │
│              │      │              │
│ - 更新见证人  │      │ - witness    │
│ - 检查错失    │      │ - witness_   │
│ - 创建快照    │      │   history    │
│              │      │ - witness_   │
│              │      │   misses     │
└──────────────┘      └──────────────┘
```

### 11.2 数据流

```
Steem 节点
    │
    ├─► get_witnesses_by_vote() → 见证人列表
    │
    ├─► update_witnesses() → witness 集合（当前状态）
    │                        witness_history 集合（历史快照）
    │
    └─► check_misses() → witness_misses 集合（错失事件）
```

## 12. 代码示例分析

### 12.1 见证人数据转换

```python
# 数值字段转换
for key in ['virtual_last_update', 'virtual_position', 'virtual_scheduled_time', 'votes']:
    user[key] = float(user[key])

# 日期字段转换
for key in ['last_sbd_exchange_update']:
    user[key] = datetime.strptime(user[key], "%Y-%m-%dT%H:%M:%S")
```

**说明**：
- Steem API 返回的数值可能是字符串，需要转换为 float
- 日期字段是 ISO 格式字符串，需要解析为 datetime 对象

### 12.2 错失检测逻辑

```python
if owner in misses.keys():
    if witness['total_missed'] > misses[owner]:
        # 记录错失事件
        record = {
            'date': datetime.now(),
            'witness': owner,
            'increase': witness['total_missed'] - misses[owner],
            'total': witness['total_missed']
        }
        db.witness_misses.insert(record)
        misses[owner] = witness['total_missed']
else:
    misses.update({owner: witness['total_missed']})
```

**说明**：
- 使用内存字典跟踪上次的错失数
- 只有当错失数增加时才记录事件
- 记录增加的数量和总数量

## 13. 总结

`witnesses.py` 是一个功能相对简单的见证人监控服务，主要特点：

**优点**：
- 功能明确，职责单一
- 使用定时任务自动更新
- 记录历史快照和错失事件
- 代码结构简单易懂

**需要改进**：
- 添加异常处理机制
- 使用标准的日志模块
- 更新已废弃的 MongoDB 方法
- 添加配置管理
- 改进错误处理和重试机制
- 添加监控和告警功能
- 优化数据库操作性能

该脚本适合作为 Steem 见证人监控的基础组件，但建议进行重构以提高可靠性、可维护性和功能完整性。


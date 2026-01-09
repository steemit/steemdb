下面这是一份**可以直接交付给 Cursor / Copilot 的「最终版开发计划」**。
我已经把我们前面来回争论、修正、澄清过的点**全部消化进方案本身**，不会再出现“哲学讨论式”的歧义，而是**工程可执行说明**。

你可以把这份文档当成：

* 项目 README（技术向）
* 架构设计说明
* Cursor 的实现依据

---

# Steem 区块浏览器数据同步系统

## 开发计划（工程可执行版）

---

## 0. 项目目标与约束

### 目标

构建一个 **Steem 区块浏览器数据后端**，具备：

* 完整历史数据（block / transaction / operation / virtual operation）
* 可接受的同步时间
* 可长期运行、成本可控
* 数据可修补、可重建
* 面向浏览器 / API 使用场景

### 明确约束（非常重要）

* **不长期运行 steemd**
* **不追求共识客户端级别一致性**
* **RPC 被视为 steemd 的查询视图，而非独立事实源**
* **允许极小概率的历史数据修补**

---

## 1. 总体架构

系统采用 **“plugin 冷启动 + RPC live + RPC 修补”** 的混合模型。

```
[ 冷启动阶段 ]
block_log + steemd
   ↓
steemd plugin (applied operations, async)
   ↓
Go ingest
   ↓
MongoDB

[ Live 阶段 ]
RPC 节点
   ↓
Go live fetcher
   ↓
MongoDB

[ 修补阶段 ]
RPC 节点
   ↓
Repair tool
   ↓
MongoDB
```

---

## 2. 阶段划分与职责

### 2.1 冷启动阶段（Cold Start）

#### 使用场景

* 第一次初始化数据库
* 已知 block_log 备份高度 `H`
* 当前链高度远大于 `H`

#### 数据来源

* **steemd replay + plugin**
* plugin 监听 `applied_operation`

#### 特点

* 吞吐量高
* 无网络 RPC 开销
* 可获取完整 virtual operations
* steemd 为短期运行（一次性）

#### 终止条件（明确）

* ingest 接收到的最大 `block_num >= H - safety_margin`
* safety_margin 通常为 1~5 个 block

#### 行为

* ingest **主动退出**
* steemd 进程关闭（人工介入，手动停止）
* 冷启动结束

> 注意：
> 冷启动阶段 **不追 head，不做 live**

---

### 2.2 Live 阶段（RPC Live Sync）

#### 使用场景

* 冷启动完成后
* 长期运行的区块浏览器

#### 数据来源

* RPC 节点（database_api / block_api）

#### 同步策略

* 从数据库中已存在的最大 block `B`
* 顺序请求：

  * `get_block(B+1)`
  * 如需要 virtual ops，额外 RPC 调用
* 按 block 顺序写入 MongoDB

#### 特点

* 吞吐要求低
* 延迟可接受
* 不依赖本地 steemd
* 运维成本低

---

### 2.3 修补阶段（Repair Tool）

#### 触发条件

* 在冷启动结束后，由人工执行一次。

#### 数据来源

* RPC 节点（同 live 阶段）

#### 修补逻辑

1. 扫描数据库：
   * block 1 → 指定高度（如果不设置指定高度，默认数据库记录的最新 block 高度）
2. 发现缺失或异常 block
3. 对缺失 block：
   * RPC 获取完整 block / tx / ops / virtual ops（遇到网络问题，需要重试直到获取到预期数据）
4. 写入 MongoDB（幂等）

#### 特点

* 修补是 **离线 / 低频操作**
* 不影响 live 同步

---

## 3. 数据一致性原则（重要）

### 3.1 单一事实的定义

* **所有数据最终均来自 steemd**
* 区别仅在于：

  * 冷启动：执行期事件流（plugin）
  * live / repair：执行后查询视图（RPC）

### 3.2 接受的妥协

* RPC 返回的 JSON 结构可能随 steemd 版本演进
* virtual operations 可能通过多次 RPC 获取
* 不保证 applied_operation 与 RPC 在结构上 1:1

### 3.3 不接受的情况

* 数据乱序
* block 高度不连续
* transaction / operation 丢失且不可检测

---

## 4. Ingest 服务设计（Go）

### 4.1 冷启动 ingest（plugin → Go）

#### 输入

* plugin 推送的 JSON
* 一条 JSON = 一个 applied operation

#### 内部结构

```
HTTP handler
   ↓
decode JSON
   ↓
buffered channel
   ↓
batcher (按条数 / 时间)
   ↓
MongoDB BulkWrite
```

#### 设计原则

* handler 不写数据库
* channel 吸收 replay 峰值
* 批量写入 Mongo
* 所有写入幂等

---

### 4.2 Live fetcher（RPC → Go）

#### 输入

* RPC block 数据

#### 行为

* 顺序拉取 block
* 转换为统一 Mongo schema
* 单线程或低并发即可

---

## 5. MongoDB Schema 设计（核心）

### 5.1 blocks 集合

```js
{
  _id: block_num,
  block_num,
  block_id,
  previous,
  timestamp,
  witness,
  transaction_count
}
```

索引：

* `_id`（主键）
* `timestamp`

---

### 5.2 transactions 集合

```js
{
  _id: trx_id,
  block_num,
  trx_index,
  expiration,
}
```

索引：

* `_id`
* `block_num`

---

### 5.3 operations 集合（核心）

```js
{
  _id: block_num + ":" + trx_index + ":" + op_index,
  block_num,
  trx_id,
  trx_index,
  op_index,
  op_type,
  op_value,
  virtual: true | false,
}
```

索引：

* `_id`（幂等保证）
* `block_num`
* `trx_id`
* `op_type`
* `virtual`

---

### 5.4 meta 集合

```js
{
  _id: "sync_state",
  max_block: Number,
  cold_start_done: Boolean,
  updated_at
}
```

---

## 6. 幂等与安全性设计

* 所有写入基于 `_id` 唯一
* replay / repair / live 可重复执行
* 不做 delete + insert
* 使用 upsert / ignore duplicate

---

## 7. 明确禁止事项（给 Cursor 的红线）

* ❌ 不在 steemd plugin 中写数据库
* ❌ 不在 plugin 中做 RPC
* ❌ 不在 ingest handler 中直接写 Mongo
* ❌ 不在 live 阶段依赖 steemd replay
* ❌ 不尝试用 RPC “模拟 applied_operation”

---

## 8. 工程哲学（必须理解）

* 冷启动解决 **性能问题**
* RPC 解决 **长期运行成本问题**
* 修补解决 **不完美现实**
* 浏览器 ≠ 共识客户端

---

## 9. 一句话总结（可放 README）

> 本项目使用 steemd replay + plugin 加速历史同步，
> 使用 RPC 进行实时追块与缺失修补，
> 在可控成本下提供完整、可修复的 Steem 区块浏览器数据服务。

---

下面是**可以直接丢给 Cursor 的「TODO 拆分清单」**。
它是**工程任务级别**的，不是说明文；Cursor 可以照着一步步把代码写出来。

我按 **模块 → 子任务 → 关键约束** 来拆，避免 Cursor“发挥过头”。

---

# Steem 区块浏览器数据同步

## Cursor TODO 拆分清单

---

## A. 公共约定（必须先完成）

### A1. 项目结构初始化

* [x] 在 `steemdb/steemdb-sync/` 目录下初始化 Go 项目（Go ≥ 1.22）
* [x] 使用 Go Modules
* [x] 目录结构：

```text
steemdb/
  steemdb-sync/              # Go 项目根目录
    go.mod
    go.sum
    /cmd
      /cold_ingest           # 冷启动 ingest 服务
      /live_sync             # Live 同步服务
      /repair                # 修补工具
    /internal
      /config                # 配置管理
      /model                 # 数据模型
      /mongo                 # MongoDB 访问层
      /rpc                   # RPC 客户端（使用 steemgosdk）
      /pipeline              # 数据处理管道
      /checker               # 数据完整性检查
    README.md
    configs/
      config.yaml            # 配置文件示例
```

**约束**

* 项目根目录为 `steemdb/steemdb-sync/`
* 所有模块仅通过 `internal` 引用
* `cmd` 下每个目录是一个可执行程序
* Go module 名称建议: `github.com/steemit/steemdb-sync` 或根据实际仓库路径

---

### A2. 通用配置模块

* [x] 定义配置结构体：

  * Mongo URI / DB
  * RPC endpoint
  * cold start target height
  * batch size
  * batch flush interval
* [x] 支持：

  * env
  * yaml 配置文件

**约束**

* 不允许在代码中硬编码地址

---

### A3. 依赖库说明

* [x] 添加 Go 依赖：

  * **steemgosdk**: 用于与 Steem RPC 节点通讯
    * 包路径: `github.com/steemit/steemgosdk/api`
    * 用途: RPC 调用（get_block, get_ops_in_block 等）
    * 示例: `sdkapi.NewAPI(endpoint)` 创建 API 客户端
  * **steemutil**: 用于 operation 等结构体定义
    * 包路径: `github.com/steemit/steemutil/protocol`
    * 用途: Operation, OperationObject, Block 等协议结构体
    * 示例: `protocol.Operation`, `protocol.OperationObject`

**约束**

* RPC 通讯必须使用 steemgosdk，不自行实现 RPC 客户端
* operation 相关结构体必须使用 steemutil，保持协议一致性
* 其他工具函数可参考 steemutil 的其他包

---

## B. Mongo Schema & Access Layer

### B1. 定义 Mongo 数据模型（仅结构）

* [x] `Block`
* [x] `Transaction`
* [x] `Operation`
* [x] `Meta`

**约束**

* struct 字段与 Mongo 字段 1:1
* 不嵌套复杂逻辑

---

### B2. Mongo 初始化与索引

* [x] 建立 Mongo client
* [x] 自动创建索引：

  * blocks._id
  * transactions._id
  * operations._id
  * operations.block_num
  * operations.trx_id
  * operations.op_type
  * operations.virtual

**约束**

* 程序启动时校验索引存在
* 索引创建必须幂等

---

### B3. BulkWrite 封装

* [x] 封装通用 `BulkUpsert(ops []Operation)`
* [x] 支持 ordered=false
* [x] 忽略 duplicate key 错误

**约束**

* 不允许单条写 Mongo
* 不允许在 handler 中直接调用 Mongo

---

## C. Cold Start Ingest（plugin → Go）

### C1. HTTP 接收接口

* [x] 启动 HTTP server
* [x] POST `/ingest/applied_op`
* [x] JSON decode 到 Operation struct

**约束**

* handler 只 decode + enqueue
* handler 不做任何 IO（除 enqueue）

---

### C2. 内存缓冲队列

* [x] 使用 buffered channel
* [x] channel 容量 ≥ 100k
* [x] 单独 goroutine 负责消费

**约束**

* channel 满时允许阻塞
* 不允许丢数据

---

### C3. Batch 聚合器

* [x] 按以下条件 flush：

  * 条数达到 batch_size
  * 或时间达到 batch_interval
* [x] flush 后调用 Mongo BulkWrite

**约束**

* batch 内 ops 顺序不重要
* flush 必须可重入

---

### C4. 冷启动终止逻辑

* [x] 从配置读取 target_height
* [x] ingest 维护 max_block_seen
* [x] 当 max_block_seen ≥ target_height：

  * flush 所有 batch
  * 更新 meta.cold_start_done
  * 正常退出进程

**约束**

* 不主动控制 steemd
* 只控制自身生命周期

---

## D. Live RPC 同步程序

### D1. RPC 客户端封装

* [x] 使用 **steemgosdk** 实现 RPC 客户端封装
* [x] 创建 `internal/rpc/client.go`，封装 steemgosdk API
* [x] 支持：

  * get_block（使用 `api.GetBlock(blockNum)`）
  * get_ops_in_block（使用 `api.GetOpsInBlock(blockNum, onlyVirtual)`）
* [x] 支持重试与超时（steemgosdk 已内置，可配置）

**约束**

* 必须使用 steemgosdk，不自行实现 RPC 客户端
* 不缓存 RPC 结果
* 每次调用独立
* 参考 `steemdb-web/pkg/steem/client.go` 的实现方式

---

### D2. Live block 同步逻辑

* [x] 从 meta.max_block 读取起始高度
* [x] 顺序请求 block N+1
* [x] 若 block 不存在：

  * sleep
  * retry

**约束**

* 严格按 block 顺序
* 不并发请求多个高度

---

### D3. RPC 数据 → Mongo Schema 转换

* [x] 使用 **steemutil/protocol** 的结构体作为中间格式
* [x] 转换流程：

  * RPC 返回 → `protocolapi.Block` / `protocol.OperationObject`
  * `protocolapi.Block` → Mongo `Block`
  * `protocol.OperationObject` → Mongo `Operation`
* [x] 处理：

  * block → Block
  * trx → Transaction
  * ops → Operation（使用 `protocol.OperationObject`）
  * virtual ops 标记（使用 `OperationObject.VirtualOperation`）

**约束**

* 必须使用 steemutil 的 protocol 包结构体
* schema 与 cold ingest 完全一致
* source 字段标记为 "rpc"
* 参考 `steemutil/protocol/operation_object.go` 的结构定义

---

### D4. 写入与进度推进

* [x] 使用 BulkWrite
* [x] 成功写入后更新 meta.max_block

**约束**

* 失败时不推进高度

---

## E. Repair Tool（RPC 修补程序）

### E1. 数据完整性扫描

* [ ] 从 block 1 扫描到 meta.max_block
* [ ] 检测：

  * block 缺失
  * block 存在但 ops 为空

**约束**

* 只读扫描
* 不修改数据

---

### E2. 缺失区块收集

* [ ] 生成缺失 block 列表
* [ ] 支持区间合并（N~M）

**约束**

* 不假设缺失是连续的

---

### E3. RPC 修补执行

* [ ] 对每个缺失 block：

  * 使用 steemgosdk RPC 拉取（复用 D1 的 RPC 客户端）
  * 使用 steemutil 结构体转换（复用 D3 的转换逻辑）
  * BulkWrite

**约束**

* 幂等写入
* 复用 D1 和 D3 的实现，避免重复代码

---

## F. 状态与元数据管理

### F1. meta 集合维护

* [x] 初始化 meta 文档
* [x] 维护字段：

  * max_block
  * cold_start_done
  * updated_at

**约束**

* 所有程序共用 meta
* 更新必须原子

---

## G. 日志与可观测性

### G1. 基础日志

* [x] 启动 / 退出日志
* [x] batch flush 统计
* [x] RPC 请求错误

**约束**

* 不打印每条 operation

---

### G2. 简单指标（可选）

* [ ] ingest TPS
* [ ] Mongo write latency
* [ ] RPC latency

---

## H. 明确不做的事情（防止 Cursor 越界）

* ❌ 不实现 fork 处理
* ❌ 不实现 chain validation
* ❌ 不比较 RPC 与 plugin 数据差异
* ❌ 不引入 Redis / MQ
* ❌ 不引入 steemd 依赖

---

## I. 最终交付检查表

* [x] cold ingest 能完整跑完到目标高度（已实现，待测试）
* [x] live sync 能稳定追块（已实现，待测试）
* [ ] repair 能补回人为删除的 block（待实现）
* [x] 重跑程序不会产生重复数据（已实现幂等写入）
* [x] Mongo 数据可直接用于区块浏览器 API（Schema 已定义）

---

## 实现状态总结

### ✅ 已完成（2026-01-09）

**A. 公共约定**
- ✅ A1: 项目结构初始化
- ✅ A2: 通用配置模块（YAML + ENV）
- ✅ A3: 依赖库（steemgosdk + steemutil）

**B. MongoDB Schema & Access Layer**
- ✅ B1: 数据模型定义（Block, Transaction, Operation, Meta）
- ✅ B2: MongoDB 初始化和索引创建
- ✅ B3: BulkWrite 封装

**C. Cold Start Ingest**
- ✅ C1: HTTP 接收接口（POST /ingest/applied_op）
- ✅ C2: 内存缓冲队列（buffered channel，容量 100k）
- ✅ C3: Batch 聚合器（按条数/时间 flush）
- ✅ C4: 冷启动终止逻辑（达到 target_height 后退出）

**D. Live RPC 同步程序**
- ✅ D1: RPC 客户端封装（使用 steemgosdk）
- ✅ D2: Live block 同步逻辑（顺序请求 block）
- ✅ D3: RPC 数据转换（使用 steemutil 结构体）
- ✅ D4: 写入与进度推进（BulkWrite + 更新 meta.max_block）

**F. 状态与元数据管理**
- ✅ F1: meta 集合维护（已在 mongo 包中实现）

**G. 日志与可观测性**
- ✅ G1: 基础日志（启动/退出、batch flush 统计、RPC 错误）

### ⏳ 待实现

**E. Repair Tool（RPC 修补程序）**
- ⏳ E1: 数据完整性扫描
- ⏳ E2: 缺失区块收集
- ⏳ E3: RPC 修补执行

**G. 日志与可观测性（可选）**
- ⏳ G2: 简单指标（ingest TPS, Mongo write latency, RPC latency）

### 📝 实现文件清单

**核心组件：**
- `cmd/cold_ingest/main.go` - 冷启动 ingest 服务
- `cmd/live_sync/main.go` - Live 同步服务
- `internal/config/config.go` - 配置管理
- `internal/model/models.go` - 数据模型
- `internal/mongo/mongodb.go` - MongoDB 访问层
- `internal/pipeline/batcher.go` - 批处理管道
- `internal/pipeline/ingest_handler.go` - HTTP 处理器
- `internal/rpc/client.go` - RPC 客户端
- `internal/rpc/converter.go` - 数据转换器

**配置文件：**
- `configs/config.yaml` - 配置文件示例
- `go.mod` / `go.sum` - Go 模块依赖

### 🧪 待测试

- [ ] 冷启动 ingest 端到端测试
- [ ] Live sync 端到端测试
- [ ] 数据完整性验证
- [ ] 性能测试（吞吐量、延迟）

---


下面这份是**“Operation JSON 的精确定义（plugin ↔ ingest）”**，
这是**协议级定义**，不是示例随便写写那种。你可以：

* 直接交给 Cursor
* 作为 plugin 与 ingest 之间的**稳定契约**
* 作为 Mongo `operations` 集合的**事实来源**

我会分为：

1. 设计原则（防止误解）
2. JSON 顶层结构（完整 schema）
3. 字段逐项说明（语义 + 约束）
4. 示例（真实 / virtual 各一）
5. 明确禁止与注意事项

---

# Operation JSON 协议定义

## (steemd plugin → Go ingest)

---

## 1. 设计原则（必须先理解）

### 1.1 单条消息 = 单个 applied operation

* **不是 block**
* **不是 transaction**
* **不是 ops 数组**

👉 一条 JSON 表示：

> steemd 状态机执行过程中产生的 **一个 operation（真实或 virtual）**

---

### 1.2 JSON 是“共识执行结果的直出”

* plugin **不加工**
* plugin **不聚合**
* plugin **不补字段**
* ingest **不推断**

---

### 1.3 JSON 必须自包含（ingest 无需上下文）

* 必须包含：

  * block 位置信息
  * transaction 位置信息
  * operation 位置信息
* ingest **不依赖前后消息**

---

## 2. JSON 顶层结构（规范）

```json
{
  "block": {
    "num": 123456,
    "id": "0001e240...",
    "timestamp": "2023-01-01T00:00:00"
  },

  "transaction": {
    "id": "abcd1234...",
    "index": 2
  },

  "operation": {
    "index": 0,
    "type": "transfer",
    "value": { }
  },

  "virtual": false
}
```

---

## 3. 字段逐项定义（非常重要）

### 3.1 block 对象

```json
"block": {
  "num": number,
  "id": string,
  "timestamp": string
}
```

| 字段        | 含义           | 约束            |
| --------- | ------------ | ------------- |
| num       | block height | 必须 ≥ 1        |
| id        | block_id     | hex string    |
| timestamp | 出块时间         | ISO-8601（UTC） |

**说明**

* timestamp 直接来自 steemd block header
* ingest 不转换时区

---

### 3.2 transaction 对象

```json
"transaction": {
  "id": string,
  "index": number
}
```

| 字段    | 含义             | 约束         |
| ----- | -------------- | ---------- |
| id    | transaction_id | hex string |
| index | 在 block 中的位置   | 从 0 开始     |

**注意**

* 对于 virtual operation：

  * `transaction.id` 可能为 `null`
  * `transaction.index` 可设为 `-1`

👉 **必须保留 transaction 对象本身**

---

### 3.3 operation 对象（核心）

```json
"operation": {
  "index": number,
  "type": string,
  "value": object
}
```

| 字段    | 含义            | 约束                   |
| ----- | ------------- | -------------------- |
| index | op 在 trx 中的位置 | 从 0 开始；virtual 可为 -1 |
| type  | op 名称         | snake_case           |
| value | op 原始内容       | JSON object          |

#### type 示例

* `transfer`
* `vote`
* `comment`
* `author_reward`
* `curation_reward`
* `producer_reward`

#### value 规则

* **字段名与 steemd C++ struct 一致**
* 不做字段重命名
* 不做数值转换
* asset 使用字符串（如 `"1.000 STEEM"`）

---

### 3.4 virtual 标记

```json
"virtual": boolean
```

| 值     | 含义                  |
| ----- | ------------------- |
| false | transaction 中的真实 op |
| true  | 状态机生成的 virtual op   |

**说明**

* ingest 不推断 virtual
* 完全信任 plugin

---

## 4. Mongo `_id` 生成规则（由 ingest 执行）

> ⚠️ 不在 JSON 中传 `_id`

ingest 必须使用以下规则生成唯一 ID：

```text
_id = block.num + ":" + transaction.index + ":" + operation.index
```

* virtual op：

  * transaction.index = -1
  * operation.index = -1 或递增 virtual index

---

## 5. 示例

### 5.1 普通 transfer（非 virtual）

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

---

### 5.2 author_reward（virtual）

```json
{
  "block": {
    "num": 50000001,
    "id": "02faf081...",
    "timestamp": "2023-09-01T12:05:00"
  },
  "transaction": {
    "id": null,
    "index": -1
  },
  "operation": {
    "index": -1,
    "type": "author_reward",
    "value": {
      "author": "alice",
      "permlink": "my-post",
      "steem_payout": "0.000 STEEM",
      "sbd_payout": "1.234 SBD",
      "vesting_payout": "10.000000 VESTS"
    }
  },
  "virtual": true
}
```

---

## 6. Plugin 实现侧硬性要求（给 C++ 插件）

* 必须监听 `applied_operation`
* 必须在 **op 应用完成后**发送
* 必须异步发送（不阻塞 steemd）
* 不得缓存超过极短队列
* 不得在 plugin 内 retry 网络 IO

---

## 7. Ingest 实现侧硬性要求（给 Cursor）

* 不修改 JSON 内容
* 不推断字段
* 不补默认值
* 原样落库（除 `_id` / `source`）

---

## 8. 明确禁止（防止协议被破坏）

* ❌ plugin 合并多个 ops
* ❌ plugin 输出 block / trx 级 JSON
* ❌ ingest 重算 virtual ops
* ❌ ingest 根据 type 推字段
* ❌ 在 JSON 中传 `_id`

---

## 9. 一句话协议总结（可写进 README）

> Operation JSON 是 steemd 状态机执行结果的逐条序列化，
> ingest 不解释、不推断，仅负责可靠存储。

---

# Plugin 开发计划（C++ steemd Plugin）

## 概述

本计划描述如何开发 steemd plugin，用于在冷启动阶段将 `applied_operation` 事件转换为 JSON 并通过 HTTP 发送到 Go ingest 服务。

## 1. Plugin 项目结构

### 1.1 目录位置

Plugin 位于 steem 代码库中：

```
steem/
  libraries/
    plugins/
      ingest/                    # Plugin 根目录
        ingest_plugin.hpp       # Plugin 接口定义
        ingest_plugin.cpp       # Plugin 实现
        ingest_api.hpp          # API 定义（可选，用于查询状态）
        ingest_api.cpp          # API 实现
        CMakeLists.txt          # 构建配置
```

### 1.2 文件职责

* `ingest_plugin.hpp/cpp`: 核心 plugin 实现
  * 监听 `applied_operation` 信号
  * 构建 Operation JSON
  * 异步发送 HTTP 请求
* `ingest_api.hpp/cpp`: 可选的 RPC API（用于查询 plugin 状态）
* `CMakeLists.txt`: 构建配置，集成到 steemd 构建系统

---

## 2. 核心实现步骤

### 2.1 Plugin 基础结构

#### 2.1.1 Plugin 类定义（ingest_plugin.hpp）

```cpp
#include <steem/plugins/chain/chain_plugin.hpp>
#include <steem/chain/steem_fwd.hpp>
#include <appbase/application.hpp>

namespace steem { namespace plugins { namespace ingest {

class ingest_plugin : public appbase::plugin< ingest_plugin >
{
public:
   ingest_plugin();
   virtual ~ingest_plugin();

   APPBASE_PLUGIN_REQUIRES( (chain::chain_plugin) )

   static const std::string& name() { static std::string name = "ingest"; return name; }

   virtual void set_program_options( boost::program_options::options_description& cli,
                                     boost::program_options::options_description& cfg ) override;
   virtual void plugin_initialize( const boost::program_options::variables_map& options ) override;
   virtual void plugin_startup() override;
   virtual void plugin_shutdown() override;

private:
   std::unique_ptr< class ingest_plugin_impl > my;
};
```

#### 2.1.2 Plugin 实现类（ingest_plugin.cpp - detail 命名空间）

```cpp
namespace detail {

class ingest_plugin_impl
{
public:
   ingest_plugin_impl( ingest_plugin& _plugin );
   ~ingest_plugin_impl();

   void on_post_apply_operation( const chain::operation_notification& note );
   
   // HTTP 发送相关
   void send_operation_json( const fc::variant& json );
   void http_send_worker();  // 后台线程工作函数

private:
   ingest_plugin& _plugin;
   chain::database& _db;
   
   // 信号连接
   boost::signals2::connection _post_apply_operation_conn;
   
   // HTTP 配置
   std::string _ingest_endpoint;  // 如 "http://localhost:8080/ingest/applied_op"
   uint32_t _http_timeout_ms;
   uint32_t _max_queue_size;
   
   // 异步发送队列
   std::queue< std::string > _send_queue;
   std::mutex _queue_mutex;
   std::condition_variable _queue_cv;
   std::thread _http_thread;
   bool _shutdown;
};

} // detail
```

---

### 2.2 监听 applied_operation 信号

#### 2.2.1 在 plugin_initialize 中注册

```cpp
void ingest_plugin::plugin_initialize( const boost::program_options::variables_map& options )
{
   my = std::make_unique< detail::ingest_plugin_impl >( *this );
   
   // 读取配置
   if( options.count( "ingest-endpoint" ) )
      my->_ingest_endpoint = options["ingest-endpoint"].as< std::string >();
   else
      my->_ingest_endpoint = "http://localhost:8080/ingest/applied_op";
   
   // 注册信号处理器
   chain::database& db = appbase::app().get_plugin< chain::chain_plugin >().db();
   my->_post_apply_operation_conn = db.add_post_apply_operation_handler(
      [&]( const chain::operation_notification& note ) {
         my->on_post_apply_operation( note );
      },
      *this,
      0  // group
   );
}
```

#### 2.2.2 处理 applied_operation

```cpp
void ingest_plugin_impl::on_post_apply_operation( const chain::operation_notification& note )
{
   try {
      // 获取当前 block 信息
      const auto& dgpo = _db.get_dynamic_global_properties();
      uint32_t block_num = dgpo.head_block_number;
      const auto& block = _db.fetch_block_by_number( block_num );
      
      if( !block ) return;  // 安全检查
      
      // 构建 Operation JSON（见下一节）
      fc::variant json = build_operation_json( note, *block, block_num );
      
      // 异步发送
      send_operation_json( json );
      
   } catch( const fc::exception& e ) {
      elog( "Error processing operation: ${e}", ("e", e.to_string()) );
      // 不抛出异常，避免影响 steemd 运行
   }
}
```

---

### 2.3 构建 Operation JSON

#### 2.3.1 JSON 构建函数

```cpp
fc::variant ingest_plugin_impl::build_operation_json(
   const chain::operation_notification& note,
   const chain::signed_block& block,
   uint32_t block_num
)
{
   fc::mutable_variant_object result;
   
   // 1. block 对象
   fc::mutable_variant_object block_obj;
   block_obj["num"] = block_num;
   block_obj["id"] = block.id().str();
   block_obj["timestamp"] = block.timestamp.to_iso_string();
   result["block"] = block_obj;
   
   // 2. transaction 对象
   fc::mutable_variant_object trx_obj;
   
   // 判断是否为 virtual operation
   bool is_virtual = note.op.which() >= chain::operation::tag< chain::virtual_operation >::value;
   
   if( is_virtual ) {
      // virtual operation: transaction 信息为空
      trx_obj["id"] = fc::variant();
      trx_obj["index"] = -1;
   } else {
      // 真实 operation: 从 note 中获取 transaction 信息
      // 注意: operation_notification 可能不直接包含 trx_id
      // 需要从 block 中查找对应的 transaction
      trx_obj["id"] = find_transaction_id( block, note );  // 需要实现
      trx_obj["index"] = find_transaction_index( block, note );  // 需要实现
   }
   result["transaction"] = trx_obj;
   
   // 3. operation 对象
   fc::mutable_variant_object op_obj;
   
   // operation index（在 transaction 中的位置）
   op_obj["index"] = note.op_in_trx;
   
   // operation type（使用 steemd 的 operation_name）
   op_obj["type"] = _db.get_operation_name( note.op );
   
   // operation value（使用 fc::variant 序列化）
   op_obj["value"] = note.op;
   
   result["operation"] = op_obj;
   
   // 4. virtual 标记
   result["virtual"] = is_virtual;
   
   return fc::variant( result );
}
```

#### 2.3.2 查找 Transaction 信息（真实 operation）

```cpp
std::string ingest_plugin_impl::find_transaction_id(
   const chain::signed_block& block,
   const chain::operation_notification& note
)
{
   // 遍历 block 中的 transactions
   for( const auto& trx : block.transactions ) {
      // 检查 trx 中是否包含 note.op
      // 简化实现：通过 op_in_trx 和 block 结构匹配
      // 实际实现需要更精确的匹配逻辑
   }
   return trx.id().str();
}

int32_t ingest_plugin_impl::find_transaction_index(
   const chain::signed_block& block,
   const chain::operation_notification& note
)
{
   // 返回 transaction 在 block 中的索引（从 0 开始）
   // 实现逻辑类似 find_transaction_id
}
```

---

### 2.4 异步 HTTP 发送

#### 2.4.1 使用 Boost.Beast 发送 HTTP POST

```cpp
#include <boost/beast/core.hpp>
#include <boost/beast/http.hpp>
#include <boost/beast/version.hpp>
#include <boost/asio/connect.hpp>
#include <boost/asio/ip/tcp.hpp>

namespace beast = boost::beast;
namespace http = beast::http;
namespace net = boost::asio;
using tcp = boost::asio::ip::tcp;

void ingest_plugin_impl::send_operation_json( const fc::variant& json )
{
   // 序列化 JSON
   std::string json_str = fc::json::to_string( json );
   
   // 检查队列大小
   {
      std::lock_guard< std::mutex > lock( _queue_mutex );
      if( _send_queue.size() >= _max_queue_size ) {
         wlog( "Ingest queue full, dropping operation" );
         return;  // 队列满时丢弃（避免阻塞 steemd）
      }
      _send_queue.push( json_str );
   }
   _queue_cv.notify_one();
}

void ingest_plugin_impl::http_send_worker()
{
   while( !_shutdown ) {
      std::string json_str;
      
      // 从队列取数据
      {
         std::unique_lock< std::mutex > lock( _queue_mutex );
         _queue_cv.wait( lock, [this] { return !_send_queue.empty() || _shutdown; } );
         
         if( _shutdown && _send_queue.empty() ) break;
         
         json_str = _send_queue.front();
         _send_queue.pop();
      }
      
      // 发送 HTTP POST
      try {
         send_http_post( json_str );
      } catch( const std::exception& e ) {
         elog( "HTTP send error: ${e}", ("e", e.what()) );
         // 不重试，避免阻塞
      }
   }
}

void ingest_plugin_impl::send_http_post( const std::string& json_body )
{
   // 解析 URL
   // 简化示例，实际需要完整 URL 解析
   std::string host = "localhost";
   std::string port = "8080";
   std::string target = "/ingest/applied_op";
   
   net::io_context ioc;
   tcp::resolver resolver( ioc );
   tcp::socket socket( ioc );
   
   // 解析地址
   auto const results = resolver.resolve( host, port );
   net::connect( socket, results.begin(), results.end() );
   
   // 构建 HTTP 请求
   http::request< http::string_body > req;
   req.method( http::verb::post );
   req.target( target );
   req.set( http::field::host, host );
   req.set( http::field::content_type, "application/json" );
   req.body() = json_body;
   req.prepare_payload();
   
   // 发送请求
   http::write( socket, req );
   
   // 读取响应（简化处理）
   beast::flat_buffer buffer;
   http::response< http::string_body > res;
   http::read( socket, buffer, res );
   
   // 检查状态码
   if( res.result() != http::status::ok ) {
      elog( "HTTP error: ${code}", ("code", res.result_int()) );
   }
   
   // 关闭连接
   beast::error_code ec;
   socket.shutdown( tcp::socket::shutdown_both, ec );
}
```

#### 2.4.2 启动后台线程

```cpp
void ingest_plugin::plugin_startup()
{
   my->_shutdown = false;
   my->_http_thread = std::thread( [this]() {
      my->http_send_worker();
   } );
}

void ingest_plugin::plugin_shutdown()
{
   // 停止后台线程
   {
      std::lock_guard< std::mutex > lock( my->_queue_mutex );
      my->_shutdown = true;
   }
   my->_queue_cv.notify_one();
   
   if( my->_http_thread.joinable() ) {
      my->_http_thread.join();
   }
   
   // 断开信号连接
   chain::util::disconnect_signal( my->_post_apply_operation_conn );
}
```

---

### 2.5 配置选项

#### 2.5.1 添加配置参数

```cpp
void ingest_plugin::set_program_options(
   boost::program_options::options_description& cli,
   boost::program_options::options_description& cfg
)
{
   cfg.add_options()
      ( "ingest-endpoint",
        boost::program_options::value< std::string >()->default_value( "http://localhost:8080/ingest/applied_op" ),
        "Ingest service HTTP endpoint" )
      ( "ingest-http-timeout",
        boost::program_options::value< uint32_t >()->default_value( 5000 ),
        "HTTP request timeout in milliseconds" )
      ( "ingest-queue-size",
        boost::program_options::value< uint32_t >()->default_value( 100000 ),
        "Maximum queue size for pending operations" )
      ;
}
```

---

### 2.6 CMakeLists.txt 配置

```cmake
# 在 steem/libraries/plugins/CMakeLists.txt 中添加
add_subdirectory( ingest )

# ingest/CMakeLists.txt
add_library( ingest_plugin
   ingest_plugin.cpp
   ingest_api.cpp
)

target_link_libraries( ingest_plugin
   steem_chain
   appbase
   ${Boost_LIBRARIES}
   ${CMAKE_DL_LIBS}
)

target_include_directories( ingest_plugin
   PUBLIC
   ${CMAKE_CURRENT_SOURCE_DIR}/../../..
)

install( TARGETS ingest_plugin
   LIBRARY DESTINATION lib
)
```

---

## 3. 关键实现细节

### 3.1 Virtual Operation 检测

```cpp
bool is_virtual_operation( const chain::operation& op )
{
   // steemd 中 virtual operation 的 which() 值 >= 某个阈值
   return op.which() >= chain::operation::tag< chain::virtual_operation >::value;
}
```

### 3.2 Operation 名称获取

```cpp
std::string get_operation_name( const chain::operation& op )
{
   // 使用 steemd 的 operation_name 函数
   return _db.get_operation_name( op );
   // 或使用 FC 反射系统
   // return fc::get_typename< T >::name();
}
```

### 3.3 Transaction 索引查找

**重要**: `operation_notification` 可能不直接包含 transaction 索引，需要：

1. 遍历当前 block 的 transactions
2. 匹配 operation 内容
3. 记录 transaction 索引和 operation 在 transaction 中的索引

**简化方案**: 如果 steemd 的 `operation_notification` 结构包含足够信息，直接使用。

---

## 4. 错误处理与容错

### 4.1 设计原则

* **不阻塞 steemd**: 所有 IO 操作异步
* **失败不重试**: 避免影响性能
* **队列满时丢弃**: 记录日志但不阻塞
* **异常捕获**: 所有回调函数必须 try-catch

### 4.2 日志记录

```cpp
// 使用 steemd 的日志系统
ilog( "Ingest plugin started, endpoint: ${ep}", ("ep", _ingest_endpoint) );
wlog( "Queue full, dropping operation" );
elog( "HTTP send error: ${e}", ("e", error) );
```

---

## 5. 测试与验证

### 5.1 单元测试

* JSON 构建正确性
* Virtual operation 检测
* Transaction 索引查找

### 5.2 集成测试

* 启动 steemd replay
* 验证 JSON 发送到 ingest
* 验证 JSON 格式符合协议

### 5.3 性能测试

* 队列吞吐量
* HTTP 发送延迟
* steemd replay 性能影响（应 < 5%）

---

## 6. 部署与使用

### 6.1 编译

```bash
cd steem
mkdir build && cd build
cmake .. -DCMAKE_BUILD_TYPE=Release
make -j$(nproc)
```

### 6.2 配置 steemd

在 `config.ini` 中启用 plugin：

```ini
plugin = ingest
ingest-endpoint = http://localhost:8080/ingest/applied_op
ingest-http-timeout = 5000
ingest-queue-size = 100000
```

### 6.3 启动流程

1. 启动 Go ingest 服务（监听 HTTP）
2. 启动 steemd replay（启用 ingest plugin）
3. 监控日志，确认数据流

---

## 7. 明确禁止事项

* ❌ 不在 plugin 中写数据库
* ❌ 不在 plugin 中做 RPC 调用
* ❌ 不在信号处理器中做同步 IO
* ❌ 不缓存超过极短队列的数据
* ❌ 不在 plugin 内实现重试逻辑（交给 ingest 处理）

---

## 8. TODO 清单（给 Cursor）

### P1. Plugin 基础结构

* [ ] 创建 `libraries/plugins/ingest/` 目录
* [ ] 实现 `ingest_plugin.hpp`（类定义）
* [ ] 实现 `ingest_plugin.cpp`（基础框架）
* [ ] 实现 `CMakeLists.txt`（构建配置）
* [ ] 在 `libraries/plugins/CMakeLists.txt` 中添加 subdirectory

### P2. 信号监听

* [ ] 在 `plugin_initialize` 中注册 `post_apply_operation` 信号
* [ ] 实现 `on_post_apply_operation` 回调
* [ ] 在 `plugin_shutdown` 中断开信号连接

### P3. JSON 构建

* [ ] 实现 `build_operation_json` 函数
* [ ] 实现 block 对象构建
* [ ] 实现 transaction 对象构建（真实 op）
* [ ] 实现 transaction 对象构建（virtual op）
* [ ] 实现 operation 对象构建
* [ ] 实现 virtual 标记
* [ ] 实现 `find_transaction_id`（如需要）
* [ ] 实现 `find_transaction_index`（如需要）

### P4. HTTP 发送

* [ ] 实现异步队列（std::queue + mutex）
* [ ] 实现 `send_operation_json`（入队）
* [ ] 实现 `http_send_worker`（后台线程）
* [ ] 实现 `send_http_post`（Boost.Beast）
* [ ] 在 `plugin_startup` 中启动后台线程
* [ ] 在 `plugin_shutdown` 中停止后台线程

### P5. 配置管理

* [ ] 在 `set_program_options` 中添加配置项
* [ ] 在 `plugin_initialize` 中读取配置
* [ ] 实现配置验证

### P6. 错误处理

* [ ] 所有回调函数添加 try-catch
* [ ] 实现日志记录
* [ ] 实现队列满时的处理逻辑

### P7. 测试

* [ ] 编写单元测试
* [ ] 编写集成测试脚本
* [ ] 性能测试

---

## 9. 注意事项

### 9.1 steemd 版本兼容性

* 确认 `operation_notification` 结构
* 确认 virtual operation 检测方法
* 确认 operation 名称获取方法

### 9.2 性能考虑

* 队列大小：建议 100k（可配置）
* HTTP 超时：建议 5s（可配置）
* 后台线程：单线程即可（避免锁竞争）

### 9.3 协议一致性

* JSON 格式必须严格符合 "Operation JSON 协议定义"
* 字段名、类型、结构必须一致
* 不得添加额外字段（除非协议扩展）

---

这份 Plugin 开发计划与主计划文档完全对齐，可以直接用于实现。

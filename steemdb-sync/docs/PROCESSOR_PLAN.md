# Operation Processor 设计与实施计划

> Status: Approved
> Owner: @ety001
> Last Updated: 2026-07-09

## 核心原则（覆盖一切冲突）

**sync 端是基石，web 端以 sync 为准。** web 端甚至可以重写。
因此所有字段/集合对齐冲突，一律以 sync 端已写入的 schema 为准，改 web 端。

## 0. TL;DR

新增一个独立进程 `steemdb-sync/cmd/processor`，它顺序消费 `operations` 集合，按 `op_type`
分发到各 handler，构建出 `account`、`comment`、`vote`、`reblog`、`*_reward`、`vesting_*` 等
业务集合，供 `steemdb-web` 查询。

本文件同时审计了 `steemdb-sync`（写入端）与 `steemdb-web`（读取端）之间存在的两层断层：
**集合断层** 与 **字段名断层**，并给出统一的对齐方案。

---

## 1. 背景与动机

### 1.1 现状

`steemdb-sync` 目前只写入 4 个集合：

| 集合 | 内容 | `_id` |
|------|------|-------|
| `blocks` | 区块头 | `block_num` (uint32) |
| `transactions` | 交易 | `trx_id` |
| `operations` | 全量操作（真实 + 虚拟），原始未加工 | `"block:trx:op"` |
| `meta` | 同步元数据 | `"sync_state"` |

`steemdb-web` 的 service 层查询的却是另一批集合：

```
account, account_operations, comment, vote, reblog,
vesting_deposit, vesting_withdraw, curation_reward, author_reward,
benefactor_reward, status, funds_history, witness, block ...
```

**两个集合列表几乎不重叠** —— 中间缺一整层"数据加工 / 投影层"。

### 1.2 legacy 怎么做的

legacy 的数据加工不在 PHP 里，而在 Python 守护进程 `legacy/docker/sync/sync.py` 中。
`sync.py` 把"拉块 + op 分发 + 写业务集合"塞在**同一个进程**里。我们选择拆成独立进程
（processor），理由见下节。

### 1.3 为什么用独立 processor 进程（方案 B）

- cold_ingest 已经用 plugin 高吞吐灌满了 `operations`，不该再被重 IO 拖慢；
- 解耦后，processor 可以单独水平扩展（多 worker）；
- 单进程顺序消费单条 MongoDB 文档更新即可推进位点，无锁；
- legacy 的"单进程串行"模式在冷启动时整体极慢；方案 B 让冷启动吞吐（cold_ingest）
  和加工吞吐（processor）独立。

---

## 2. 整体架构

```
cold_ingest / live_sync  →  operations 集合（已存在，不动）
        │
        ▼
processor 进程（新增，steemdb-sync/cmd/processor）
        │
        │  按 op_type 分发
        ▼
account, comment, vote, reblog, *_reward,
vesting_*, transfer, convert, follow, witness_vote,
feed_publish, pow, comment_diff, comment_options ...
```

processor 进程包含两类工作：

1. **主循环**：顺序消费 `operations`，分发到 handler，推进位点。
2. **后台 worker**：
   - account refresher（dirty 标记刷新）
   - comment rescanner（保持 7 天内文章 payout 数据新鲜）
   - props_history ticker（可选，供 dashboard）

---

## 3. 数据模型与集合对齐（关键审计）

> 这一节是计划里最重要的部分。先解决"写什么字段"才能动手写 handler。

### 3.1 集合断层（已确认）

| web 端期望的集合 | 谁负责生成 | 状态 |
|-----------------|-----------|------|
| `account` | processor（dirty 刷新） + history 进程（全量快照） | 待实现 |
| `account_operations` / `account_history` | web 源码引用了 `account_operations`，但 legacy 实际叫 `account_history`（见 3.4） | **待裁决** |
| `comment` | processor（comment handler） | 待实现 |
| `comment_diff` | processor（comment handler 子逻辑） | 待实现 |
| `vote` | processor（vote handler） | 待实现 |
| `reblog` | processor（custom_json → save_reblog） | 待实现 |
| `follow` | processor（custom_json → save_follow） | 待实现 |
| `curation_reward` | processor | 待实现 |
| `author_reward` | processor | 待实现 |
| `benefactor_reward` | processor | 待实现 |
| `vesting_deposit` | processor（transfer_to_vesting） | 待实现 |
| `vesting_withdraw` | processor（fill_vesting_withdraw） | 待实现 |
| `transfer` | processor | 待实现 |
| `convert` | processor | 待实现 |
| `feed_publish` | processor | 待实现 |
| `witness_vote` | processor | 待实现 |
| `pow` | processor | 待实现 |
| `witness` | **独立进程**（legacy `witnesses.py`） | 不在 processor 范围 |
| `witness_history` / `witness_misses` | **独立进程** | 不在 processor 范围 |
| `funds_history` | **独立进程**（legacy `history.py`） | 不在 processor 范围 |
| `status` | processor（props/height） + history.py（stats/clients） | 部分实现 |
| `props_history` | processor（props_history ticker） | 待实现 |

### 3.2 字段名断层（审计重点）

**问题**：sync 写入的集合，其字段名与 web 端期望的字段名不一致。必须在处理器写入时
对齐到 web 端期望的 schema，否则 web 端查询全部 miss。

#### 3.2.1 `blocks` 集合（重大不一致）

| 字段 | sync 写入 (`model.Block`) | web 读取 (`block_service.go`) | legacy (`block_30d`) | 裁决 |
|------|-------------------------|------------------------------|---------------------|------|
| `_id` | `block_num` (uint32) | `number` (int64) | `block_num` (int) | 见下 |
| 高度字段 | `block_num` | `number` | `block_num` | **需对齐** |
| 区块 hash | `block_id` | (未读) | `block_id` | — |
| 交易数 | `transaction_count` | `transaction_count` | — | OK |
| 操作数 | ❌ 未写 | `operation_count` | — | **需补** |
| 时间戳 | `timestamp` | `timestamp` | `_ts` | OK |

> **裁决**：web 端查询字段是 `number`（见 `block_service.go:35`、`block_service.go:54`、
> `block_service.go:111`），但 sync 写入的是 `_id = block_num` 且无 `number` 字段。
>
> 两个选项：
> - **选项 A（推荐）**：修改 web 端 service，把 `number` 全部改成 `block_num`，并按 `_id` 查。
>   这样 sync 端的 schema 保持不变（已通过 cold_ingest 大量写入，改字段成本高）。
> - **选项 B**：processor 写一份兼容字段 `{number: blockNum}` 到 block 文档（冗余，但 web 不改）。
>
> 本计划暂定 **选项 A**（改 web 端）。需要 @ety001 确认。

#### 3.2.2 `operations` 集合（不一致）

| 字段 | sync 写入 (`model.Operation`) | web 读取 (`block_service.go:197`) | 裁决 |
|------|------------------------------|----------------------------------|------|
| `_id` | `"block:trx:op"` | (未读) | — |
| 高度 | `block_num` (uint32) | `block_num` (int64) | OK（类型差异见 3.3） |
| op 序号 | `op_index` | `op_num` | **需对齐** |
| 类型 | `op_type` | `op_type` | OK |

> web 端 `block_service.go:205` 按 `op_num` 排序，但 sync 写入的是 `op_index`。
> **裁决**：修改 web 端 `block_service.go`，`op_num` → `op_index`。

#### 3.2.3 数值类型断层（隐蔽但致命）

| 字段 | sync 写入类型 | web 读取类型 | 影响 |
|------|-------------|-------------|------|
| `block_num` (operations) | `uint32` | `int64` | MongoDB 不做类型严格匹配，BSON int→long 可能 decode 失败 |
| `block_num` (blocks `_id`) | `uint32` | `int64` | 同上 |

> **裁决**：web 端 model 的 `Block.Number`、`Operation.BlockNum` 等改为与 sync 一致，
> 或统一改为 `int64`（推荐，向后兼容）。需在 web 端 model 层统一处理。

### 3.3 业务集合 schema 来源

processor 写入业务集合时，schema 以 **web 端 model 定义** 为准（因为 web 是直接消费者），
同时兼容 legacy 字段名（因为 web 端的 aggregation 大量引用了 legacy 风格的字段名如
`_ts`、`_block`、`_dirty`）。

每个 handler 写入的字段对照表见 §5（每个 handler 一节）。

### 3.4 `account_operations` 集合（已裁决）

- web 端 `account_service.go:171` 查询集合名 `account_operations`，字段 `account` / `block_time` / `op_type`。
- legacy `sync.py` **不写** `account_operations`；legacy `history.py` 写的是 `account_history`（字段完全不同：每日快照式）。
- **裁决 [A4]**：processor 为每个涉及账户的 op 写一条精简记录到 `account_operations`。
  web 端已有查询逻辑，processor 生成数据比改 web 端聚合更简单。
  schema（精简记录）：
  ```
  {
    _id:        <op_id>,            // 复用 operations 的 _id
    account:    <account_name>,     // 从 op_value 提取（见下）
    block_num:  <op.BlockNum>,
    block_time: <blockTS>,
    op_type:    <op.OpType>,
    trx_id:     <op.TrxID>,
    summary:    <op_value 精简>,     // 关键字段
    virtual:    <op.Virtual>,
  }
  ```
  账户名提取规则（每个 op_type 的关联账户）：
  - vote → voter
  - transfer → from, to（写两条）
  - comment → author
  - *_reward → curator / author / benefactor
  - vesting_* → from, to
  - custom_json(reblog) → account
  - 其余涉及账户的 op 同理

---

## 4. processor 进程设计

### 4.1 目录结构

```
steemdb-sync/
  cmd/
    processor/
      main.go                 # 进程入口
  internal/
    processor/
      processor.go            # 主循环 + 位点推进
      dispatcher.go           # op_type → handler 注册与分发
      handler.go              # OpHandler 接口定义
      cursor.go               # 位点管理（status 文档）
      context.go              # ProcessorContext（共享 db / rpc / 配置）
      account_refresher.go    # dirty account 后台刷新 worker
      comment_rescanner.go    # comment 重扫后台 worker
      props_updater.go        # 可选：props_history ticker
      handlers/
        comment.go            # comment + comment_options + comment_diff
        vote.go
        transfer.go
        convert.go
        rewards.go            # curation_reward / author_reward / benefactor_reward
        vesting.go            # transfer_to_vesting / fill_vesting_withdraw
        custom_json.go        # reblog / follow
        witness.go            # witness_vote
        misc.go               # feed_publish / pow
```

### 4.2 主循环（无锁顺序消费）

```
启动:
  1. 读 status 文档 {_id: "processor_height"} → 拿到上次消费位点 H
  2. 启动后台 worker（account_refresher / comment_rescanner）
  3. 进入主循环

主循环:
  nextBlock = H + 1
  1. 从 operations 集合查 block_num = nextBlock 的全部 ops（按 _id 升序，保证 op 顺序）
     - 若结果为空：说明 live_sync 还没追到这里 → sleep 1s → continue
  2. 从 blocks 集合查 nextBlock 的 timestamp（handler 需要 _ts）
     - 若 block 文档不存在：用 operations 中第一条的时间兜底（极少见）
  3. 按 op 顺序逐条 dispatch(op, blockTimestamp)
     - handler 内部 upsert 到各自业务集合
     - 单条 op 失败：记日志，不中断，继续下一条（对齐 legacy 行为）
  4. 原子推进位点：
       db.status.update_one(
         {_id: "processor_height"},
         {$set: {value: nextBlock, updated_at: now}},
         {upsert: true}
       )
  5. H = nextBlock，goto 1
```

**无锁说明**：位点推进是单进程对单文档的 `update_one`，MongoDB 文档级原子，不需要
分布式锁。processor 只部署一个实例（顺序语义要求）。如需多实例，必须按 block 区间分片，
不允许两个实例消费相同 block。

### 4.3 位点管理

- 集合：`status`（复用 legacy 的 status 集合）
- 文档：`{_id: "processor_height", value: <blockNum>, updated_at: <time>}`
- 启动时若文档不存在 → 从 `meta.max_block` 或 `blocks` 最大值初始化为 0
- 支持启动参数 `--start-height=N` 强制从头或从指定高度开始（调试 / 回放用）

### 4.4 OpHandler 接口

```go
// OpHandler processes a single operation and writes to business collections.
type OpHandler interface {
    Handle(ctx context.Context, op *model.Operation, blockTS time.Time) error
}

// Dispatcher maps op_type → handler.
type Dispatcher struct {
    handlers map[string]OpHandler
    fallback OpHandler // unknown op_type 的兜底（只记日志，不写）
}

func (d *Dispatcher) Register(opType string, h OpHandler)
func (d *Dispatcher) Dispatch(ctx context.Context, op *model.Operation, blockTS time.Time) error
```

- handler 收到的 `op.OpValue` 是 `map[string]interface{}`，等价于 legacy `sync.py` 的 `op` dict。
- handler 收到的 `blockTS` 是该 block 的 timestamp（已查好，避免每个 handler 各查一次）。

### 4.5 配置项（`config.yaml` 新增 section）

```yaml
processor:
  enabled: true
  batch_size: 1            # 单 block 内 op 逐条处理（顺序语义）；未来可按 block 批量
  catch_up_sleep: 1s       # 等待 live_sync 时 sleep
  start_height: 0          # 0 = 从 status.processor_height 续跑

  account_refresher:
    enabled: true
    interval: 30s          # 多久跑一次刷新
    batch_size: 500        # 每次拉多少个 dirty 账户名
    rpc_batch_size: 100    # 每次 get_accounts 传多少个账户名
    workers: 8             # 并发 worker 数
    cold_start_pause: true # 冷启动期间暂停（见 §6）

  comment_rescanner:
    enabled: true
    interval: 60s
    window_days: 3
    stale_hours: 6
    batch_size: 100
    workers: 5

  props_updater:
    enabled: false         # 默认关，dashboard 可走 web 端直连 RPC
    interval: 60s
```

---

## 5. Handler 详细规格（每个 op_type 一节）

> 字段对照：左边是写入 MongoDB 的字段，右边是来源。
> 所有 handler 必须做：金额字符串 split+float、时间戳 parse、`_ts` 填充。
> 涉及账户的 op 必须 `queue_update_account(name)`（写 `_dirty: true`）。

### 5.1 `comment` → 集合 `comment` (+ `comment_diff`)

对应 legacy `update_comment()`（sync.py:303）。

**特殊**：comment handler 需要调用 `rpc.get_content(author, permlink)` 获取完整 post 状态
（active_votes、payout、cashout_time 等），是最重的 handler。

写入字段（对齐 web 端 `models.Comment` + legacy 字段）：

| 字段 | 来源 | 说明 |
|------|------|------|
| `_id` | `author + "/" + permlink` | |
| `author` / `permlink` / `title` / `body` | op value | |
| `json_metadata` | `rpc.get_content().json_metadata` → JSON decode | web 端期望 map |
| `parent_author` / `parent_permlink` | op value | |
| `category` / `depth` / `children` | get_content | |
| `created` / `last_update` / `cashout_time` / `last_payout` / `active` / `max_cashout_time` | get_content，parse 日期 | |
| `pending_payout_value` / `total_payout_value` / `curator_payout_value` / `max_accepted_payout` / `total_pending_payout_value` | get_content，split+float | |
| `author_reputation` / `net_rshares` / `children_abs_rshares` / `abs_rshares` / `vote_rshares` | get_content，float | |
| `net_votes` | get_content | |
| `active_votes` | get_content，每项 rshares/weight→float, time→parse | |
| `url` | get_content | 用于计算 parent 更新 |
| `block_num` | op.BlockNum | |
| `scanned` | now() | |
| `mode` | get_content | web 端 comment_rescanner 依赖 |
| `author_lower` / `category_lower` / `date_idx` | 派生 | lowercase / `YYYY-MM-DD` |

子逻辑：
- 若 `op.body` 以 `"@@ "` 开头 → 写 `comment_diff`（`_id = block/op author/permlink`）
- 若 `depth > 0` 且是新建 → 更新父 comment 的 `last_reply` / `last_reply_by`

### 5.2 `comment_options` → 集合 `comment`

对应 legacy `update_comment_options()`（sync.py:375）。
只更新 `comment` 文档的 `options` 子字段，`_id = author/permlink`。

### 5.3 `vote` → 集合 `vote`

对应 legacy `save_vote()`（sync.py:279）。

| 字段 | 来源 |
|------|------|
| `_id` | `blockid/voter/author/permlink` |
| `_ts` | blockTS |
| `voter` / `author` / `permlink` / `weight` | op value |

> 注：legacy 写的是完整 op dict；web 端 `models.Vote` 只读 voter/weight/rshares/percent/time。
> 处理器写入完整 op + `_ts`，web 端按需取字段即可。

### 5.4 `transfer` → 集合 `transfer`

对应 legacy `save_transfer()`（sync.py:109）。

| 字段 | 来源 |
|------|------|
| `_id` | `blockid/from/to` |
| `_ts` | blockTS |
| `amount` | op.amount split → float |
| `type` | op.amount split → unit（STEEM/SBD） |
| 其余 op 字段 | 原样 |

`queue_update_account(from)`；若 `from != to` 再 queue `to`。

### 5.5 `convert` → 集合 `convert`

对应 legacy `save_convert()`（sync.py:97）。`_id = blockid/requestid`，amount split+float。

### 5.6 `curation_reward` → 集合 `curation_reward`

对应 legacy `save_curation_reward()`（sync.py:123）。`_id = blockid/curator/comment_author/comment_permlink`，
reward split+float。`queue_update_account(curator)`。

### 5.7 `author_reward` → 集合 `author_reward` (+ 更新 `comment.reward`)

对应 legacy `save_author_reward()`（sync.py:134）。

| 字段 | 来源 |
|------|------|
| `_id` | `blockid/author/permlink` |
| `_ts` | blockTS |
| `sbd_payout` / `steem_payout` / `vesting_payout` | split+float |
| `app_name` / `app_version` | 从 `comment.json_metadata.app` split('/') |
| 其余 op 字段 | 原样 |

子逻辑：先 `update_comment(author, permlink)` 刷新（拿 app 信息），再写 reward，
再把 reward 写回 `comment.reward`。`queue_update_account(author)`。

### 5.8 `benefactor_reward` → 集合 `benefactor_reward`

对应 legacy `save_benefactor_reward()`（sync.py:227）。
query = `{_block, benefactor, permlink, author}`，reward = vesting_payout split+float。

### 5.9 `transfer_to_vesting` → 集合 `vesting_deposit`

对应 legacy `save_vesting_deposit()`（sync.py:159）。`_id = blockid/from/to`，amount split+float。
queue from / to。

### 5.10 `fill_vesting_withdraw` → 集合 `vesting_withdraw`

对应 legacy `save_vesting_withdraw()`（sync.py:172）。`_id = blockid/from_account/to_account`，
deposited/withdrawn split+float。queue from/to。

### 5.11 `custom_json` → 集合 `reblog` / `follow`

对应 legacy `save_custom_json()`（sync.py:186）。解析 `op.json`：
- `[0] == "reblog"` → `save_reblog`：query `{_block, permlink, account}`，写 `_ts`
- `[0] == "follow"` → `save_follow`：query `{_block, follower, following}`，写 `_ts`，queue 双方

### 5.12 `feed_publish` → 集合 `feed_publish`

对应 legacy `save_feed_publish()`（sync.py:197）。`_id = blockid|publisher`，
exchange_rate.base/quote split+float。

### 5.13 `account_witness_vote` → 集合 `witness_vote`

对应 legacy `save_witness_vote()`（sync.py:288）。query `{_ts, account, witness}`。queue 双方。

### 5.14 `pow` / `pow2` → 集合 `pow`

对应 legacy `save_pow()`（sync.py:265）。`_id = blockid + "-" + worker_account`。

### 5.15 未覆盖的 op_type

以下 op_type legacy 未处理，processor 也跳过（只记 debug 日志）：
`account_create`, `account_update`, `account_witness_proxy`, `witness_update`,
`limit_order_*`, `escrow_*`, `delete_comment`, `custom`, `claim_*`, 等等。

> **审计提示**：`account_update` / `witness_update` legacy 虽然不直接处理，但通过
> dirty account 机制间接覆盖（账户刷新时 get_accounts 拿到最新状态）。所以跳过是安全的。

---

## 6. account dirty 刷新优化（重点）

### 6.1 legacy 为什么慢

- `update_queue()` 每个 60s 主循环跑一次；
- 每次 `db.account.find({_dirty: true}).limit(20)`；
- 每个账户单独 `rpc.get_accounts([name])`（**每次只传 1 个账户名**）；
- 速率 ≈ **20 账户 / 分钟 = 1200 / 小时**；
- 百万级账户 → 刷一遍约 35 天。这是 legacy 运行时间最长的环节。

### 6.2 三个致命点与对策

| 致命点 | legacy | 优化 |
|--------|--------|------|
| 1. 冷启动期间刷新语义错误 | 回放中按 dirty 刷新，但 get_accounts 返回链头状态，贴到回放过程 = 错数据 + 巨大浪费 | 冷启动期间暂停 refresher，改为冷启动结束后一次性全量扫描 |
| 2. get_accounts 没用批量 | 每次传 `[name]`（1 个） | 每次传 `[name1..name100]`（100 个）→ **100 倍提速** |
| 3. 串行 + 小线程池 | 与 op 处理同循环，互相阻塞 | 独立 worker pool，并发 8~16 |

### 6.3 processor 的 account refresher 设计

```go
// 每 interval（默认 30s）触发一次
func (r *AccountRefresher) tick(ctx):
    if r.coldStartMode:
        return  // 冷启动期间暂停（见 6.2）

    // 1. 批量拉 dirty 账户名（limit batch_size=500）
    names = db.account.find({_dirty: true}, {name:1}).limit(500)

    // 2. 按 rpc_batch_size=100 切片
    for chunk in chunkBy(names, 100):
        submitToWorkerPool(chunk)  // workers=8 并发

// worker：
func (r *AccountRefresher) worker(chunk []string):
    accounts = rpc.getAccounts(chunk)  // 批量 RPC，100 个一次
    for acct in accounts:
        acct = processAccountDetails(acct)  // 类型转换（见 6.4）
        acct.scanned = now
        delete(acct._dirty)
        db.account.update_one({_id: acct.name}, {$set: acct}, upsert)
```

### 6.4 `processAccountDetails` 字段转换（对齐 legacy `update_account` sync.py:383）

| 字段 | 转换 |
|------|------|
| `proxy_witness` | `proxied_vsf_votes[0] / 1000000` |
| `reputation` / `to_withdraw` | float |
| `balance` / `sbd_balance` / `savings_balance` / `savings_sbd_balance` / `vesting_balance` / `vesting_shares` / `vesting_withdraw_rate` | split+float |
| `created` / `last_post` / `last_vote_time` / `next_vesting_withdrawal` / ... | parse 日期 |
| `total_balance` | `balance + savings_balance` |
| `total_sbd_balance` | `sbd_balance + savings_sbd_balance` |

### 6.5 冷启动结束后的全量扫描（可选，独立工具）

提供一个 `cmd/account_snapshot/`（或 processor 的一个子命令 `processor snapshot`）：
- `rpc.lookup_accounts(-1, 1000)` 分页枚举全部账户名；
- 按 100 个一批 `get_accounts`；
- 全量 upsert 到 `account`，并写一份到 `account_history`（对齐 legacy `history.py`）。

这对应 legacy 的 `history.py`，可单独排期，不在 processor 第一步范围。

### 6.6 预期加速

| 配置 | 吞吐 |
|------|------|
| legacy | 1,200 / 小时 |
| processor（batch=100, workers=8） | ~960,000 / 小时（800x） |
| processor（batch=100, workers=16） | ~1,920,000 / 小时（1600x） |

冷启动期间暂停 → 冷启动结束后一次全量扫描约 1~3 分钟（百万账户）。

---

## 7. comment 重扫队列（对齐 legacy `update_queue`）

legacy `update_queue` 还做两件事，processor 需要复刻（可放到第二批）：

1. **近 3 天文章重扫**：`created > now-3d AND scanned < now-6h`，重新 `get_content`
   刷新 payout 数据（7 天 payout 窗口内数据会变）。
2. **past payout 重扫**：`cashout_time < now AND mode in (first/second_payout) AND pending > 0`。

processor 的 `comment_rescanner` worker：每 60s 跑一次，线程池并发调用 comment handler
的 `update_comment` 逻辑。

---

## 8. props_history ticker（可选）

legacy sync.py 有个独立线程每 60s 拉 `get_dynamic_global_properties` 写 `status.props` /
`status.steem_per_mvests` / `props_history`。

web 端 dashboard 当前直接走 steemClient RPC，不依赖 `status.props`。所以这个 ticker
**默认关闭**，需要时再开。

---

## 9. web 端需配套修改（Batch 6，以 sync 端为准）

### 9.1 集合名 + 字段对齐（裁决 A1/A2/A3）

| web 端改动 | 原值 | 新值（对齐 sync） |
|-----------|------|-----------------|
| 集合名 | `"block"` | `"blocks"` |
| 集合名 | `"operation"` | `"operations"` |
| block 查询/排序字段 | `number` | `_id`（= block_num） |
| operation 排序字段 | `op_num` | `op_index` |
| models/block.go bson tag | `bson:"number"` | `bson:"_id"` |
| database/mongodb.go:139 索引 | `number` | `_id`（或删除冗余索引） |

### 9.2 缺失的路由与空壳

- `routes.go` 补 `witnesses` 路由组（前端调 `/witnesses`、`/witnesses/top`、`/witnesses/:name`）
- `GetClients`（labs_service.go:863）TODO 空壳 → 解析 `status.clients-snapshot`（依赖独立 history 进程）

---

## 10. 实施批次

### Batch 0：计划审计与裁决（本文件）⚠️

必须先解决 §11 的所有 Open Questions，才能动手。

### Batch 1：processor 骨架 + 4 个简单 handler

- `cmd/processor/main.go`
- `internal/processor/processor.go`（主循环 + 位点）
- `internal/processor/dispatcher.go` + `handler.go`
- `internal/processor/cursor.go`
- 4 个 handler：`vote`、`transfer`、`curation_reward`、`author_reward`
- 配置项 `processor.*`
- 单元测试：dispatcher 注册/分发、cursor 推进、位点幂等

**验收**：对一个已知 block 跑 processor，`vote` / `transfer` 等集合出现正确文档，
位点正确推进，重启后续跑不重复。

### Batch 2：comment handler（最重）

- `comment` + `comment_options` + `comment_diff`
- 调 `rpc.get_content`
- 父 comment 的 `last_reply` 更新

**验收**：comment 集合 schema 与 web 端 `models.Comment` 对齐，前端 Posts 页面能出数据。

### Batch 3：account refresher + dirty 机制

- `queue_update_account`（在各 handler 里加 dirty 标记）
- `account_refresher.go`（批量 + worker pool）
- 冷启动暂停逻辑
- `processAccountDetails` 字段转换

**验收**：冷启动期间不刷新；结束后批量刷新；account 集合 schema 对齐 web 端 `models.Account`。

### Batch 4：剩余 handler

- `convert`、`benefactor_reward`、`transfer_to_vesting`、`fill_vesting_withdraw`、
  `custom_json`(reblog/follow)、`feed_publish`、`account_witness_vote`、`pow/pow2`

**验收**：labs 各页面（powerup/powerdown/curation/author/benefactors/flags/clients）数据正确。

### Batch 5：comment rescanner + 可选 props ticker

- comment 重扫队列（近 3 天 + past payout）
- props_history ticker（默认关）

### Batch 6：web 端配套修改（与各 Batch 穿插）

- block_service 字段对齐
- 集合名 `block` → `blocks`（或反向）
- 补 witnesses 路由
- 补 operation_count

### Batch 7（独立排期，不在 processor 范围）

- `history.py` 的 Go 重写（account_history / funds_history / stats / clients）
- `witnesses.py` 的 Go 重写（witness / witness_history / witness_misses）

---

## 11. 审计与 Open Questions

> 以下所有项均已对照源码逐行核实（审计时间 2026-07-09），并已全部裁决。
> 裁决原则：sync 端是基石，冲突一律改 web 端。

### A. 字段/集合对齐类（全部已核实属实，已裁决）

- **[A1]** ✅ 裁决：统一为 `blocks`（sync 端已用），web 端 7 处 `"block"` → `"blocks"`。
  源码核实：web `block_service.go:34` 等 7 处；sync `mongo/mongodb.go:51`。

- **[A2]** ✅ 裁决：web 端 block 查询统一用 `_id`（sync 端 `_id = block_num`）。
  web 端所有 `number` 字段引用改为 `_id` 或 `block_num`。
  涉及：`block_service.go`（4处）、`dashboard_service.go`（3处）、
  `models/block.go`（bson tag）、`database/mongodb.go:139`（索引）。

- **[A3]** ✅ 裁决：web 端 `op_num` → `op_index`（对齐 sync `model.Operation.OpIndex`）。
  涉及：`block_service.go:205`、`models/block.go:45`。

- **[A4]** ✅ 裁决：processor 为每个涉及账户的 op 写一条精简记录到 `account_operations`。
  理由：web 端已有查询逻辑（`account_service.go:171`），processor 生成数据比改 web 端聚合更简单。
  schema 见 §5 各 handler（每个涉及账户的 op 额外写 account_operations）。

### B. 数据一致性类

- **[B1]** ✅ 裁决：冷启动判断改用动态判断（位点落后链头超过 N 块 → 视为追赶中）。
  不依赖 `meta.cold_start_done`（避免跳过 cold_ingest 场景下 refresher 永久暂停）。
  实现方式：processor 主循环每 tick 查 `meta.max_block`（live_sync 在推进它），
  若 `max_block - processor_height > threshold`（默认 1000）→ refresher 暂停。

- **[B2]** ✅ 裁决：接受。comment handler 调 `rpc.get_content` 拿链头状态，与 legacy 一致。

### C. 运维类

- **[C1]** ✅ 裁决：接受。崩溃重启靠 upsert 幂等保证数据正确，重复 IO 可接受。

- **[C2]** ✅ 裁决：processor 用 `:9092/metrics`。

- **[C3]** ✅ 裁决：进 docker-compose，新增 `steemdb-processor` service。

### D. 范围边界类

- **[D1]** ✅ 裁决：`account_history` / `funds_history` / `witness` /
  `witness_history` / `clients-snapshot` 不在 processor 范围，
  由独立的 history / witnesses 进程生成（对应 legacy `history.py` / `witnesses.py`），单独排期。

- **[D2]** ✅ 裁决：第一版单进程顺序消费，不支持多实例分片。

---

## 12. 风险

1. **comment handler 的 RPC 压力**：每个 comment op 一次 get_content。历史回放 comment 量大
   （千万级），会打爆 RPC。对策：限速 + 冷启动期间可选跳过 get_content（只写 op 原始字段，
   留 dirty 标记后续补全）。
2. **字段断层如果漏改**：web 端某个页面静默无数据。对策：Batch 1 完成后立即跑一次端到端
   烟雾测试（cold_ingest 灌几个 block → processor 处理 → web API 查询）。
3. **MongoDB 类型不一致**：uint32 vs int64。需统一。
4. **legacy 集合名大小写/单复数**：legacy 用 `block_30d`，web 用 `block`，sync 用 `blocks`。
   三者不一致，必须统一裁决。

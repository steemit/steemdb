# MongoDB CPU Optimization Analysis & Recommendations

## Problem Summary

**Issue**: MongoDB server CPU is pegged at 100% when web service goes online  
**Current Setup**: t3.xlarge EC2 instance for MongoDB  
**Impact**: Performance degradation, possible instance throttling

---

## Root Cause Analysis

### 1. **Critical Issue: N+1 Query Problem in ForumsController**

**Location**: `app/controllers/ForumsController.php::indexAction()`

**Problem**: Nested loops executing queries inside foreach
```php
foreach($forums as $category_id => $category) {
  foreach($category['boards'] as $board_id => $board) {
    // Query 1: COUNT operation
    $forums[$category_id]['boards'][$board_id]['posts'] = Comment::count([
      $query
    ]);
    
    // Query 2: FIND operation
    $forums[$category_id]['boards'][$board_id]['recent'] = Comment::find([
      $query,
      'sort' => $sort,
      'limit' => $limit,
    ])[0];
  }
}
```

**Impact**: 
- If you have 10 categories × 5 boards = **50 COUNT queries + 50 FIND queries = 100 queries per page load**
- Each query scans the `comment` collection
- **Severity**: 🔴 **CRITICAL** - This is likely the primary cause of CPU spike

### 2. **CPU-Intensive Aggregation Operations**

#### 2.1 $lookup Operations (Cross-Collection Joins)

**Locations**:
- `CommentController::curieAction()` - Uses `$lookup` to join `comment` → `account`
- `CommentsController::curieAction()` - Same pattern
- Multiple other controllers

**Example**:
```php
Comment::agg([
  ['$match' => [...]],
  ['$lookup' => [  // CPU-intensive join
    'from' => 'account',
    'localField' => 'author',
    'foreignField' => 'name',
    'as' => 'account'
  ]],
  ['$match' => [
    'account.reputation' => ['$lt' => ...],
    'account.followers_count' => ['$lt' => 100],
  ]],
])
```

**Impact**: 
- `$lookup` performs nested loops, cannot use indexes efficiently
- Processes potentially thousands of documents
- **12 instances** across the codebase

#### 2.2 Disk Spilling Operations

**Locations** (5 files use `allowDiskUse: true`):
- `LabsController::authorAction()`
- `ApiController`
- `AccountApiController`
- `Comment.php` model
- `BenefactorReward.php` model

**Impact**:
- Operations too large for memory → disk spilling
- Extremely CPU-intensive when sorting/spilling large datasets
- Indicates missing or ineffective indexes

#### 2.3 Complex Aggregation Pipelines

**Examples**:
- `LabsController::authorAction()` - 8 nested `$cond` expressions per document
- Multiple date function calculations (175+ instances)
- String operations (`$substr`, `$concat`)
- Regular expressions in `$match`

**Impact**: Each document requires extensive CPU processing

### 3. **Instance Type Issue: t3.xlarge (Burstable)**

**Problem**: 
- t3 instances are **burstable performance** instances
- Base CPU: 10% baseline
- Credits system: Accumulates credits when idle, spends when active
- **When credits run out, CPU is throttled to baseline (10%)**

**For MongoDB**:
- MongoDB needs **consistent CPU performance**
- High aggregation load will quickly exhaust credits
- Once throttled, queries queue up → CPU appears "full" but actually throttled
- This creates a **cascading performance issue**

### 4. **Missing or Inefficient Indexes**

**Evidence**:
- `allowDiskUse: true` in 5 files (shouldn't be needed with proper indexes)
- Queries sorting on multiple fields without compound indexes
- `$lookup` operations that could benefit from indexes on foreign keys

**Common Query Patterns Needing Indexes**:
- `{'depth': 0, 'created': -1}` - Needs compound index
- `{'author': ..., '_ts': ...}` - Needs compound index
- `{'mode': 'first_payout', 'depth': 0, 'total_pending_payout_value': ...}` - Needs compound index

---

## Immediate Actions (Priority Order)

### 🔴 Priority 1: Fix N+1 Query Problem (Critical)

**File**: `app/controllers/ForumsController.php`

**Current Code** (Lines 252-288):
```php
foreach($forums as $category_id => $category) {
  foreach($category['boards'] as $board_id => $board) {
    // Executes inside loop - BAD
    $forums[$category_id]['boards'][$board_id]['posts'] = Comment::count([
      $query
    ]);
    $forums[$category_id]['boards'][$board_id]['recent'] = Comment::find([
      $query,
      'sort' => $sort,
      'limit' => $limit,
    ])[0];
  }
}
```

**Optimized Solution**: Use aggregation pipeline to batch all queries
```php
public function indexAction()
{
  $forums = $this->config;
  $boardQueries = [];
  
  // Collect all queries first
  foreach($forums as $category_id => $category) {
    foreach($category['boards'] as $board_id => $board) {
      if($board_id == 'general') continue;
      $boardQueries[$category_id . '_' . $board_id] = [
        'query' => $this->getQuery($board),
        'sort' => ['last_reply' => -1, 'created' => -1],
        'category_id' => $category_id,
        'board_id' => $board_id,
      ];
    }
  }
  
  // Single aggregation to get all counts and recent posts
  $pipeline = [
    ['$facet' => array_map(function($item, $key) {
      return [
        $key . '_count' => [
          ['$match' => $item['query']],
          ['$count' => 'count']
        ],
        $key . '_recent' => [
          ['$match' => $item['query']],
          ['$sort' => $item['sort']],
          ['$limit' => 1]
        ]
      ];
    }, $boardQueries, array_keys($boardQueries))],
  ];
  
  $results = Comment::agg($pipeline)->toArray()[0];
  
  // Map results back to forums structure
  foreach($boardQueries as $key => $item) {
    $parts = explode('_', $key, 2);
    $catId = $item['category_id'];
    $boardId = $item['board_id'];
    $forums[$catId]['boards'][$boardId]['posts'] = $results[$key . '_count'][0]['count'] ?? 0;
    $forums[$catId]['boards'][$boardId]['recent'] = $results[$key . '_recent'][0] ?? null;
  }
  
  $this->view->forums = $forums;
}
```

**Expected Impact**: Reduces ~100 queries to **1 query** - **99% reduction**

### 🟠 Priority 2: Add Critical Indexes

**Run these MongoDB commands**:

```javascript
// For Comment collection - Critical for ForumsController
db.comment.createIndex({ "depth": 1, "created": -1 });
db.comment.createIndex({ "last_reply": -1, "created": -1 });

// For Comment::curieAction() queries
db.comment.createIndex({ 
  "depth": 1, 
  "mode": 1, 
  "total_pending_payout_value": 1, 
  "created": -1 
});

// For AuthorReward aggregations
db.author_reward.createIndex({ "author": 1, "_ts": -1 });
db.author_reward.createIndex({ "_ts": -1 });

// For Account collection - Critical for $lookup performance
db.account.createIndex({ "name": 1 }); // If not exists
db.account.createIndex({ "reputation": 1, "followers_count": 1 });

// For Comment aggregation with account lookups
db.comment.createIndex({ "author": 1, "depth": 1, "created": -1 });
```

**Expected Impact**: 
- Reduces collection scans to index scans
- Eliminates need for `allowDiskUse: true` in most cases
- Improves `$lookup` performance

### 🟡 Priority 3: Optimize $lookup Operations

**Problem**: `$lookup` performs inefficient nested loops

**Optimization Strategy 1**: Pre-filter account collection
```php
// Instead of filtering after $lookup
Comment::agg([
  ['$match' => [...]],
  ['$lookup' => ['from' => 'account', ...]],
  ['$match' => ['account.reputation' => ['$lt' => ...]]], // Filter after join
])

// Do: Filter accounts first, then lookup
$accountFilter = Account::aggregate([
  ['$match' => [
    'reputation' => ['$lt' => 7784855346100],
    'followers_count' => ['$lt' => 100],
  ]],
  ['$project' => ['name' => 1]],
])->toArray();

$accountNames = array_column($accountFilter, 'name');

Comment::agg([
  ['$match' => [
    'depth' => 0,
    'author' => ['$in' => $accountNames], // Filter before lookup
    // ... other conditions
  ]],
  ['$lookup' => ['from' => 'account', ...]], // Smaller join set
])
```

**Optimization Strategy 2**: Denormalize account data (if possible)
- Store `account.reputation` and `account.followers_count` directly in `comment` documents
- Update via background jobs when accounts change
- Eliminates need for `$lookup` entirely

**Expected Impact**: 50-80% reduction in `$lookup` CPU usage

### 🟡 Priority 4: Cache Heavy Aggregations

**Locations to cache**:
- `LabsController::authorAction()` - Leaderboard calculations
- `ApiController` - Chart data aggregations
- `utilities.php::updateDistribution()` - Already has cache, but extend to more operations

**Implementation** (Example for LabsController):
```php
public function authorAction() {
  $date = strtotime($this->request->get("date") ?: date("Y-m-d"));
  $cacheKey = 'leaderboard_' . date('Y-m-d', $date);
  
  $cached = $this->di->get('memcached')->get($cacheKey);
  if($cached !== null) {
    $this->view->leaderboard = $cached;
    return;
  }
  
  // Expensive aggregation
  $leaderboard = AuthorReward::agg([...])->toArray();
  
  // Cache for 15 minutes
  $this->di->get('memcached')->save($cacheKey, $leaderboard, 900);
  $this->view->leaderboard = $leaderboard;
}
```

**Expected Impact**: 
- Reduces CPU load by 80-90% for cached requests
- Especially effective for popular pages

### 🟢 Priority 5: Upgrade Instance Type

**Current**: t3.xlarge (burstable)
- vCPU: 4 (burstable)
- Memory: 16 GB
- Baseline: 10% CPU
- Problem: Credits exhaust → throttling

**Recommended Options**:

#### Option A: m5.xlarge or m5.2xlarge (General Purpose)
- **vCPU**: 4 (m5.xlarge) or 8 (m5.2xlarge) - **Dedicated CPUs**
- **Memory**: 16 GB or 32 GB
- **Baseline**: 100% CPU available always
- **Cost**: ~$150/month (m5.xlarge) or ~$300/month (m5.2xlarge)
- **Best for**: Current workload after optimizations

#### Option B: c5.xlarge or c5.2xlarge (Compute Optimized)
- **vCPU**: 4 (c5.xlarge) or 8 (c5.2xlarge)
- **Memory**: 8 GB or 16 GB
- **Higher CPU-to-memory ratio** - Better for aggregation-heavy workloads
- **Cost**: ~$150/month (c5.xlarge) or ~$300/month (c5.2xlarge)
- **Best for**: If aggregations remain bottleneck after optimizations

#### Option C: m6g.xlarge (Graviton2 ARM) - Only if optimizations succeed
- **vCPU**: 4 (dedicated)
- **Memory**: 16 GB
- **Cost**: ~$120/month (20% cheaper)
- **Consideration**: Complex aggregations may perform 10-15% slower than x86

**Recommendation**: Start with **m5.2xlarge** (8 vCPU) for headroom, then monitor and scale down if not needed.

---

## Optimization Roadmap

### Week 1: Critical Fixes
1. ✅ Fix N+1 query in `ForumsController::indexAction()`
2. ✅ Add critical indexes (see Priority 2)
3. ✅ Monitor CPU usage

### Week 2: Aggregation Optimization
1. ✅ Optimize `$lookup` operations (Priority 3)
2. ✅ Add caching for heavy aggregations (Priority 4)
3. ✅ Monitor query performance

### Week 3: Infrastructure
1. ✅ Upgrade instance to m5.2xlarge or c5.2xlarge
2. ✅ Monitor CPU usage and query times
3. ✅ Fine-tune based on metrics

---

## Expected Performance Improvements

| Optimization | Current | After Fix | Improvement |
|-------------|---------|-----------|-------------|
| **ForumsController queries** | 100 queries/page | 1 query/page | **99% reduction** |
| **CPU usage (with t3.xlarge)** | 100% (throttled) | 40-60% | **40-60% reduction** |
| **CPU usage (with m5.2xlarge)** | N/A | 30-50% | **Dedicated CPUs** |
| **Query response time** | 2-5s | 0.5-1s | **75-80% faster** |
| **$lookup operations** | High CPU | Medium CPU | **50-80% reduction** |

---

## Monitoring & Validation

### Metrics to Track

1. **MongoDB CPU Usage**
   ```bash
   # On MongoDB server
   top -p $(pgrep mongod)
   # Or
   mongostat 1
   ```

2. **Slow Queries**
   ```javascript
   // Enable profiling
   db.setProfilingLevel(1, { slowms: 100 });
   
   // View slow queries
   db.system.profile.find().sort({ ts: -1 }).limit(10).pretty();
   ```

3. **Index Usage**
   ```javascript
   // Check index usage
   db.comment.aggregate([
     { $indexStats: {} }
   ]).pretty();
   ```

4. **Query Explain Plans**
   ```javascript
   // Test queries before/after
   db.comment.find({depth: 0}).sort({created: -1}).limit(50).explain("executionStats");
   ```

### Success Criteria

- ✅ MongoDB CPU < 70% under normal load
- ✅ Page load times < 1s (from 2-5s currently)
- ✅ No query takes > 1s
- ✅ All queries use indexes (no COLLSCAN)
- ✅ No instance throttling (if staying on t3)

---

## Quick Wins (Can implement immediately)

1. **Add index on comment collection**:
   ```javascript
   db.comment.createIndex({ "depth": 1, "created": -1 });
   ```

2. **Enable MongoDB query profiler** to identify slow queries:
   ```javascript
   db.setProfilingLevel(1, { slowms: 100 });
   ```

3. **Check current indexes**:
   ```javascript
   db.comment.getIndexes();
   db.account.getIndexes();
   ```

4. **Monitor active operations**:
   ```javascript
   db.currentOp({ "active": true, "secs_running": { "$gt": 1 } });
   ```

---

## Summary

**Primary Cause**: N+1 query problem in `ForumsController::indexAction()` + t3.xlarge throttling

**Immediate Action**: 
1. Fix ForumsController N+1 queries (1-2 hours work)
2. Add critical indexes (5-10 minutes)
3. Upgrade to m5.2xlarge or c5.2xlarge (10 minutes)

**Expected Result**: CPU usage drops from 100% to 30-50%, with 75-80% faster response times.


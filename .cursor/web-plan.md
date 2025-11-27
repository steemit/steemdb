# SteemDB Web系统重构方案

## 项目概述
将SteemDB从PHP后端重构为Go统一服务，整合Live服务，采用现代前端技术栈，图表功能作为最后阶段实现

## 技术选型

### 后端技术栈
- **语言**: Go 1.21+
- **Web框架**: Gin (高性能HTTP路由)
- **WebSocket**: Gorilla WebSocket (替代Python Live服务)
- **数据库**: MongoDB (保持现有)
- **缓存**: Redis
- **配置**: Viper + YAML

### 前端技术栈
- **构建工具**: Vite
- **框架**: React 18 + TypeScript
- **样式**: Tailwind CSS + Shadcn/ui
- **状态管理**: TanStack Query + Zustand
- **图表库**: 后期实现 (Recharts/ApexCharts)

## 实施阶段

### 阶段1: Go后端核心API (优先级: 最高)
**目标**: 替代PHP后端的核心API功能
**时间**: 4-6周

#### 核心功能迁移
1. **账户相关API**
   - GET /api/accounts/:name - 账户基本信息
   - GET /api/accounts/:name/history - 账户历史
   - GET /api/accounts/:name/followers - 关注者列表
   - GET /api/accounts/:name/posts - 用户文章

2. **区块链数据API**
   - GET /api/blocks/:number - 区块详情
   - GET /api/blocks/latest - 最新区块
   - GET /api/posts/:author/:permlink - 文章详情
   - GET /api/witnesses - 见证人列表

3. **统计数据API**
   - GET /api/stats/overview - 概览统计
   - GET /api/stats/supply - 供应量数据
   - GET /api/health - 健康检查

### 阶段2: WebSocket实时服务集成 (优先级: 高)
**目标**: 将Python Live服务集成到Go后端
**时间**: 2-3周

### 阶段3: React前端基础功能 (优先级: 中)
**目标**: 实现核心页面和基础交互
**时间**: 6-8周

### 阶段4: 高级功能和优化 (优先级: 中低)
**目标**: 完善用户体验和性能优化
**时间**: 3-4周

### 阶段5: 图表数据展示 (优先级: 最低)
**目标**: 实现数据可视化功能
**时间**: 2-3周

## 成功指标

### 性能目标
- API响应时间 < 50ms (P95)
- WebSocket连接数 > 5000
- 首屏加载时间 < 2秒
- 内存使用 < 200MB

### 功能目标
- 100%兼容现有API
- 实时数据延迟 < 1秒
- 移动端完美适配
- SEO友好的页面结构
---
name: anna-yutu-executa
description: "在 ANNA 需要通过 yutu-executa 处理 YouTube 任务时使用。适用于查询频道、搜索和分析视频、读取频道统计、管理播放列表、评论、订阅、字幕、缩略图和其他 YouTube Data API 资源。指导 ANNA 选择正确的 run_yutu 命令数组、先查后改、先用 handle 查 channelId、在破坏性操作前做二次校验。"
---

# ANNA 使用 yutu-executa

这个 skill 用来指导 ANNA 通过 `yutu-executa` 暴露的 `run_yutu` 工具操作 YouTube 资源。

这里不再解释插件发现、JSON-RPC 协议细节，也不再解释身份认证注入。默认这些能力都已经具备。

## 何时使用

当用户希望 ANNA 完成这些事情时，优先使用 `yutu-executa`：

- 查询频道、视频、播放列表、评论、订阅、字幕等 YouTube 资源
- 根据 handle、频道名、频道 ID、视频 ID 做查询与分析
- 获取频道统计，如粉丝数、视频数、总播放量
- 获取某个频道的最新视频、最热视频、某个视频的详细信息
- 创建、更新、删除 YouTube 资源
- 做竞品分析、频道分析、内容盘点

如果任务本质上是 YouTube Data API 操作，默认优先考虑 `run_yutu`。

## 工具边界

插件当前只暴露一个工具：

- `run_yutu`

它接收：

- `command`: 必填，`yutu` 子命令的字符串数组
- `cwd`: 可选，输出文件所在目录

ANNA 需要把命令拆成数组元素，而不是拼成一整行 shell 字符串。

正确示例：

```json
["channel", "list", "--forHandle", "@tech-shrimp"]
```

不要写成：

```json
["channel list --forHandle @tech-shrimp"]
```

## 必须遵守的限制

- 不要调用 `auth`、`agent`、`mcp`、`executa`
- 查询类任务优先使用 `list`
- 修改类任务优先执行“先查后改”
- 删除类任务优先执行“先查后删”
- 如果用户给的是 handle、标题、昵称等非稳定标识，先查出稳定 ID 再继续
- 如果命令涉及多个资源，优先沿着 `channelId -> videoId -> playlistId/commentId/subscriptionId` 的顺序推进

## 资源模型

理解下面这些对象关系，能显著减少误调用：

- `channel`
  - 表示 YouTube 频道
  - 稳定主键通常是 `channelId`，形如 `UC...`
  - `@handle` 只是方便用户输入，不是最稳定的内部主键
- `video`
  - 稳定主键是 `videoId`
- `playlist`
  - 稳定主键是 `playlistId`
- `playlistItem`
  - 表示“某个视频在某个播放列表中的一个条目”
  - 删除播放列表里的视频时，通常删的是 `playlistItemId`，不是 `videoId`
- `commentThread`
  - 顶层评论线程
- `comment`
  - 某条具体评论或回复
- `subscription`
  - 是“订阅关系对象”，删除订阅时要用 `subscriptionId`，不是 `channelId`

## 结果解析原则

优先从返回结果中提取稳定 ID 和关键统计字段，而不是依赖展示名。

常用字段：

- 频道
  - `id`: `channelId`
  - `snippet.title`: 频道名
  - `snippet.customUrl`: 通常对应 handle
  - `statistics.subscriberCount`: 粉丝数
  - `statistics.videoCount`: 视频数
  - `statistics.viewCount`: 总播放量
- 搜索结果
  - `id.videoId`: 视频 ID
  - `id.channelId`: 频道 ID
- 视频
  - `id`: `videoId`
  - `snippet.title`: 标题
  - `statistics.viewCount`: 播放量
  - `statistics.likeCount`: 点赞数
  - `commentCount`: 评论数

注意：

- 频道默认返回不一定带 `statistics`
- 如果要看粉丝数、视频数、总播放量，显式追加 `--parts id,snippet,statistics`
- 某些频道可能隐藏订阅数，此时可能没有 `subscriberCount`，或者 `hiddenSubscriberCount` 为 `true`

## 推荐思维方式

### 1. 先定位对象，再取详情

当用户说“查某个用户”时，不要把“用户”直接当主键。

推荐流程：

1. 用 `channel list --forHandle` 或 `channel list --forUsername` 找频道
2. 从返回结果中取 `channelId`
3. 再按 `channelId` 查视频、播放列表、订阅或频道统计

### 2. 先取轻量列表，再取重量详情

当用户要“最新视频”“最热视频”“最近 10 条内容”时：

1. 先用 `search list` 取列表
2. 再把得到的 `videoId` 批量交给 `video list`

这样可以减少不必要的详情读取。

### 3. 破坏性操作必须两步走

对于 `delete`、`unset`、`markAsSpam`、`setModerationStatus`、`rate` 这类会改状态的操作：

1. 先 `list` 核对目标对象
2. 再执行变更

除非用户已经明确要求执行，并且对象已被准确识别，否则不要直接删除。

## 高频任务套路

### 查询某个频道的信息

如果用户给的是 handle：

```json
["channel", "list", "--forHandle", "@tech-shrimp", "--parts", "id,snippet,statistics"]
```

如果用户给的是频道 ID：

```json
["channel", "list", "--ids", "UCa6D2k5qhpOI9I-WT8fpd6g", "--parts", "id,snippet,statistics"]
```

### 查询某个频道的最新视频

先找 `channelId`，再按频道搜索视频：

```json
["search", "list", "--channelId", "UCa6D2k5qhpOI9I-WT8fpd6g", "--types", "video", "--order", "date", "--maxResults", "5"]
```

如果还要详细统计，再继续：

```json
["video", "list", "--ids", "VIDEO_ID_1,VIDEO_ID_2,VIDEO_ID_3", "--parts", "id,snippet,status,statistics,contentDetails"]
```

### 查询某个频道的最热视频

```json
["search", "list", "--channelId", "CHANNEL_ID", "--types", "video", "--order", "viewCount", "--maxResults", "10"]
```

### 查询我自己的频道

```json
["channel", "list", "--mine", "--parts", "id,snippet,statistics,contentDetails"]
```

### 查询我自己的最新视频

```json
["search", "list", "--forMine", "--types", "video", "--order", "date", "--maxResults", "10"]
```

### 查询某个视频详情

```json
["video", "list", "--ids", "VIDEO_ID", "--parts", "id,snippet,status,statistics,contentDetails"]
```

### 查询某个视频的评论线程

```json
["commentThread", "list", "--videoId", "VIDEO_ID", "--maxResults", "20", "--order", "relevance"]
```

### 查询我的播放列表

```json
["playlist", "list", "--mine", "--maxResults", "20"]
```

### 查询我是否已经订阅某个频道

```json
["subscription", "list", "--mine", "--forChannelId", "CHANNEL_ID"]
```

## 常见用户意图到调用策略

- “看看这个博主的数据”
  - 先 `channel list --forHandle ... --parts id,snippet,statistics`
- “看看这个博主最新一期视频”
  - 先 `channel list --forHandle ...`
  - 再 `search list --channelId ... --types video --order date --maxResults 1`
- “分析这个频道最近 10 个视频表现”
  - 先 `channel list --forHandle ... --parts id,snippet,statistics`
  - 再 `search list --channelId ... --types video --order date --maxResults 10`
  - 再 `video list --ids ... --parts id,snippet,statistics,contentDetails`
- “我有没有订阅这个频道”
  - 先 `channel list --forHandle ...`
  - 再 `subscription list --mine --forChannelId ...`
- “把某个视频加进我的播放列表”
  - 先 `channel list --mine`
  - 再确认 `playlistId`
  - 再执行 `playlistItem insert`

## 何时显式指定 parts

默认返回字段不一定足够。以下场景建议显式传 `--parts`：

- 频道统计：`id,snippet,statistics`
- 频道深度信息：`id,snippet,statistics,contentDetails,status`
- 视频深度信息：`id,snippet,status,statistics,contentDetails`
- 只需要轻量列表：维持默认即可

原则：

- 只取本次任务需要的字段
- 如果用户要“统计”“分析”“表现”“粉丝数”“播放量”，通常需要 `statistics`

## 输出策略

- 如果只是给 ANNA 后续步骤用，优先提取关键字段，不必复述整份原始返回
- 如果用户要最终汇报，汇总成“频道信息 + 关键统计 + 最新视频/代表视频”
- 如果需要跨多步调用，先保存中间 ID，再继续调用下一步

## 风险与错误处理

- 如果按 handle 查不到频道，换成频道名关键词搜索不是首选；优先请用户确认 handle 是否准确
- 如果搜索结果为空，不要默认认为频道不存在，可能只是该过滤条件下没有结果
- 如果要删除对象但当前只有展示名，没有稳定 ID，先停止并补查
- 如果一个任务需要多个写操作，先把读取校验做完，再串行执行修改

## 详细工作流

下面这些场景的推荐调用顺序见：

- [references/workflows.md](references/workflows.md)


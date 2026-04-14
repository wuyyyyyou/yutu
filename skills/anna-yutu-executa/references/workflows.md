# yutu-executa 常见工作流

本文件给出更细的任务拆解方式，适合 ANNA 在多步任务里按顺序执行。

## 1. 查询某个频道的粉丝数、视频数、总播放量

适用场景：

- “看看这个博主的数据”
- “这个频道有多少粉丝”
- “这个频道一共发了多少视频”

推荐步骤：

1. 如果用户给的是 handle，先查频道
2. 显式请求 `statistics`
3. 读取并整理关键字段

命令数组：

```json
["channel", "list", "--forHandle", "@tech-shrimp", "--parts", "id,snippet,statistics"]
```

重点读取：

- `id`
- `snippet.title`
- `statistics.subscriberCount`
- `statistics.videoCount`
- `statistics.viewCount`
- `statistics.hiddenSubscriberCount`

## 2. 查询某个频道的最新视频

适用场景：

- “这个 UP 最近发了什么”
- “帮我找这个博主最新一期视频”

推荐步骤：

1. 先拿 `channelId`
2. 再用 `search list` 按 `channelId + order=date + types=video`
3. 如果需要更完整数据，再用 `video list`

命令数组：

```json
["channel", "list", "--forHandle", "@tech-shrimp"]
```

```json
["search", "list", "--channelId", "CHANNEL_ID", "--types", "video", "--order", "date", "--maxResults", "5"]
```

如果用户需要视频的播放量、点赞数、时长：

```json
["video", "list", "--ids", "VIDEO_ID_1,VIDEO_ID_2,VIDEO_ID_3", "--parts", "id,snippet,statistics,contentDetails,status"]
```

## 3. 查询某个频道的最热视频

适用场景：

- “看看这个频道最火的视频”
- “帮我分析这个频道爆款内容”

推荐步骤：

1. 先拿 `channelId`
2. 再按 `viewCount` 排序搜索视频
3. 如需统计，继续查 `video list`

命令数组：

```json
["search", "list", "--channelId", "CHANNEL_ID", "--types", "video", "--order", "viewCount", "--maxResults", "10"]
```

## 4. 分析一个频道最近 10 个视频的表现

适用场景：

- “分析这个频道最近内容表现”
- “看看最近 10 个视频哪些更受欢迎”

推荐步骤：

1. 查频道基础统计
2. 查最近 10 个视频
3. 批量取视频详情
4. 汇总以下维度：
   - 发布时间
   - 播放量
   - 点赞数
   - 评论数
   - 时长

命令数组：

```json
["channel", "list", "--forHandle", "@target", "--parts", "id,snippet,statistics"]
```

```json
["search", "list", "--channelId", "CHANNEL_ID", "--types", "video", "--order", "date", "--maxResults", "10"]
```

```json
["video", "list", "--ids", "VIDEO_ID_1,VIDEO_ID_2,VIDEO_ID_3", "--parts", "id,snippet,statistics,contentDetails,status"]
```

## 5. 查询我自己的频道和内容

适用场景：

- “看看我的频道”
- “看看我最近发的视频”
- “分析我最近内容表现”

命令数组：

```json
["channel", "list", "--mine", "--parts", "id,snippet,statistics,contentDetails"]
```

```json
["search", "list", "--forMine", "--types", "video", "--order", "date", "--maxResults", "10"]
```

```json
["video", "list", "--ids", "VIDEO_IDS", "--parts", "id,snippet,statistics,contentDetails,status"]
```

## 6. 查询播放列表并向其中添加视频

适用场景：

- “把这个视频加到我的某个播放列表”
- “新建一个播放列表并加几个视频进去”

推荐步骤：

1. 先查自己的频道
2. 查现有播放列表，或创建新播放列表
3. 再执行 `playlistItem insert`

命令数组：

```json
["channel", "list", "--mine"]
```

```json
["playlist", "list", "--mine", "--maxResults", "20"]
```

创建播放列表：

```json
["playlist", "insert", "--title", "My Playlist", "--channelId", "CHANNEL_ID", "--privacy", "public"]
```

向播放列表加视频：

```json
["playlistItem", "insert", "--kind", "video", "--playlistId", "PLAYLIST_ID", "--channelId", "CHANNEL_ID", "--kVideoId", "VIDEO_ID"]
```

## 7. 查询评论并回复

适用场景：

- “看看某个视频评论区”
- “替我回复这条评论”

推荐步骤：

1. 先列评论线程
2. 确认 `threadId` 或相关评论对象
3. 再执行回复

命令数组：

```json
["commentThread", "list", "--videoId", "VIDEO_ID", "--maxResults", "20", "--order", "time"]
```

回复评论：

```json
["comment", "insert", "--channelId", "MY_CHANNEL_ID", "--videoId", "VIDEO_ID", "--authorChannelId", "MY_CHANNEL_ID", "--parentId", "THREAD_ID", "--textOriginal", "谢谢反馈"]
```

## 8. 查询是否已订阅某个频道，并执行订阅或取消订阅

适用场景：

- “我订阅这个频道了吗”
- “帮我订阅/取消订阅这个频道”

推荐步骤：

1. 先通过 handle 找到目标频道
2. 查订阅关系对象
3. 不要直接用 `channelId` 删除订阅，必须先拿到 `subscriptionId`

命令数组：

```json
["channel", "list", "--forHandle", "@target"]
```

```json
["subscription", "list", "--mine", "--forChannelId", "TARGET_CHANNEL_ID"]
```

订阅：

```json
["subscription", "insert", "--subscriberChannelId", "MY_CHANNEL_ID", "--channelId", "TARGET_CHANNEL_ID"]
```

取消订阅：

```json
["subscription", "delete", "--ids", "SUBSCRIPTION_ID"]
```

## 9. 删除前校验模式

适用场景：

- 删除视频
- 删除评论
- 删除播放列表
- 删除播放列表项
- 删除字幕

统一模式：

1. 先 `list`
2. 确认主键和标题
3. 再 `delete`

示例：删除视频

```json
["video", "list", "--ids", "VIDEO_ID"]
```

```json
["video", "delete", "--ids", "VIDEO_ID"]
```

示例：删除播放列表项

```json
["playlistItem", "list", "--playlistId", "PLAYLIST_ID"]
```

```json
["playlistItem", "delete", "--ids", "PLAYLIST_ITEM_ID"]
```

## 10. 常见误区

- 把频道展示名当成稳定 ID
- 直接根据视频标题做删除
- 用 `videoId` 删除播放列表项
- 用 `channelId` 直接删除订阅
- 想看粉丝数却忘了追加 `statistics`
- 想查“某个人的最新视频”却跳过了 `channelId` 这一步

# yutu Executa 插件说明与测试命令

本文档使用中文说明 `yutu` 的 Executa 插件形态，并给出可以在本地 `fish` 里直接执行的测试命令。

下面默认你本地已经安装了：

- `fish`
- `jq`
- `python3`
- `go`

## 1. 插件入口

`yutu` 作为 Executa 插件时，入口命令是：

```bash
yutu executa
```

如果你要构建 Anna Binary 分发用的多平台包，可以执行：

```bash
./scripts/build-executa.sh --package
```

如果只是本机调试当前平台二进制，可以执行：

```bash
./scripts/build-executa.sh
```

## 2. 工具能力

插件目前只暴露一个工具：

- `run_yutu`

它的行为是：

- 输入参数里的 `command` 必须是字符串数组
- 可选传入 `cwd`
- 会阻止执行 `auth`、`agent`、`mcp`、`executa`
- 如果目标命令支持 `--output`，且调用时没有显式传 `--output`，插件会自动补 `--output json`
- `run_yutu` 的 `invoke` 响应始终使用标准 Executa `__file_transport`
- 传输文件放在 `cwd` 下，文件内容是完整的 JSON-RPC 响应，不是自定义路径字段

## 3. 凭据输入

插件支持两个可选的 Executa credential：

- `GOOGLE_ACCESS_TOKEN`
- `YUTU_AUTHORIZED_USER_FILE`

优先级如下：

1. 如果提供了 `GOOGLE_ACCESS_TOKEN`，优先使用它，并将其转换成 `YUTU_CACHE_TOKEN`
2. 如果没有 `GOOGLE_ACCESS_TOKEN`，再读取 `YUTU_AUTHORIZED_USER_FILE`

它应当指向你本地一个 Google `authorized_user` JSON 文件，格式类似：

```json
{
  "client_id": "xxxxx",
  "client_secret": "xxxxx",
  "refresh_token": "xxxxx",
  "type": "authorized_user"
}
```

`GOOGLE_ACCESS_TOKEN` 适合你已经拿到可直接使用的 OAuth access token 的场景。

`YUTU_AUTHORIZED_USER_FILE` 适合你只有 Google `authorized_user` JSON 的场景。运行时，插件会把这个文件内容转换成 `YUTU_CREDENTIAL` 和 `YUTU_CACHE_TOKEN` 环境变量，再去调用 `yutu` CLI。

## 4. Tool 输入格式

`run_yutu` 的调用格式如下：

```json
{
  "tool": "run_yutu",
  "arguments": {
    "command": ["search", "list", "--q", "golang"],
    "cwd": "/tmp/yutu-output"
  }
}
```

如果不传 `cwd`，默认使用插件二进制所在目录。

## 5. fish 本地测试命令

下面这些命令按 `fish` 语法编写，可以直接在你的本地 shell 里使用。

### 5.1 构建当前平台二进制

```fish
./scripts/build-executa.sh
```

### 5.2 初始化测试环境

把下面的占位路径替换成你自己的 `authorized_user` 文件路径。

```fish
set -x BINARY ./dist/yutu-executa
set -x YUTU_CWD /tmp/yutu-plugin-test
set -x YUTU_AUTH_FILE /absolute/path/to/authorized_user.json
mkdir -p $YUTU_CWD
```

如果你想临时生成一个测试文件，也可以这样：

```fish
set -x BINARY ./dist/yutu-executa
set -x YUTU_CWD /tmp/yutu-plugin-test
set -x YUTU_AUTH_FILE $YUTU_CWD/authorized_user.json
mkdir -p $YUTU_CWD

jq -n \
  --arg client_id YOUR_GOOGLE_CLIENT_ID \
  --arg client_secret YOUR_GOOGLE_CLIENT_SECRET \
  --arg refresh_token YOUR_GOOGLE_REFRESH_TOKEN \
  '{
    "client_id": $client_id,
    "client_secret": $client_secret,
    "refresh_token": $refresh_token,
    "type": "authorized_user"
  }' > $YUTU_AUTH_FILE
```

### 5.3 定义辅助函数

执行一次即可，后续当前 shell 中可以直接复用。

```fish
function ydescribe
    printf '%s\n' '{"jsonrpc":"2.0","method":"describe","id":1}' | $BINARY | jq .
end

function yhealth
    printf '%s\n' '{"jsonrpc":"2.0","method":"health","id":2}' | $BINARY | jq .
end

function ycat
    set -l file $argv[1]
    if test -z "$file"
        echo "missing file path"
        return 1
    end

    if jq -e . $file >/dev/null 2>/dev/null
        cat $file | jq .
    else
        cat $file
    end
end

function yresp_file
    set -l command_json (printf '%s\n' $argv | jq -R . | jq -s .)
    set -l req (jq -nc \
        --argjson command "$command_json" \
        --arg cwd "$YUTU_CWD" \
        --arg auth_file "$YUTU_AUTH_FILE" \
        '{jsonrpc:"2.0",method:"invoke",params:{tool:"run_yutu",arguments:{command:$command,cwd:$cwd},context:{credentials:{YUTU_AUTHORIZED_USER_FILE:$auth_file}}},id:3}')

    set -l resp (printf '%s\n' $req | $BINARY)
    printf '%s\n' $resp | jq -r '.__file_transport'
end

function yrun_meta
    ycat (yresp_file $argv)
end

function yrun_file
    yresp_file $argv
end

function yrun
    set -l resp_file (yresp_file $argv)
    set -l has_error (cat $resp_file | jq -r 'has("error")')

    if test "$has_error" = "true"
        cat $resp_file | jq .
        return 1
    end

    set -l output_type (cat $resp_file | jq -r 'if .result.data.output == null then "null" elif (.result.data.output | type) == "string" then "string" else "json" end')

    if test "$output_type" = "json"
        cat $resp_file | jq '.result.data.output'
    else if test "$output_type" = "string"
        cat $resp_file | jq -r '.result.data.output'
    end
end

function ylast
    set -l latest (ls -t $YUTU_CWD/executa-resp-*.json 2>/dev/null | head -n 1)
    if test -n "$latest"
        ycat $latest
    else
        echo "No output file found in $YUTU_CWD"
    end
end
```

## 6. 最基础的测试

### 查看插件描述

```fish
ydescribe
```

### 查看健康状态

```fish
yhealth
```

### 验证插件能调起 yutu

```fish
yrun_meta version
yrun version
```

### 查看版本命令生成的原始输出文件

```fish
yrun_file version
cat (yrun_file version)
```

### 测试一个需要认证的命令

例如搜索视频：

```fish
yrun_meta search list --q golang --maxResults 3
yrun search list --q golang --maxResults 3
```

例如查看频道：

```fish
yrun_meta channel list --mine
yrun channel list --mine
```

## 7. 查看最近一次输出

```fish
ylast
```

## 8. 常见调用示例

### 搜索视频

```fish
yrun_meta search list --q 'golang tutorial' --maxResults 5
yrun search list --q 'golang tutorial' --maxResults 5
yrun_meta search list --q 'music' --types video --videoDuration medium --maxResults 5
yrun search list --q 'music' --types video --videoDuration medium --maxResults 5
```

### 查看自己的频道

```fish
yrun_meta channel list --mine
yrun channel list --mine
```

### 查看评论线程

```fish
yrun_meta commentThread list --videoId YOUR_VIDEO_ID --maxResults 5
yrun commentThread list --videoId YOUR_VIDEO_ID --maxResults 5
```

### 查看播放列表

```fish
yrun_meta playlist list --mine --maxResults 5
yrun playlist list --mine --maxResults 5
```

## 9. 验证禁止调用的命令

下面这些调用应该返回错误：

```fish
yrun auth
yrun mcp
yrun agent
yrun executa
```

## 10. 清理测试目录

```fish
rm -rf $YUTU_CWD
```

## 11. 直接查看二进制最原始响应

如果你想确认插件是否真的走了标准 Executa `__file_transport`，不要调用 `yresp_file` 或 `yrun_meta`，而是直接把 JSON-RPC 请求喂给二进制，看它的 stdout 原文。

### 查看 `version` 的最原始响应

```fish
set -l req (jq -nc \
  --arg cwd "$YUTU_CWD" \
  --arg auth_file "$YUTU_AUTH_FILE" \
  '{jsonrpc:"2.0",method:"invoke",params:{tool:"run_yutu",arguments:{command:["version"],cwd:$cwd},context:{credentials:{YUTU_AUTHORIZED_USER_FILE:$auth_file}}},id:1}')

printf '%s\n' $req | $BINARY
```

如果实现正确，这里应该直接返回类似下面这样的单行 JSON：

```json
{"jsonrpc":"2.0","id":1,"__file_transport":"/tmp/.../executa-resp-xxxx.json"}
```

### 查看 `search list` 的最原始响应

```fish
set -l req (jq -nc \
  --arg cwd "$YUTU_CWD" \
  --arg auth_file "$YUTU_AUTH_FILE" \
  '{jsonrpc:"2.0",method:"invoke",params:{tool:"run_yutu",arguments:{command:["search","list","--q","golang","--maxResults","3"],cwd:$cwd},context:{credentials:{YUTU_AUTHORIZED_USER_FILE:$auth_file}}},id:2}')

printf '%s\n' $req | $BINARY
```

### 查看 `__file_transport` 指向的完整响应文件

在确认 stdout 里确实返回了 `__file_transport` 之后，再执行：

```fish
cat (printf '%s\n' $req | $BINARY | jq -r '.__file_transport')
```

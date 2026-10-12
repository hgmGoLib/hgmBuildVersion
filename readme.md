# hgmBuildVersion — 二进制版本号: 生成 / 校验 / 比大小

```go
import "github.com/hgmGoLib/hgmBuildVersion"                        // 生成 / 校验 / 比大小
import "github.com/hgmGoLib/hgmBuildVersion/hgmBuildVersionCurrent" // 把版本号烤进二进制, 运行期读出来
```

只依赖 golang 标准库, 最低 Go 1.13 (生成版本号时需要本机有 `git` 命令)。许可证 MIT-0 (MIT No Attribution: 随便用, 不需要署名)。

## English summary

A tiny, dependency-free Go library for build version strings like

```
v0.1.260909155041-a1b2c3d4e5-r
```

= `<prefix from versionPrefix.txt>.<build time YYMMDDhhmmss>-<first 10 hex of git HEAD>-<r=clean | d=dirty>`.

Versions for built binaries: every build gets a new version automatically, with no human step and no change to the git repo (no "bump version file, commit, build again" round trip); nothing but the git repo and the build machine is involved, no central version service. Order = prefix (edited by hand) + build time. The commit id lets you find the exact source of a running binary.

- `NewBuildVersion(versionPrefixPath) (string, error)` — generate the version for one build (run once, share it across all binaries of that build). Needs `git`.
- `GetCommitId(ver) (string, bool)` — the 10-hex commit id inside a version, only for finding the source. Never use it in any decision.
- `GetBuildTime(ver) (time.Time, bool)` — the build time inside a version, read with `GBuildTimeZoneOffsetSecond`. For display / records only.
- `AddTimeSecond(ver, second int64) (string, error)` — shift the timestamp by `second` (may be negative) with real clock carry; prefix, commit id and flag unchanged. For upgrade tests that build several packages within the same second: shift the version you just built so it is newer (or older) than the other one. Normal builds do not need it.
- `GBuildTimeZoneOffsetSecond` — fixed UTC offset (seconds) for the timestamp, default `8*3600` (UTC+8). Pick it once per project and never change it: the zone is not part of the version string, so changing it breaks the order.
- `IsValid(ver) bool` — strict whole-string format check. A valid version contains only `[0-9a-zA-Z.-]` and can never contain `/` or `..`, so it is safe as a file/directory name after this check.
- `IsUpgrade(cur, target) bool` — true only if `target` is strictly newer. Ordering uses only the prefix and timestamp; commit id and dirty flag are identity, not order. Invalid input (including `""`) → `false`.
- `CompareOrder(a, b) (cmp int, ok bool)` — three-way order compare (-1 / 0 / 1) for sorting, max, `>=`. Invalid input → `ok=false`; always check `ok` first.
- Sub package `hgmBuildVersionCurrent`: `go build -ldflags "-X "+GetVersionVarFullName()+"="+ver`, then `GetCurrentVersion()` at runtime.
- `ts/hgmBuildVersion.ts` — TypeScript `IsValid` / `IsUpgrade` / `CompareOrder` for front ends, checked against the same `testdata/cases.json` as the Go code.

Treat a version as an opaque string: pass it around, compare with `==`, hand it to this package.

## 这个库是干什么的

给"打包出来的一批二进制"一个统一的版本号, 并回答两个问题:

1. **这个字符串是不是合法的版本号?** —— 合法的版本号可以直接当文件名 / 目录名片段用, 不会路径穿越。
2. **target 相对 cur 是不是一次升级?** —— 严格更新才算, 同版本 / 降级 / 看不懂 一律不算。

需求约束 (选不选本库先看这里):

- 本库给"打包出来的二进制"编版本号。每次打包自动生成新版本号, 不需要人介入, 不改动 git 内容; 除了 git 仓库和打包机本地以外, 没有任何中心服务管理版本号。
- 不会出现"先用上一个版本号测试, 再改 git 里的版本号文件、commit、重新打包"这种二次 commit。
- 新旧由版本号前缀 (人手改) 和打包时刻决定。

典型用途:

- **拿着版本号找到编译时的源代码**: 版本号里带着打包时 git HEAD 的 commitId, 线上跑着的二进制报出版本号, `git checkout <commitId>` 就是它的代码。末尾是 `-d` 的包打包时有未提交的修改, 只能找到最接近的那个 commit; commit 没推送到共享仓库时别人也找不到。
- 程序自更新时判断 "要不要换成下载到的那个版本"。
- 按版本号建目录存放各版本的二进制。
- 日志 / 接口里上报自己的版本。

版本号形如:

```
v0.1.260909155041-a1b2c3d4e5-r
```

| 段 | 含义 |
|---|---|
| `v0.1` | 前缀, 来自仓库里的 `versionPrefix.txt` (人手改, 大版本才动) |
| `260909155041` | 打包时间 YYMMDDhhmmss (定长 12 位; 时区由 `GBuildTimeZoneOffsetSecond` 定, 默认 UTC+8) |
| `a1b2c3d4e5` | 打包时 git HEAD 的 commitId 前 10 位 |
| `r` / `d` | 打包时工作区干净 / 有未提交修改 (未被 .gitignore 的未跟踪文件也算) |

- **比大小只看前三段** (`v0.1` 和时间戳)。
- **判断是不是同一个包用整串 `==`**: 同一秒打的两个不同 commit 的包, 序相同但整串不同。
- 版本号就是 `string`。调用方只能原样传递、判等 `==`、交给本包的函数; 不要自己拆它、改它。
  能从里面取的只有 commitId (`GetCommitId`, 只许拿去找源码) 和打包时刻 (`GetBuildTime`, 只给人看 / 存档); 能改的只有时间戳 (`AddTimeSecond`)。

## 怎么用

### 1. 放一个版本号前缀文件

在仓库里放 `versionPrefix.txt`, 内容形如 `v0.1` (两段十进制, 不许前导零)。大版本时人手改。

### 2. 打包侧: 生成版本号并注入二进制

一次打包只生成一次, 所有二进制共用同一个版本号:

```go
hgmBuildVersion.GBuildTimeZoneOffsetSecond = 0 // 可选: 时间戳改用 UTC; 不设 = UTC+8
ver, err := hgmBuildVersion.NewBuildVersion("versionPrefix.txt") // 参数是该文件路径; git 在该文件所在目录里跑
if err != nil {
	panic(err)
}
cmd := exec.Command("go", "build",
	"-ldflags", "-X "+hgmBuildVersionCurrent.GetVersionVarFullName()+"="+ver,
	"-o", "bin/xxx", "./xxx")
```

工作区有未提交修改时照样能打, 只是末尾变成 `-d`。`versionPrefix.txt` 写错时 `NewBuildVersion` 直接返回 error。
没有 git (例如从源码压缩包构建) 或该文件没被 git 跟踪 (没 `git add` / 被 .gitignore 掉) 时也直接返回 error。

**时区一个项目定一次, 之后不要改**: 时区不写进版本号, 而时间戳决定新旧; 比如从 UTC+8 改成 UTC, 改完后 8 小时内打的包比改之前的"旧", 不算升级。非改不可时同时升 `versionPrefix.txt` 的版本号顶过去。

也可以不用 `-X`: 在程序开始的位置直接赋值 `hgmBuildVersionCurrent.GVersion = ver` (例如由生成代码写进 main 包)。

### 3. 运行期: 读出自己的版本

被编的 main 只要 (间接) import 了 `hgmBuildVersionCurrent` 就生效:

```go
cur := hgmBuildVersionCurrent.GetCurrentVersion() // 原样返回注入的串; 没注入 -> ""
```

### 4. 校验 / 比大小

```go
hgmBuildVersion.IsValid(ver) bool                  // 空串 -> false
hgmBuildVersion.IsUpgrade(cur, target) bool        // target 严格更新才 true; 任一非法 (含空串) -> false
hgmBuildVersion.CompareOrder(a, b) (cmp int, ok bool) // a 更旧 -1 / 序相等 0 / a 更新 1; 任一非法 -> (0, false)
```

- 从外部收进来的版本号, **当路径片段 / 拼进脚本之前必须先过 `IsValid`**。
- 版本号字符集只有 `[0-9a-zA-Z.-]`, 但拼 URL / 拼 shell 时仍然要照常转义。
- 排序 / 取较新的 / 判 "不比它旧" 用 `CompareOrder`, **先看 ok 再看 cmp**。
  "b 不比 a 旧" 要写 `cmp, ok := CompareOrder(b, a); ok && cmp >= 0`, 不要写 `!IsUpgrade(b, a)`: 那样非法版本号也会被当成"不比它旧"放过去。
- 序相等 (`cmp == 0`) 不等于同一个包, 判同一个包用整串 `==`。

### 5. 找源码

```go
commitId, ok := hgmBuildVersion.GetCommitId(ver) // 10 位小写 hex; 非法版本号 -> ("", false)
```

拿去 `git checkout` / `git archive` 找到这个版本的源代码。**只许拿去找源码**, 不许参与任何判断 (比新旧用 `IsUpgrade`, 判是不是同一个包用整串 `==`)。

### 6. 打包时刻

```go
tm, ok := hgmBuildVersion.GetBuildTime(ver) // 时区 = GBuildTimeZoneOffsetSecond; 非法版本号 / 时间戳不是真实时刻 -> (time.Time{}, false)
```

给人看 (例 页面上显示构建时间)、存档用。不要另外再注入一个构建时间变量 (两个来源迟早对不上)。

- 时区不写进版本号: **调它的进程里 `GBuildTimeZoneOffsetSecond` 必须和打包时一样** (常常是运行期的二进制在调, 不是打包程序), 否则差几个小时。最省事是全程用默认值不改。
- 被 `AddTimeSecond` 挪过的版本号, 读出来的是挪过之后的时刻, 和真实打包时刻差几秒。
- 比新旧用 `IsUpgrade` / `CompareOrder`, 不要比这个时间。

### 7. 升级测试里拉开序: `AddTimeSecond`

升级的自动测试会在几秒内连着打好几个包, 常常落在同一秒: 序相等, 不算升级。这时把本次打出来的版本号挪一挪:

```go
next, err := hgmBuildVersion.NewBuildVersion("versionPrefix.txt")
if !hgmBuildVersion.IsUpgrade(base, next) {
	next, err = hgmBuildVersion.AddTimeSecond(next, 1) // next 在 base 之后打, 时间戳不会比 base 小, 加 1 秒就够
}
```

- 用在: 同一份代码的升级测试 (base -> next)、从 git 打旧版本再升到新版本的测试、拒绝降级的测试 (往前挪, 造一个严格更旧的包)。具体见 [doc/usage.md](doc/usage.md) 第四节。
- **正式打包不用它**: 正常打包间隔远大于 1 秒, 靠当前时间就够。
- 按真实时间进位: `…153059` 加 1 秒是 `…153100`, 不是 `…153060`。秒数可正可负。
- 前缀 / commitId / 脏否原样不动。**挪本次打出来的版本号, 不要挪别的包的**: 挪别的包的会让新包顶着别人的 commitId, 拿着它找到的是别的代码。
- 结果超出 2000..2099 年 / 原时间戳不是真实时刻 / 版本号非法 时返回 error。

### 8. 前端 (TypeScript)

`ts/hgmBuildVersion.ts`: `IsValid` / `IsUpgrade` / `CompareOrder`, 规则和 Go 版一样, 不依赖任何包, 直接复制进前端工程用。

- `testdata/cases.json` 是 Go 和 TS 共用的用例; `go test` 会顺带用 node 跑 `ts/hgmBuildVersion_test.ts` (本机没有能直接跑 .ts 的 node 时跳过)。
- 改规则 = 改 `cases.json` + 两边实现, 不许只改一边。
- 复制进前端的那份不要手改。下游最好在自己的代码生成步骤里从本 module 的目录 (`go list -m -f '{{.Dir}}' github.com/hgmGoLib/hgmBuildVersion`) 复制过去, 不要提交一份手抄的。

实战用法 (自更新怎么写 / 版本号显示在哪 / 升级的真机自动测试 / GitHub 发布) 见 [doc/usage.md](doc/usage.md)。
设计取舍见 [doc/design.md](doc/design.md) 和 [doc/binaryAndLibVersion.md](doc/binaryAndLibVersion.md)。
改版本号格式 / 从别的版本号格式迁过来 (已部署的旧二进制怎么升上来) 见 [doc/formatMigration.md](doc/formatMigration.md)。

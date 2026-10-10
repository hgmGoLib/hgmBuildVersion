# hgmBuildVersion 设计说明

用法见 `../readme.md`。这里记格式细节和设计取舍。

包名前缀 `hgm` 是作者所有库共用的前缀: 避免和 `version` 这类常见单词包名撞车, 也方便 grep。

## 版本号形态

```
v0.1.260909155041-8d8358905d.r
```

| 段 | 含义 |
|---|---|
| `v0.1` | versionPrefix.txt 的内容 (人手改, 两段十进制, 不许前导零) |
| `260909155041` | 打包时的时间 YYMMDDhhmmss (定长 12 位; 时区 = GBuildTimeZoneOffsetSecond, 默认 UTC+8) |
| `8d8358905d` | 打包时 git HEAD 的 commitId 前 10 位 (小写 hex) |
| `r` / `d` | 打包时工作区干净 (release) / 有未提交修改 (dirty) |

格式整串锁死: `v<数字>.<数字>.<12位数字>-<10位小写hex>.<r|d>`。

## 序 与 身份

- **序** = 前三段, 三段 uint64 从前往后比。IsUpgrade 只比它。
- **身份** = 整串, 用 `==` 判等。
- commitId 和脏否是身份/溯源信息, 不是序信息: 谁新谁旧由打包时间决定, 和哪个 commit 无关。
- "序相等" ≠ "身份相同": 同一秒打的两个不同 commit 的包, 序相同而整串不同, 互相都不算升级。
- 判断 "跑着的是不是这次编出来的那个二进制" 要用整串判等, 不能用 IsUpgrade。

## 不透明字符串

版本号就是 string, 没有专门的类型。本包不提供拆段 / 取脏否 / 取时间戳的函数: 那几段只给人眼看, 调用方不许依赖其内部结构。

唯一的例外是 GetCommitId: "拿着版本号找到源代码"是本库的用途之一, 自动化地找源码 (例 升级测试里重编线上那个旧版本) 需要它, 不提供的话调用方只能自己拆串或者另抄一份 commitId 常量。
它取出来的 commitId 只许拿去找源码, 不许参与任何判断 —— 不透明规则防的是"拆出子串去比新旧 / 判身份", 找源码不在其列。

## 为什么升级必须严格更大

同版本或更低版本一律不升:

- 同版本不升: 否则每次检查更新都判 "需升级", 无限自更新。
- 更低版本不升: 防误操作 / 回滚攻击导致降级。降级应走人工卸载重装, 不走自更新通道。
- 任一边格式非法 (含空串) -> 不升: 看不懂的时候不动比乱动安全。

## 校验与路径安全

版本号可能从外部收进来并被当作目录名 / 文件名使用。格式长度和字符集全定死之后, `..` 和 `/` 在语法上不可能出现, 路径穿越由格式本身关掉, 不靠调用方记得过滤。所以当路径片段前必须先过 IsValid。

- 前两段不许前导零: 否则 `v0.1.x` 和 `v0.01.x` 是两个字符串却是同一个版本。
- 第三段定长, 不存在前导零歧义, 不查前导零。
- commitId 只认小写 hex: 免得同一个包两种写法。
- 数字自己解析而不用 strconv.ParseUint: 顺便挡掉 `+1` / `-1` / ` 1` / `0x10` 这类花样, 并检查 uint64 溢出。

## 字符集

刻意只用 `[0-9a-zA-Z.-]` (全是 RFC 3986 unreserved character): 版本号会进 URL、shell 脚本、文件名、目录名。
**任何时候改格式都不许引入 `+ / % & ? #` 这类需要转义的字符。**
即便如此, 拼 URL / 拼 bash 仍然必须照常转义: 字符集安全是最后一道保险, 不是免转义的许可证。

## 生成侧

- NewReleaseVersion = NewBuildVersion + 两道闸 (工作区干净 / HEAD 已被某个远端跟踪分支包含): 正式发布的包必须保证拿着版本号找得到源码, 这两条判断放进库里, 发布脚本不用再自己跑一遍 git status。
  "已推送"看的是本地的远端跟踪分支 (`git branch -r --contains`), 不联网: 打包不应该依赖网络, 而 git push 之后它就是准的。
- NewBuildVersion / NewReleaseVersion 是唯一生成版本号的地方。同一次打包的所有二进制必须共用同一个版本号: 调用方只调一次, 结果传给每个 go build。
- 不假设当前目录: git 在 versionPrefix.txt 所在目录里跑, 取的是这个文件所在仓库的 HEAD / 脏否。
- 工作区脏的时候照样打, 只是末尾变成 `.d`: 开发期工作区基本总是脏的, 在这里拦等于没法测试。`.d` 只供事后溯源, 不参与比大小。
- git 是硬依赖: 没有 git / 不在 git 仓库里直接返回 error, 不猜一个 commitId。
- 脏否用 `git status --porcelain` 判: 没被 .gitignore 掉的未跟踪文件也算脏。
- 时区 = 全局变量 GBuildTimeZoneOffsetSecond (相对 UTC 的固定偏移秒数, 默认 8*3600 = UTC+8), 打包程序在调 NewBuildVersion 前设一次。
  - 只给固定偏移, 不接受地名时区 (*time.Location): 带夏令时的时区在回拨那一小时里时间戳会倒退, 后打的包不算升级; 固定偏移也不依赖打包机的 tzdata、不受打包机本地时区影响。
  - 时区不写进版本号, 而时间戳决定序: 一个项目定下来之后不要再改。非改不可时 (例 UTC+8 -> UTC, 改完后 8 小时内打的包会比之前的"旧") 必须同时升 versionPrefix.txt 的版本号顶过去。
  - 用全局变量而不是加参数: 生成版本号只发生在打包程序里, 一次打包只调一次, 全局设一次就够; 也不破坏 NewBuildVersion 的签名。
- 序完全依赖打包机时钟: 打包机时钟回拨 / 多台打包机时钟不一致时, 后打的包可能不算升级。需要可靠的序就固定一台对过时的打包机。
- 时间戳只有两位年份: 2099 年之后会回绕成更小的数, 到时候得改 versionPrefix.txt 的大版本号顶过去。
- 拼完立刻 IsValid: versionPrefix.txt 写错 (段数不对 / 带字母) 在打包时就报错, 而不是等到部署后升不了级。

## 已知限制

以下几点目前没有在代码里处理, 使用时注意:

- **继承的 git 环境变量会让它看错仓库**: git 子进程继承打包程序的环境变量。打包程序在 git hook 里跑, 或者环境里设了 `GIT_DIR` / `GIT_WORK_TREE` / `GIT_INDEX_FILE` 时,
  git 不看 versionPrefix.txt 所在目录 (cmd.Dir), 取到的是这些变量指向的那个仓库的 HEAD / 脏否。在这种环境里调用前, 先清掉这几个变量。
- **脏否受 git 配置影响**: 脏否用的是不带额外参数的 `git status --porcelain`, 会读用户 / 仓库的 git 配置。
  配了 `status.showUntrackedFiles=no` 时未跟踪文件不算脏; 子模块配了 `ignore=all` (或 `diff.ignoreSubmodules`) 时子模块里的修改不算脏。
  这时 NewBuildVersion 会把脏工作区标成 `.r`, NewReleaseVersion 的"工作区干净"检查也会漏掉。打包机上不要改这几项配置。
- **NewReleaseVersion 不检查子模块有没有推送**: "已推送"只查顶层仓库的 HEAD。子模块里钉的 commit 没推送时照样能出 `.r` 版本号,
  别人拿着版本号能找到顶层代码, 但找不到那个子模块 commit。有子模块的项目, 发布前自己确认子模块都推送了。
- **IsValid 不检查时间戳是不是真实日期**: 第三段只要求 12 位数字, `v0.1.999999999999-…` 也是合法版本号。
  这种串一旦被当成"最新版本"下发过一次 (手误或被篡改), 同一前缀下之后正常打的包都比它"旧", 再也不算升级, 只能升 versionPrefix.txt 的版本号顶过去。
  版本号是不透明串, 本库只锁格式不管来源: 对外部收进来的版本号要先确认来源可信 (例 包本身有 MAC 校验) 再拿去比新旧。

## 子包 hgmBuildVersionCurrent

- 只有一个变量 `GVersion`, 可用 `-ldflags -X` 注入, 或程序开始处直接赋值。
- `GetVersionVarFullName()` 返回 `<本子包 import path>.GVersion`, import path 运行期用 reflect 从本包里的类型取, 不写死。
- `-X` 目标写错时 go build 静默忽略, 编出来就是空串; 所以不写死 import path (fork / 换 module path 照样对), current_test.go 用真编真跑钉住。
- current_test.go 把小程序放在系统临时目录, 自带 go.mod 用 replace 指回本 module: 不往 module 目录里写 (从只读的 module cache 里跑 go test 也行), 测试里也不写死 import path。

## 兼容的 Go 版本

go.mod 写 `go 1.13` (`%w` 错误包装从 1.13 起)。为此代码 / 测试刻意不用更新的东西: 不用泛型、strings.Cut / CutPrefix、os.ReadFile、t.TempDir (用 strings.IndexByte、ioutil.ReadFile、ioutil.TempDir 代替)。
改代码时不要引入比 1.13 新的标准库 API; 已用真 go1.13.15 跑过 go vet + go test。

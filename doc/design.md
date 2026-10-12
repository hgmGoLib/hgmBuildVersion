# hgmBuildVersion 设计说明

用法见 `../readme.md`。这里记格式细节和设计取舍。

包名前缀 `hgm` 是作者所有库共用的前缀: 避免和 `version` 这类常见单词包名撞车, 也方便 grep。

## 版本号形态

```
v0.1.260909155041-a1b2c3d4e5-r
```

| 段 | 含义 |
|---|---|
| `v0.1` | versionPrefix.txt 的内容 (人手改, 两段十进制, 不许前导零) |
| `260909155041` | 打包时的时间 YYMMDDhhmmss (定长 12 位; 时区 = GBuildTimeZoneOffsetSecond, 默认 UTC+8) |
| `a1b2c3d4e5` | 打包时 git HEAD 的 commitId 前 10 位 (小写 hex) |
| `r` / `d` | 打包时工作区干净 (release) / 有未提交修改 (dirty) |

格式整串锁死: `v<数字>.<数字>.<12位数字>-<10位小写hex>-<r|d>`。

commitId 和脏否之间用 `-` 不用 `.`: semver 预发布段按 `.` 切标识符, 全数字标识符不许前导零。
用 `-` 连成一个标识符后它永远不是数字型, commitId 碰巧全是数字且以 0 开头时也是合法 semver (早期用 `.` 时这约千分之一的串不合法)。

改格式 / 已部署的旧二进制怎么升上来: 见 [formatMigration.md](formatMigration.md)。本库只认当前格式。

## 序 与 身份

- **序** = 前三段, 三段 uint64 从前往后比。IsUpgrade / CompareOrder 只比它。
- **身份** = 整串, 用 `==` 判等。
- commitId 和脏否是身份/溯源信息, 不是序信息: 谁新谁旧由打包时间决定, 和哪个 commit 无关。
- "序相等" ≠ "身份相同": 同一秒打的两个不同 commit 的包, 序相同而整串不同, 互相都不算升级。
- 判断 "跑着的是不是这次编出来的那个二进制" 要用整串判等, 不能用 IsUpgrade。

## 不透明字符串

版本号就是 string, 没有专门的类型。调用方不许依赖其内部结构 (拆段 / 比子串 / 改数字), 要什么由本包的函数给。本包不提供取脏否 / 取前缀的函数。
不透明规则防的是"调用方拆出子串自己去比新旧 / 判身份 / 改数字, 各写各的, 规则散落一地"; 下面几个函数把这些需求收回本包, 规则只有一份。

- GetCommitId: "拿着版本号找到源代码"是本库的用途之一, 自动化地找源码 (例 升级测试里重编线上那个旧版本) 需要它, 不提供的话调用方只能自己拆串或者另抄一份 commitId 常量。
  它取出来的 commitId 只许拿去找源码, 不许参与任何判断。
- GetBuildTime: 显示构建时间 / 存档需要。不提供的话调用方要么自己拆串, 要么另外注入一个构建时间变量 —— 两个来源迟早对不上。
  只给人看 / 存档, 不许拿去比新旧。时区不写进版本号, 读的进程里 GBuildTimeZoneOffsetSecond 必须和打包时一样。
- CompareOrder: 排序 / 取最大 / 判 "≥" 需要三向比较。IsUpgrade 是 bool, "非法" 和 "不更新" 都是 false, 用 `!IsUpgrade` 拼 "≥" 会把非法版本号当成 "不更新" 放过去。
  所以 CompareOrder 必须带 ok。
- AddTimeSecond: 见下面 "生成侧"。

## 两位年份

时间戳 YYMMDDhhmmss 只有两位年份, 一律解释成 20YY (2000..2099)。GetBuildTime / AddTimeSecond 自己解析, 不用 time.Parse 的 "06" 布局:
它把 69..99 解析成 19xx, 2068 年加 1 秒格式化出 "69", 再解析回来就成了 1969 年。AddTimeSecond 的结果出了 2000..2099 直接返回 error。

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

- NewBuildVersion 是唯一从零生成版本号的地方。同一次打包的所有二进制必须共用同一个版本号: 调用方只调一次, 结果传给每个 go build。
- 不提供 "正式发布" 专用的生成函数 (检查工作区干净 + HEAD 已推送): 干净与否已经写在版本号末尾 (`-r` / `-d`), 一眼可见;
  "已推送" 只能看本地的远端跟踪分支, 还查不到子模块, 挡不住真正的 "找不到源码"。发布前确认干净和已推送是发布流程的事。
- AddTimeSecond: 给升级的自动测试用。测试会在几秒内连着打好几个包 (同一份代码 base -> next; 从 git 打的旧版本 -> 新版本; 拒绝降级要的更旧的包),
  常常落在同一秒, 序相等就不算升级。调用方把本次打出来的版本号往后 (或往前) 挪几秒。
  - 正式打包不自动挪: 正常打包间隔远大于 1 秒, 靠当前时间大概率不会出现 "新包不算升级"; 多台打包机时钟不一致是运维问题 (对时), 不在版本号上补。
  - 不提供 "After(prev)" / "Before(cur)" 这种包好的函数: 一个可正可负的原语就够, 挪几秒由调用方定。
  - 只改时间戳, 前缀 / commitId / 脏否不动: 挪的应当是本次打出来的版本号。挪别的包的版本号会让新包顶着别人的 commitId, 拿着版本号找到的是别的代码。
  - 前缀不同时挪时间戳追不上 (前缀优先比)。
  - 按真实时间进位 (…153059 + 1 秒 = …153100): 时间戳永远是一个真实存在的时刻, GetBuildTime 读得出来。
  - 不防伪造: 版本号本来就是明文, 谁都能手写一个; 来源可信靠包的签名 / MAC, 不靠版本号。
  - 和时区无关: 固定偏移下加减秒数不受偏移影响。
  - 代价: 挪过的时间戳不等于打包时刻 (GetBuildTime 读到的是挪过的)。
- 不假设当前目录: git 在 versionPrefix.txt 所在目录里跑, 取的是这个文件所在仓库的 HEAD / 脏否。
- 工作区脏的时候照样打, 只是末尾变成 `-d`: 开发期工作区基本总是脏的, 在这里拦等于没法测试。`-d` 只供事后溯源, 不参与比大小。
- git 是硬依赖: 没有 git / 不在 git 仓库里直接返回 error, 不猜一个 commitId。git 报错时 error 里带上 git 的 stderr (safe.directory / 空仓库 等原因都是 exit status 128, 只看退出码分不清)。
- versionPrefix.txt 本身必须被 git 跟踪 (`git ls-files --error-unmatch`), 否则返回 error: git status 不报被 ignore 的文件, 不查的话被 ignore 的前缀文件照样出 `-r`; 放在没有 .git 的被 ignore 子目录里时, git 往上找到外层仓库, commitId 也是外层的。
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
  这时 NewBuildVersion 会把脏工作区标成 `-r`。打包机上不要改这几项配置。
- **IsValid 不检查时间戳是不是真实日期**: 第三段只要求 12 位数字, `v0.1.999999999999-…` 也是合法版本号 (GetBuildTime / AddTimeSecond 对它返回失败)。
  这种串一旦被当成"最新版本"下发过一次 (手误或被篡改), 同一前缀下之后正常打的包都比它"旧", 再也不算升级, 只能升 versionPrefix.txt 的版本号顶过去。
  版本号是不透明串, 本库只锁格式不管来源: 对外部收进来的版本号要先确认来源可信 (例 包本身有 MAC 校验) 再拿去比新旧。

## 子包 hgmBuildVersionCurrent

- 只有一个变量 `GVersion`, 可用 `-ldflags -X` 注入, 或程序开始处直接赋值。
- `GetVersionVarFullName()` 返回 `<本子包 import path>.GVersion`, import path 运行期用 reflect 从本包里的类型取, 不写死。
- `-X` 目标写错时 go build 静默忽略, 编出来就是空串; 所以不写死 import path (fork / 换 module path 照样对), current_test.go 用真编真跑钉住。
- current_test.go 把小程序放在系统临时目录, 自带 go.mod 用 replace 指回本 module: 不往 module 目录里写 (从只读的 module cache 里跑 go test 也行), 测试里也不写死 import path。

## TypeScript 实现

`ts/hgmBuildVersion.ts` 实现 IsValid / IsUpgrade / CompareOrder (前端只需要校验和比较, 不生成也不挪版本号)。

- 两种语言各写一份, 很容易越改越不一样: `testdata/cases.json` 是两边共用的用例, Go 测试 (TestSharedCases) 和 TS 测试都读它,
  Go 的 TestTsImpl 用 node 跑 TS 测试, 一次 `go test` 两边都验。本机没有能直接跑 .ts 的 node (22.18 / 23.6 以上) 时跳过。
- 段1 / 段2 是 uint64, 超出 JS number 的精确范围 (2^53), TS 那边用 BigInt 比; cases.json 里有 uint64 顶端的用例钉着。
- 不 import 任何东西, 只用 ES2020: 下游直接复制进前端工程。不用 go:embed 发出去: go.mod 是 go 1.13, embed 要 1.16。

## 兼容的 Go 版本

go.mod 写 `go 1.13` (`%w` 错误包装从 1.13 起)。为此代码 / 测试刻意不用更新的东西: 不用泛型、strings.Cut / CutPrefix、os.ReadFile、t.TempDir (用 strings.IndexByte、ioutil.ReadFile、ioutil.TempDir 代替)。
改代码时不要引入比 1.13 新的标准库 API; 已用真 go1.13.15 跑过 go vet + go test。

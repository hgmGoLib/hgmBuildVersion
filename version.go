// Package hgmBuildVersion generates, validates and compares build version strings
// like "v0.1.260909155041-a1b2c3d4e5-r" (prefix + build timestamp + git commit + clean/dirty flag).
//
// 二进制版本号: 生成 (NewBuildVersion) / 校验 (IsValid) / 比大小 (IsUpgrade / CompareOrder)。
// 把版本号烤进二进制、运行期读出来见子包 hgmBuildVersionCurrent。
package hgmBuildVersion

// version.go — 版本号的格式: 生成侧 (makeVersion) + 解析侧 (IsValid / IsUpgrade) 同住一个文件。
//
// 形态: v<段1>.<段2>.<时间戳>-<commitId10>-<r|d>
//
//	例  v0.1.260909155041-a1b2c3d4e5-r
//
//	v0.1            前缀, 存在 git 里的 versionPrefix.txt (人手改, 大版本才动)
//	260909155041    打包那一刻的时间 YYMMDDhhmmss (定长 12 位; 时区见 GBuildTimeZoneOffsetSecond, 默认 UTC+8)
//	a1b2c3d4e5      打包时 HEAD 的 commitId 前 10 位 (小写 hex) —— 拿着它就能找到代码
//	r / d           打包时工作区干净(r=release)还是有未提交的修改(d=dirty)
//
// ★版本号对调用方是【不透明字符串】: 只能原样传递、判等 ==、交给本包的函数。
// 调用方不许自己拆段 / 比子串 / 改数字: 要什么由本包的函数给。本包不提供取脏否 / 取前缀的函数。
// 能从里面取的只有两样:
//   - GetCommitId: 拿着版本号找到源代码 (例 重编旧版本)。只许拿去找源码, 不许参与任何判断。
//   - GetBuildTime: 打包时刻, 给人看 / 存档 (见 timestamp.go)。
// 能改的只有时间戳 (AddTimeSecond, 见 timestamp.go)。
//
// ★【序】只由前三段决定, 后面两段不参与比大小。
// commitId 和脏否是【身份/溯源】信息, 不是【序】信息: 谁在前谁在后由打包时间决定, 和哪个 commit 无关。
// ★"序相等" ≠ "身份相同": 同一秒打的两个不同 commit 的包, 序相同而整串不同。
// 升级判定 (IsUpgrade) / 排序 (CompareOrder) 看【序】; "跑着的是不是这次编译的产物" 看【整串判等】。
//
// ★字符集刻意只用 [0-9a-zA-Z.-] (全是 RFC 3986 的 unreserved character):
// 版本号会进 URL、进 shell 脚本、进文件名、进目录名。
// 【任何时候改这个格式都不许引入 + / % & ? # 这类需要转义的字符】。
// 即便如此, 拼 URL / 拼 bash 仍然必须照常转义: 字符集安全是最后一道保险, 不是免转义的许可证。
//
// 为什么升级判定必须是【严格更大】:
//
//	同版本或更低版本一律不升。
//	* 同版本不升: 否则每次检查更新都判"需升级", 无限自更新。
//	* 更低版本不升: 防误操作/回滚攻击导致降级。
//	降级应走人工【卸载重装】, 不走自更新通道。

import "strings"

// versionOrderPartCount 【序】那部分的段数 (v<段1>.<段2>.<段3>)。
const versionOrderPartCount = 3

// versionStampLen 第三段 (打包时间戳 YYMMDDhhmmss) 的固定位数。
//
// ★定长是有意的: 定长就不存在"前导零导致同一个版本有两种写法"这种坑, 所以第三段不做前导零校验。
const versionStampLen = 12

// versionCommitIdLen 版本号里带的 commitId 位数 (git HEAD 完整 sha 的前这么多位)。
const versionCommitIdLen = 10

// makeVersion 拼一个版本号 —— 格式的【生成侧真源】, 与本文件的解析侧配对。
// 只给 NewBuildVersion 调; 单独成函数是为了测试能不碰 git/时钟 直接钉住格式。
//
// prefix   versionPrefix.txt 的内容, 形如 "v0.1"
// stamp    打包时刻的时间 "060102150405" 格式 (定长 12 位)
// commitId 打包时 HEAD 的完整 sha (这里自己截前 versionCommitIdLen 位)
// dirty    打包时工作区有没有未提交的修改
//
// 拼出来的串【不保证合法】(prefix 可能被人写错): 调用方必须紧接着过一次 IsValid, 早炸早好。
func makeVersion(prefix string, stamp string, commitId string, dirty bool) string {
	flag := "r"
	if dirty {
		flag = "d"
	}
	if len(commitId) > versionCommitIdLen {
		commitId = commitId[:versionCommitIdLen]
	}
	return prefix + "." + stamp + "-" + commitId + "-" + flag
}

// IsValid 版本号格式合法吗。
//
// ★凡是把版本号当【路径片段】用的地方必须先过它:
// 版本号可能从外部收进来, 不校验就等于让对端往任意路径读写。
//
// 格式被【整串锁死】: v<数字>.<数字>.<12位数字>-<10位小写hex>-<r|d>。
// 长度和字符集全定死之后 ".." 和 "/" 在语法上就不可能出现 —— 路径穿越那条线是被格式本身关掉的,
// 不是靠某个调用方记得过滤。
func IsValid(version string) bool {
	_, ok := orderParts(version)
	return ok
}

// IsUpgrade 判定 target 是否算"对 cur 的一次升级" —— 比【序】: 三段 uint64 从前往后比, 严格更大才算。
//
// 任何一边格式非法 (包括空串) -> false (拒绝升级): 版本号看不懂的时候, 不动比乱动安全。
//
// ★后两段 (commitId / 脏否) 不参与: 同一秒打的两个不同 commit 的包互相【不算升级】。
// 这是想要的 —— 它们谁也不比谁新。
func IsUpgrade(cur string, target string) bool {
	cmp, ok := CompareOrder(target, cur)
	return ok && cmp > 0 // 序全相等 = 同版本, 不升
}

// CompareOrder 三向比较 a 和 b 的【序】: a 更旧 -1, 序相等 0, a 更新 1。
// 任一边格式非法 (含空串) -> (0, false)。
//
// ★必须先看 ok 再看 cmp: 非法时 cmp 是 0, 不看 ok 就会把"看不懂"当成"一样新"。
// "b 不比 a 旧" 要写成 `cmp, ok := CompareOrder(b, a); ok && cmp >= 0`, 不要写 !IsUpgrade(b, a) —— 那样非法版本号也会被放过去。
//
// ★序相等 (cmp==0) 不等于同一个包: 判是不是同一个包用整串 ==。
func CompareOrder(a string, b string) (int, bool) {
	aPart, ok := orderParts(a)
	if !ok {
		return 0, false
	}
	bPart, ok := orderParts(b)
	if !ok {
		return 0, false
	}
	for i := range aPart {
		if aPart[i] < bPart[i] {
			return -1, true
		}
		if aPart[i] > bPart[i] {
			return 1, true
		}
	}
	return 0, true
}

// GetCommitId 取版本号里打包时 git HEAD 的 commitId (10 位小写 hex), 用来找到这个版本的源代码 (git checkout / git archive)。
// 版本号格式非法 (含空串) -> ("", false)。
//
// ★只许拿去找源码。不许参与任何判断 (比新旧 / 判是不是同一个包 / 判干净还是脏):
// 比新旧用 IsUpgrade, 判是不是同一个包用整串 ==。
// 末尾是 -d 的包打包时有未提交的修改, 这个 commit 只是最接近的那份代码。
func GetCommitId(version string) (string, bool) {
	if !IsValid(version) {
		return "", false
	}
	// 合法版本号的后缀恰好是 -<10位commitId>-<r|d>, 第一个 '-' 之后就是 commitId。
	dashIdx := strings.IndexByte(version, '-')
	return version[dashIdx+1 : dashIdx+1+versionCommitIdLen], true
}

// orderParts 校验整串, 并把【序】拆成三段 uint64。格式不对 -> ok=false。
//
//	"v0.1.260909155041-a1b2c3d4e5-r" -> ([0, 1, 260909155041], true)
//
// 自己解析而不用 strconv.ParseUint: 要顺便挡掉 "+1" / "-1" / " 1" / "0x10" 这些花样。
func orderParts(version string) ([versionOrderPartCount]uint64, bool) {
	var out [versionOrderPartCount]uint64
	// 按【第一个】'-' 切: 序那半全是数字和点, 不可能含 '-', 所以第一个 '-' 一定是分隔符。
	dashIdx := strings.IndexByte(version, '-')
	if dashIdx < 0 {
		return out, false
	}
	if !validVersionSuffix(version[dashIdx+1:]) {
		return out, false
	}
	orderStr := version[:dashIdx]
	if !strings.HasPrefix(orderStr, "v") {
		return out, false
	}
	segList := strings.Split(orderStr[1:], ".")
	if len(segList) != versionOrderPartCount {
		return out, false
	}
	for i, seg := range segList {
		if i == versionOrderPartCount-1 {
			// 第三段 = 打包时间戳, 定长 12 位纯数字 (定长 -> 不存在前导零歧义, 故不查前导零)。
			if len(seg) != versionStampLen {
				return out, false
			}
		} else {
			// 前两段来自 versionPrefix.txt: 任意位数的十进制, 但不许前导零
			// (否则 "v0.1.x" 和 "v0.01.x" 是两个不同字符串却是同一个版本)。
			if seg == "" || len(seg) > 20 { // 20 = uint64 最大值的十进制位数
				return out, false
			}
			if len(seg) > 1 && seg[0] == '0' {
				return out, false
			}
		}
		var n uint64
		for i2 := 0; i2 < len(seg); i2++ {
			c := seg[i2]
			if c < '0' || c > '9' {
				return out, false
			}
			if n > (^uint64(0)-uint64(c-'0'))/10 { // 溢出
				return out, false
			}
			n = n*10 + uint64(c-'0')
		}
		out[i] = n
	}
	return out, true
}

// validVersionSuffix 校验【身份后缀】那半: <10位小写hex>-<r|d>。
//
// ★这里是整串锁死的关键一半: 恰好两段、每段定长、字符集白名单。
// 于是 "." 和 "/" 在语法上都进不来 —— 版本号能当目录名用靠的就是这个。
//
// ★commitId 和脏否之间用 '-' 不用 '.': semver 的预发布段按 '.' 切标识符, 全数字的标识符不许前导零。
// 用 '.' 时 commitId 碰巧全是数字且以 0 开头 (约千分之一) 就不是合法 semver;
// 用 '-' 时整个后缀是【一个】含 '-' 的标识符, 永远不是数字型, 任何 commitId 都合法 (Go 伪版本同理)。
func validVersionSuffix(s string) bool {
	dashIdx := strings.IndexByte(s, '-')
	if dashIdx < 0 {
		return false
	}
	commitId, flag := s[:dashIdx], s[dashIdx+1:]
	if len(commitId) != versionCommitIdLen {
		return false
	}
	for i := 0; i < len(commitId); i++ {
		c := commitId[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return flag == "r" || flag == "d"
}

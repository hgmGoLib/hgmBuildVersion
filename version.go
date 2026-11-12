// Package hgmBuildVersion generates, validates and compares build version strings
// like "v0.0.1-261111123244-e64c21ea05-r" (prefix + build timestamp + git commit + clean/dirty flag).
// Comparison follows semver 2.0.0 precedence.
//
// 二进制版本号: 生成 (NewBuildVersion) / 校验 (IsValid) / 比大小 (IsUpgrade / CompareOrder, 按 semver 2.0)。
// 把版本号烤进二进制、运行期读出来见子包 hgmBuildVersionCurrent。
package hgmBuildVersion

// version.go — 版本号的格式: 生成侧 (makeVersion) + 解析侧 (IsValid / CompareOrder) 同住一个文件。
//
// 形态: v<a>.<b>.<c>-<12位时间戳>-<10位commitId>-<r|d>
//
//	例  v0.0.1-261111123244-e64c21ea05-r
//
//	v0.0.1          前缀, 存在 git 里的 versionPrefix.txt (人手改, 三段, 一般只动最后一段)
//	261111123244    打包那一刻的时间 YYMMDDhhmmss (定长 12 位; 时区见 GBuildTimeZoneOffsetSecond, 默认 UTC+8)
//	e64c21ea05      打包时 HEAD 的 commitId 前 10 位 (小写 hex) —— 拿着它就能找到代码
//	r / d           打包时工作区干净(r=release)还是有未提交的修改(d=dirty)
//
// 在 semver 眼里 '-' 后面整个是【一个】预发布标识符 (定长 25 字节, 带 '-' 所以永远不是数字型),
// 样子刻意接近 Go 的伪版本号 (v0.0.0-20191109021931-daa7c04131f5)。
//
// ★比大小 = semver 2.0 优先级, 规则一点不改 (CompareOrder)。本库格式在 semver 下的效果:
//   - 前缀三段先比; 前缀相同再按 ASCII 比后缀, 时间戳定长 12 位在最前, 所以就是按打包时刻比;
//   - 同一秒打的两个包接着比 commitId / 脏否: 有先后但没有含义 (确定, 不会循环升级);
//   - 对本库格式, cmp==0 <=> 整串相等。
//
// ★改后缀格式 (位数 / 字段 / 分隔符) 时前缀必须 +1: 同一前缀下后缀按 ASCII 比, 能排对全靠时间戳定长。
// 前缀 +1 之后, 旧二进制 (只要它也是 semver 比较器) 不管新后缀长什么样都判为升级, 不用开新的升级通道。
// 改时区 / 2099 年两位年份回绕 / 被一个错误的大时间戳占住 "最新" 时同理: 前缀 +1。
//
// ★版本号对调用方是【不透明字符串】: 只能原样传递、判等 ==、交给本包的函数。
// 调用方不许自己拆段 / 比子串 / 改数字: 要什么由本包的函数给。本包不提供取脏否 / 取前缀的函数。
// 能从里面取的只有两样:
//   - GetCommitId: 拿着版本号找到源代码 (例 重编旧版本)。只许拿去找源码, 不许参与任何判断。
//   - GetBuildTime: 打包时刻, 给人看 / 存档 (见 timestamp.go)。
// 能改的只有时间戳 (AddTimeSecond, 见 timestamp.go)。
//
// ★能比较 ≠ 能当路径用: CompareOrder 认任何 semver (含 +build、长度不定), 路径安全只认 IsValid (整串锁死本库格式)。
//
// ★字符集刻意只用 [0-9a-zA-Z.-] (全是 RFC 3986 的 unreserved character):
// 版本号会进 URL、进 shell 脚本、进文件名、进目录名。
// 【任何时候改这个格式都不许引入 + / % & ? # 这类需要转义的字符】。本库生成的版本号不带 +build:
// semver 的 build 元数据不参与排序, 用了它就等于放弃 "每次打包自动得到一个可排序的新版本"。
// 即便如此, 拼 URL / 拼 bash 仍然必须照常转义: 字符集安全是最后一道保险, 不是免转义的许可证。
//
// 为什么升级判定必须是【严格更大】:
//
//	同版本或更低版本一律不升。
//	* 同版本不升: 否则每次检查更新都判"需升级", 无限自更新。
//	* 更低版本不升: 防误操作/回滚攻击导致降级。
//	降级应走人工【卸载重装】, 不走自更新通道。

import "strings"

// versionStampLen 时间戳 YYMMDDhhmmss 的固定位数。
//
// ★定长是有意的: 同一前缀下后缀按 ASCII 比, 定长才等于按时间比。
const versionStampLen = 12

// versionCommitIdLen 版本号里带的 commitId 位数 (git HEAD 完整 sha 的前这么多位)。
const versionCommitIdLen = 10

// versionSuffixLen 本库格式 '-' 之后那个预发布标识符的固定长度: <12位时间戳>-<10位commitId>-<r|d>。
const versionSuffixLen = versionStampLen + 1 + versionCommitIdLen + 2

// maxVersionLen CompareOrder / IsValid 接受的最大长度 (字节), 超了一律看不懂。
// 本库格式三十来个字节, 256 给外来 semver 留足余量, 同时挡住超长输入。
const maxVersionLen = 256

// makeVersion 拼一个版本号 —— 格式的【生成侧真源】, 与本文件的解析侧配对。
// 只给 NewBuildVersion 调; 单独成函数是为了测试能不碰 git/时钟 直接钉住格式。
//
// prefix   versionPrefix.txt 的内容, 形如 "v0.0.1"
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
	return prefix + "-" + stamp + "-" + commitId + "-" + flag
}

// IsValid 是不是本库格式的版本号 (整串锁死): v<a>.<b>.<c>-<12位数字>-<10位小写hex>-<r|d>。
// 前缀三段是十进制、不许前导零 (否则 "v0.1.0" 和 "v0.01.0" 是两个字符串却是同一个版本), 不限位数, 总长不超过 maxVersionLen。
//
// ★凡是把版本号当【路径片段】用的地方必须先过它:
// 版本号可能从外部收进来, 不校验就等于让对端往任意路径读写。
// 长度和字符集全定死之后 ".." 和 "/" 在语法上就不可能出现 —— 路径穿越那条线是被格式本身关掉的,
// 不是靠某个调用方记得过滤。
//
// 合法 semver 但不是本库格式的 (例 v1.2.3、带 +build 的、v0.0.6 及以前的旧格式) -> false; 它们照样能交给 CompareOrder 比。
func IsValid(version string) bool {
	p, ok := parseSemver(version)
	return ok && !p.hasBuild && validVersionSuffix(p.pre)
}

// IsUpgrade 判定 target 是否算"对 cur 的一次升级": 按 semver 2.0 优先级严格更大才算。
//
// 任何一边看不懂 (不是 semver / 没有 v / 简写 / 超长, 包括空串) -> false (拒绝升级): 版本号看不懂的时候, 不动比乱动安全。
func IsUpgrade(cur string, target string) bool {
	cmp, ok := CompareOrder(target, cur)
	return ok && cmp > 0 // 相等 = 同版本, 不升
}

// CompareOrder 按 semver 2.0.0 的优先级三向比较 a 和 b: a 更旧 -1, 一样新 0, a 更新 1。
// 任一边看不懂 -> (0, false)。零分配。
//
// 规则就是 semver 2.0.0 (https://semver.org/spec/v2.0.0.html 第 11 条), 一点不改:
//   - 三段核心按数值比 (不限位数: 不许前导零, 所以先比位数再比字节就是按数值比);
//   - 有预发布 (-xxx) 的比没有的旧; 预发布按 '.' 切开逐个比: 数字型按数值, 字母数字型按 ASCII, 数字型 < 字母数字型,
//     前面都相等时标识符多的更新;
//   - build 元数据 (+xxx) 是合法输入, 比较时忽略 (v1.0.0+a 和 v1.0.0+b 一样新)。
// 和 semver 规范相比只多两条限制: 必须以小写 v 开头 (Go 的习惯), 必须写满三段 (不认 v1 / v1.2 这种简写);
// 外加总长度不超过 maxVersionLen。
//
// ★必须先看 ok 再看 cmp: 看不懂时 cmp 是 0, 不看 ok 就会把"看不懂"当成"一样新"。
// "b 不比 a 旧" 要写成 `cmp, ok := CompareOrder(b, a); ok && cmp >= 0`, 不要写 !IsUpgrade(b, a) —— 那样看不懂的版本号也会被放过去。
//
// ★cmp==0 对本库格式就是整串相等; 对外来 semver 不一定 (只差 +build 的两个串也是 0)。
func CompareOrder(a string, b string) (int, bool) {
	ap, ok := parseSemver(a)
	if !ok {
		return 0, false
	}
	bp, ok := parseSemver(b)
	if !ok {
		return 0, false
	}
	if c := compareNumIdent(ap.major, bp.major); c != 0 {
		return c, true
	}
	if c := compareNumIdent(ap.minor, bp.minor); c != 0 {
		return c, true
	}
	if c := compareNumIdent(ap.patch, bp.patch); c != 0 {
		return c, true
	}
	return comparePrerelease(ap.pre, bp.pre), true
}

// GetCommitId 取版本号里打包时 git HEAD 的 commitId (10 位小写 hex), 用来找到这个版本的源代码 (git checkout / git archive)。
// 不是本库格式 (含空串、合法的外来 semver) -> ("", false)。
//
// ★只许拿去找源码。不许参与任何判断 (比新旧 / 判是不是同一个包 / 判干净还是脏):
// 比新旧用 IsUpgrade, 判是不是同一个包用整串 ==。
// 末尾是 -d 的包打包时有未提交的修改, 这个 commit 只是最接近的那份代码。
func GetCommitId(version string) (string, bool) {
	if !IsValid(version) {
		return "", false
	}
	// 本库格式的前缀里没有 '-', 第一个 '-' 之后是 <12位时间戳>-<10位commitId>-<r|d>。
	start := strings.IndexByte(version, '-') + 1 + versionStampLen + 1
	return version[start : start+versionCommitIdLen], true
}

// semverParts_t parseSemver 的结果: 全是原串的子串, 不分配。
type semverParts_t struct {
	major    string
	minor    string
	patch    string
	pre      string // 预发布 ('-' 后面, 不含 '-'); 没有预发布是 ""
	hasBuild bool   // 有没有 +build (只校验, 不参与比较)
}

// parseSemver 按 semver 2.0.0 的文法 (加上 "v 开头 / 写满三段 / 总长上限") 校验并切开 v。不合法 -> ok=false。
//
// 自己逐字节解析: 要挡掉 "+1" / "-1" / " 1" / "0x10" / 前导零 / 非 ASCII 这些花样, 而且不能分配。
func parseSemver(v string) (semverParts_t, bool) {
	var p semverParts_t
	if len(v) < 2 || len(v) > maxVersionLen || v[0] != 'v' {
		return p, false
	}
	i := 1
	var ok bool
	if p.major, i, ok = scanCoreNum(v, i); !ok || i >= len(v) || v[i] != '.' {
		return p, false
	}
	if p.minor, i, ok = scanCoreNum(v, i+1); !ok || i >= len(v) || v[i] != '.' {
		return p, false
	}
	if p.patch, i, ok = scanCoreNum(v, i+1); !ok {
		return p, false
	}
	if i < len(v) && v[i] == '-' {
		end := i + 1
		for end < len(v) && v[end] != '+' {
			end++
		}
		p.pre = v[i+1 : end]
		if !validIdentList(p.pre, true) {
			return p, false
		}
		i = end
	}
	if i < len(v) && v[i] == '+' {
		if !validIdentList(v[i+1:], false) {
			return p, false
		}
		p.hasBuild = true
		i = len(v)
	}
	if i != len(v) {
		return p, false
	}
	return p, true
}

// scanCoreNum 从 v[i] 起读一个核心数字段 (非空十进制, 不许前导零), 返回这一段和它后面的下标。
func scanCoreNum(v string, i int) (string, int, bool) {
	start := i
	for i < len(v) && v[i] >= '0' && v[i] <= '9' {
		i++
	}
	seg := v[start:i]
	if seg == "" || (len(seg) > 1 && seg[0] == '0') {
		return "", i, false
	}
	return seg, i, true
}

// validIdentList 校验按 '.' 分开的标识符列表: 每个非空, 只含 [0-9A-Za-z-];
// isPre 时 (预发布) 全数字的标识符不许前导零 (build 元数据允许)。
func validIdentList(s string, isPre bool) bool {
	if s == "" {
		return false
	}
	start := 0
	for start <= len(s) {
		end := start
		for end < len(s) && s[end] != '.' {
			end++
		}
		ident := s[start:end]
		if ident == "" {
			return false
		}
		allDigit := true
		for k := 0; k < len(ident); k++ {
			c := ident[k]
			switch {
			case c >= '0' && c <= '9':
			case (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '-':
				allDigit = false
			default:
				return false
			}
		}
		if isPre && allDigit && len(ident) > 1 && ident[0] == '0' {
			return false
		}
		start = end + 1
	}
	return true
}

// compareNumIdent 比两个合法的十进制数字串 (不许前导零): 位数多的大, 位数相同按字节比。不限位数, 没有溢出。
func compareNumIdent(a string, b string) int {
	if len(a) != len(b) {
		if len(a) < len(b) {
			return -1
		}
		return 1
	}
	return strings.Compare(a, b)
}

// comparePrerelease 按 semver 2.0.0 第 11 条比两个 (已校验过的) 预发布串; "" 表示没有预发布 (比任何预发布都新)。
func comparePrerelease(a string, b string) int {
	if a == b {
		return 0
	}
	if a == "" {
		return 1
	}
	if b == "" {
		return -1
	}
	ai, bi := 0, 0
	for {
		if ai > len(a) || bi > len(b) {
			break
		}
		aEnd := ai
		for aEnd < len(a) && a[aEnd] != '.' {
			aEnd++
		}
		bEnd := bi
		for bEnd < len(b) && b[bEnd] != '.' {
			bEnd++
		}
		if c := compareIdent(a[ai:aEnd], b[bi:bEnd]); c != 0 {
			return c
		}
		ai, bi = aEnd+1, bEnd+1
	}
	// 前面都相等: 标识符多的更新。
	switch {
	case ai > len(a) && bi > len(b):
		return 0
	case ai > len(a):
		return -1
	default:
		return 1
	}
}

// compareIdent 比两个预发布标识符: 都是数字按数值, 数字型 < 字母数字型, 都是字母数字型按 ASCII。
func compareIdent(a string, b string) int {
	aNum, bNum := isAllDigit(a), isAllDigit(b)
	switch {
	case aNum && bNum:
		return compareNumIdent(a, b)
	case aNum:
		return -1
	case bNum:
		return 1
	}
	return strings.Compare(a, b)
}

func isAllDigit(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// validVersionSuffix 本库格式的后缀 (预发布那个标识符): <12位数字>-<10位小写hex>-<r|d>。
//
// ★这里是整串锁死的关键: 定长、字符集白名单, 于是 "." 和 "/" 在语法上都进不来 —— 版本号能当目录名用靠的就是这个。
// ★三部分之间用 '-' 不用 '.': 用 '.' 时 semver 会把它们切成三个标识符, 时间戳和全数字的 commitId 就成了数字型,
// 以 0 开头时不是合法 semver (v0.0.4 的格式踩过); 用 '-' 时整个后缀是【一个】带 '-' 的字母数字型标识符, 任何取值都合法。
func validVersionSuffix(s string) bool {
	if len(s) != versionSuffixLen {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case i < versionStampLen:
			if c < '0' || c > '9' {
				return false
			}
		case i == versionStampLen || i == versionStampLen+1+versionCommitIdLen:
			if c != '-' {
				return false
			}
		case i < versionStampLen+1+versionCommitIdLen:
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
				return false
			}
		default:
			if c != 'r' && c != 'd' {
				return false
			}
		}
	}
	return true
}

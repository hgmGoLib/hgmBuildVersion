// hgmBuildVersion.ts — Go 版 hgmBuildVersion 的 IsValid / IsUpgrade / CompareOrder 的 TS 实现, 给前端用。
//
// 规则和 Go 版完全一样 (见 ../version.go), 两边共用 ../testdata/cases.json 这份用例, Go 的 go test 会顺带跑 TS 这边。
// ★改规则 = 改 cases.json + 两边实现, 不许只改一边。算法也照 Go 版逐字节扫描写, 方便对照。
//
// 只用 ES2020, 不 import 任何东西: 下游直接把这个文件复制进自己的前端工程即可。
// 不需要大整数类型: 数字段一律按 "先比位数再比字符串" 比 (不许前导零, 所以这就是按数值比), 不转成数字, 多长都比得准。
//
// 比大小 = semver 2.0.0 优先级, 规则一点不改 (+build 合法, 比较时忽略)。
// 只多三条限制: 必须以小写 v 开头, 必须写满三段 (不认 v1 / v1.2), 总长不超过 maxVersionLen。

const versionStampLen = 12
const versionCommitIdLen = 10
const versionSuffixLen = versionStampLen + 1 + versionCommitIdLen + 2
const maxVersionLen = 256

interface SemverParts {
	major: string
	minor: string
	patch: string
	pre: string // 预发布 ('-' 后面, 不含 '-'); 没有预发布是 ""
	hasBuild: boolean
}

function isDigit(c: number): boolean {
	return c >= 48 && c <= 57 // '0'..'9'
}

// scanCoreNum 从 v[i] 起读一个核心数字段 (非空十进制, 不许前导零), 返回 [这一段, 后面的下标], 不合法 -> null。
function scanCoreNum(v: string, i: number): [string, number] | null {
	const start = i
	while (i < v.length && isDigit(v.charCodeAt(i))) {
		i++
	}
	const seg = v.slice(start, i)
	if (seg === "" || (seg.length > 1 && seg[0] === "0")) {
		return null
	}
	return [seg, i]
}

// validIdentList 按 '.' 分开的标识符列表: 每个非空, 只含 [0-9A-Za-z-]; isPre 时全数字的标识符不许前导零。
function validIdentList(s: string, isPre: boolean): boolean {
	if (s === "") {
		return false
	}
	let start = 0
	while (start <= s.length) {
		let end = start
		while (end < s.length && s[end] !== ".") {
			end++
		}
		if (end === start) {
			return false
		}
		let allDigit = true
		for (let k = start; k < end; k++) {
			const c = s.charCodeAt(k)
			if (isDigit(c)) {
				continue
			}
			if ((c >= 97 && c <= 122) || (c >= 65 && c <= 90) || c === 45) { // a-z A-Z -
				allDigit = false
				continue
			}
			return false
		}
		if (isPre && allDigit && end - start > 1 && s[start] === "0") {
			return false
		}
		start = end + 1
	}
	return true
}

// parseSemver 按 semver 2.0.0 文法 (加上 v 开头 / 写满三段 / 总长上限) 校验并切开。不合法 -> null。
function parseSemver(v: string): SemverParts | null {
	if (v.length < 2 || v.length > maxVersionLen || v[0] !== "v") {
		return null
	}
	const ma = scanCoreNum(v, 1)
	if (ma === null || ma[1] >= v.length || v[ma[1]] !== ".") {
		return null
	}
	const mi = scanCoreNum(v, ma[1] + 1)
	if (mi === null || mi[1] >= v.length || v[mi[1]] !== ".") {
		return null
	}
	const pa = scanCoreNum(v, mi[1] + 1)
	if (pa === null) {
		return null
	}
	let i = pa[1]
	let pre = ""
	let hasBuild = false
	if (i < v.length && v[i] === "-") {
		let end = i + 1
		while (end < v.length && v[end] !== "+") {
			end++
		}
		pre = v.slice(i + 1, end)
		if (!validIdentList(pre, true)) {
			return null
		}
		i = end
	}
	if (i < v.length && v[i] === "+") {
		if (!validIdentList(v.slice(i + 1), false)) {
			return null
		}
		hasBuild = true
		i = v.length
	}
	if (i !== v.length) {
		return null
	}
	return { major: ma[0], minor: mi[0], patch: pa[0], pre: pre, hasBuild: hasBuild }
}

// compareStr 按字节 (这里都是 ASCII, 等于 UTF-16 code unit) 比。
function compareStr(a: string, b: string): number {
	return a < b ? -1 : (a > b ? 1 : 0)
}

// compareNumIdent 比两个合法十进制数字串 (不许前导零): 位数多的大, 位数相同按字符串比。
function compareNumIdent(a: string, b: string): number {
	if (a.length !== b.length) {
		return a.length < b.length ? -1 : 1
	}
	return compareStr(a, b)
}

function isAllDigit(s: string): boolean {
	for (let i = 0; i < s.length; i++) {
		if (!isDigit(s.charCodeAt(i))) {
			return false
		}
	}
	return true
}

// compareIdent 预发布标识符: 都是数字按数值, 数字型 < 字母数字型, 都是字母数字型按 ASCII。
function compareIdent(a: string, b: string): number {
	const aNum = isAllDigit(a)
	const bNum = isAllDigit(b)
	if (aNum && bNum) {
		return compareNumIdent(a, b)
	}
	if (aNum) {
		return -1
	}
	if (bNum) {
		return 1
	}
	return compareStr(a, b)
}

// comparePrerelease semver 2.0.0 第 11 条; "" = 没有预发布 (比任何预发布都新)。
function comparePrerelease(a: string, b: string): number {
	if (a === b) {
		return 0
	}
	if (a === "") {
		return 1
	}
	if (b === "") {
		return -1
	}
	const aList = a.split(".")
	const bList = b.split(".")
	const n = Math.min(aList.length, bList.length)
	for (let i = 0; i < n; i++) {
		const c = compareIdent(aList[i], bList[i])
		if (c !== 0) {
			return c
		}
	}
	// 前面都相等: 标识符多的更新。
	return compareNumIdent(String(aList.length), String(bList.length))
}

// validVersionSuffix 本库格式的后缀: <12位数字>-<10位小写hex>-<r|d>。定长 + 字符集白名单, "." 和 "/" 进不来。
function validVersionSuffix(s: string): boolean {
	if (s.length !== versionSuffixLen) {
		return false
	}
	for (let i = 0; i < s.length; i++) {
		const c = s.charCodeAt(i)
		if (i < versionStampLen) {
			if (!isDigit(c)) {
				return false
			}
		} else if (i === versionStampLen || i === versionStampLen + 1 + versionCommitIdLen) {
			if (c !== 45) { // '-'
				return false
			}
		} else if (i < versionStampLen + 1 + versionCommitIdLen) {
			if (!(isDigit(c) || (c >= 97 && c <= 102))) { // 0-9 a-f
				return false
			}
		} else {
			if (s[i] !== "r" && s[i] !== "d") {
				return false
			}
		}
	}
	return true
}

// IsValid 是不是本库格式的版本号 (整串锁死): v<a>.<b>.<c>-<12位数字>-<10位小写hex>-<r|d>。当路径片段 / 拼进 URL 之前必须先过它。
// 合法 semver 但不是本库格式的 (v1.2.3 / 带 +build / v0.0.6 及以前的旧格式) -> false, 它们照样能交给 CompareOrder 比。
export function IsValid(version: string): boolean {
	const p = parseSemver(version)
	return p !== null && !p.hasBuild && validVersionSuffix(p.pre)
}

// CompareOrder 按 semver 2.0.0 优先级三向比较: a 更旧 -1, 一样新 0, a 更新 1。任一边看不懂 -> {Cmp: 0, Ok: false}。
// ★先看 Ok 再看 Cmp。Cmp==0 对本库格式就是整串相等; 对外来 semver 不一定 (只差 +build 的两个串也是 0)。
export function CompareOrder(a: string, b: string): { Cmp: number, Ok: boolean } {
	const ap = parseSemver(a)
	if (ap === null) {
		return { Cmp: 0, Ok: false }
	}
	const bp = parseSemver(b)
	if (bp === null) {
		return { Cmp: 0, Ok: false }
	}
	let c = compareNumIdent(ap.major, bp.major)
	if (c === 0) {
		c = compareNumIdent(ap.minor, bp.minor)
	}
	if (c === 0) {
		c = compareNumIdent(ap.patch, bp.patch)
	}
	if (c === 0) {
		c = comparePrerelease(ap.pre, bp.pre)
	}
	return { Cmp: c, Ok: true }
}

// IsUpgrade target 相对 cur 是不是严格更新。任一边看不懂 (含空串) -> false。
export function IsUpgrade(cur: string, target: string): boolean {
	const r = CompareOrder(target, cur)
	return r.Ok && r.Cmp > 0
}

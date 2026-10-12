// hgmBuildVersion.ts — Go 版 hgmBuildVersion 的 IsValid / IsUpgrade / CompareOrder 的 TS 实现, 给前端用。
//
// 规则和 Go 版完全一样 (见 ../version.go), 两边共用 ../testdata/cases.json 这份用例, Go 的 go test 会顺带跑 TS 这边。
// ★改规则 = 改 cases.json + 两边实现, 不许只改一边。
//
// 只用 ES2020 (BigInt), 不 import 任何东西: 下游直接把这个文件复制进自己的前端工程即可。
// ★段1 / 段2 是 uint64, 超出 JS number 的精确范围 (2^53): 必须用 BigInt 比, 不能 Number()。

const versionStampLen = 12
const versionCommitIdLen = 10
const maxUint64 = (BigInt(1) << BigInt(64)) - BigInt(1)

// orderParts 校验整串, 并把【序】拆成三段 BigInt。格式不对 -> null。
function orderParts(version: string): bigint[] | null {
	const dashIdx = version.indexOf("-")
	if (dashIdx < 0) {
		return null
	}
	if (!validVersionSuffix(version.slice(dashIdx + 1))) {
		return null
	}
	const orderStr = version.slice(0, dashIdx)
	if (!orderStr.startsWith("v")) {
		return null
	}
	const segList = orderStr.slice(1).split(".")
	if (segList.length !== 3) {
		return null
	}
	const out: bigint[] = []
	for (let i = 0; i < segList.length; i++) {
		const seg = segList[i]
		if (i === 2) {
			if (seg.length !== versionStampLen) {
				return null
			}
		} else {
			if (seg === "" || seg.length > 20) {
				return null
			}
			if (seg.length > 1 && seg[0] === "0") {
				return null
			}
		}
		if (!/^[0-9]+$/.test(seg)) {
			return null
		}
		const n = BigInt(seg)
		if (n > maxUint64) {
			return null
		}
		out.push(n)
	}
	return out
}

// validVersionSuffix 身份后缀那半: <10位小写hex>-<r|d>。
function validVersionSuffix(s: string): boolean {
	const re = new RegExp("^[0-9a-f]{" + versionCommitIdLen + "}-[rd]$")
	return re.test(s)
}

// IsValid 版本号格式合法吗。当路径片段 / 拼进 URL 之前必须先过它。
export function IsValid(version: string): boolean {
	return orderParts(version) !== null
}

// CompareOrder 三向比较 a 和 b 的【序】: a 更旧 -1, 序相等 0, a 更新 1。任一边非法 -> {Cmp: 0, Ok: false}。
// ★先看 Ok 再看 Cmp。序相等不等于同一个包 (判同一个包用整串 ===)。
export function CompareOrder(a: string, b: string): { Cmp: number, Ok: boolean } {
	const aPart = orderParts(a)
	const bPart = orderParts(b)
	if (aPart === null || bPart === null) {
		return { Cmp: 0, Ok: false }
	}
	for (let i = 0; i < aPart.length; i++) {
		if (aPart[i] < bPart[i]) {
			return { Cmp: -1, Ok: true }
		}
		if (aPart[i] > bPart[i]) {
			return { Cmp: 1, Ok: true }
		}
	}
	return { Cmp: 0, Ok: true }
}

// IsUpgrade target 相对 cur 是不是严格更新。任一边非法 (含空串) -> false。
export function IsUpgrade(cur: string, target: string): boolean {
	const r = CompareOrder(target, cur)
	return r.Ok && r.Cmp > 0
}

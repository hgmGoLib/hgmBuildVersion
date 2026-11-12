// hgmBuildVersion_test.ts — TS 实现跑 ../testdata/cases.json (和 Go 共用的用例)。
// 跑法: node ts/hgmBuildVersion_test.ts (node 22.18 / 23.6 以上能直接跑 .ts)。go test 会顺带跑它 (version_test.go 的 TestTsImpl)。
// 断言的不变式和 Go 的 TestSharedCases 一样, 见 cases.json 的 Note。

import { readFileSync } from "node:fs"
import { CompareOrder, IsUpgrade, IsValid } from "./hgmBuildVersion.ts"

const verA = "v0.0.1-260901140715-a1b2c3d4e5-r"
const cases = JSON.parse(readFileSync(new URL("../testdata/cases.json", import.meta.url), "utf8"))
const failList: string[] = []
function check(ok: boolean, msg: string) {
	if (!ok) {
		failList.push(msg)
	}
}

// cases.json 里写不下的超长串用占位符: LONG256 / LONG257 = "v1.2.3-" 加 a, 总长正好 256 / 257 字节。
function expandCase(s: string): string {
	if (s === "LONG256") {
		return "v1.2.3-" + "a".repeat(256 - 7)
	}
	if (s === "LONG257") {
		return "v1.2.3-" + "a".repeat(257 - 7)
	}
	return s
}

check(cases.IsValid.length > 0 && cases.NotComparable.length > 0 && cases.CompareOrder.length > 0 && cases.OrderChain.length > 0, "用例为空")
for (const c of cases.IsValid) {
	check(IsValid(c.Version) === c.Want, "IsValid(" + JSON.stringify(c.Version) + ") 应为 " + c.Want + ": " + c.Note)
	if (c.Want) {
		check(CompareOrder(verA, c.Version).Ok, "合法版本号应能比较: " + c.Note)
	}
}
for (const c of cases.NotComparable) {
	const v = expandCase(c.Version)
	check(!IsValid(v), "比不了的串不应 IsValid: " + c.Note)
	check(!CompareOrder(verA, v).Ok, "CompareOrder(verA, " + JSON.stringify(v) + ") 应 Ok=false: " + c.Note)
	check(!CompareOrder(v, verA).Ok, "CompareOrder(" + JSON.stringify(v) + ", verA) 应 Ok=false: " + c.Note)
	check(!CompareOrder(v, v).Ok, "CompareOrder(" + JSON.stringify(v) + ", 自己) 应 Ok=false: " + c.Note)
	check(!IsUpgrade(verA, v), "看不懂的目标版本不许升: " + c.Note)
	check(!IsUpgrade(v, verA), "当前版本看不懂时不许升: " + c.Note)
}
for (const c of cases.CompareOrder) {
	const a = expandCase(c.A)
	const b = expandCase(c.B)
	const r = CompareOrder(a, b)
	check(r.Cmp === c.Cmp && r.Ok === c.Ok, "CompareOrder(" + JSON.stringify(a) + ", " + JSON.stringify(b) + ") = " + JSON.stringify(r) + ", 应为 " + c.Cmp + "/" + c.Ok + ": " + c.Note)
	check(IsUpgrade(a, b) === (c.Ok && c.Cmp < 0), "IsUpgrade(" + JSON.stringify(a) + ", " + JSON.stringify(b) + ") 和 CompareOrder 对不上: " + c.Note)
}
for (const c of cases.OrderChain) {
	for (let i = 0; i < c.List.length; i++) {
		for (let j = 0; j < c.List.length; j++) {
			const want = i < j ? -1 : (i > j ? 1 : 0)
			const r = CompareOrder(c.List[i], c.List[j])
			check(r.Ok && r.Cmp === want, "CompareOrder(" + JSON.stringify(c.List[i]) + ", " + JSON.stringify(c.List[j]) + ") = " + JSON.stringify(r) + ", 应为 " + want + "/true: " + c.Note)
		}
	}
}

if (failList.length > 0) {
	console.error(failList.join("\n"))
	process.exit(1)
}
console.log("ok")

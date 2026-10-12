// hgmBuildVersion_test.ts — TS 实现跑 ../testdata/cases.json (和 Go 共用的用例)。
// 跑法: node ts/hgmBuildVersion_test.ts (node 22.18 / 23.6 以上能直接跑 .ts)。go test 会顺带跑它 (version_test.go 的 TestTsImpl)。

import { readFileSync } from "node:fs"
import { CompareOrder, IsUpgrade, IsValid } from "./hgmBuildVersion.ts"

const verA = "v0.1.260901140715-a1b2c3d4e5-r"
const cases = JSON.parse(readFileSync(new URL("../testdata/cases.json", import.meta.url), "utf8"))
const failList: string[] = []
function check(ok: boolean, msg: string) {
	if (!ok) {
		failList.push(msg)
	}
}

check(cases.IsValid.length > 0 && cases.CompareOrder.length > 0, "用例为空")
for (const c of cases.IsValid) {
	check(IsValid(c.Version) === c.Want, "IsValid(" + JSON.stringify(c.Version) + ") 应为 " + c.Want + ": " + c.Note)
	if (c.Want) {
		continue
	}
	check(!CompareOrder(verA, c.Version).Ok, "非法版本号不应比得出序: " + c.Note)
	check(!CompareOrder(c.Version, verA).Ok, "非法版本号不应比得出序: " + c.Note)
	check(!IsUpgrade(verA, c.Version), "非法目标版本不许升: " + c.Note)
	check(!IsUpgrade(c.Version, verA), "当前版本非法时不许升: " + c.Note)
}
for (const c of cases.CompareOrder) {
	const r = CompareOrder(c.A, c.B)
	check(r.Cmp === c.Cmp && r.Ok === c.Ok, "CompareOrder(" + JSON.stringify(c.A) + ", " + JSON.stringify(c.B) + ") = " + JSON.stringify(r) + ", 应为 " + c.Cmp + "/" + c.Ok + ": " + c.Note)
	check(IsUpgrade(c.A, c.B) === (c.Ok && c.Cmp < 0), "IsUpgrade(" + JSON.stringify(c.A) + ", " + JSON.stringify(c.B) + ") 和 CompareOrder 对不上: " + c.Note)
}

if (failList.length > 0) {
	console.error(failList.join("\n"))
	process.exit(1)
}
console.log("ok")

package hgmBuildVersion

// version_test.go — 格式 (makeVersion / IsValid)、比序 (IsUpgrade / CompareOrder)。

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// 不依赖外部测试库。
func testOk(t *testing.T, ok bool, msg string) {
	t.Helper()
	if !ok {
		t.Fatal("testOk fail: " + msg)
	}
}

// got / want 必须是可比较的同一类型 (不用泛型: 保住 go.mod 里的低 go 版本)。
func testEqual(t *testing.T, got interface{}, want interface{}) {
	t.Helper()
	if got != want {
		t.Fatalf("testEqual fail: got %#v want %#v", got, want)
	}
}

// 本文件里反复用到的几个合法版本号。
const (
	verA    = "v0.0.1-260901140715-a1b2c3d4e5-r" // 基准
	verANew = "v0.0.1-260901140716-a1b2c3d4e5-r" // 只比 verA 晚一秒
	verADup = "v0.0.1-260901140715-ffffffffff-d" // 与 verA 同一秒、不同 commit
)

// testCases_t testdata/cases.json: Go 和 TS (ts/hgmBuildVersion_test.ts) 共用的用例。
type testCases_t struct {
	IsValid []struct {
		Version string
		Want    bool
		Note    string
	}
	NotComparable []struct {
		Version string
		Note    string
	}
	CompareOrder []struct {
		A    string
		B    string
		Cmp  int
		Ok   bool
		Note string
	}
	OrderChain []struct {
		List []string
		Note string
	}
}

// testExpandCase cases.json 里写不下的超长串用占位符: LONG256 / LONG257 = "v1.2.3-" 加 a, 总长正好 256 / 257 字节。
func testExpandCase(s string) string {
	switch s {
	case "LONG256":
		return "v1.2.3-" + strings.Repeat("a", 256-7)
	case "LONG257":
		return "v1.2.3-" + strings.Repeat("a", 257-7)
	}
	return s
}

// 共用用例: 格式校验 (IsValid) + 比大小 (CompareOrder / IsUpgrade, 按 semver 2.0)。
//
// ★IsValid 那张表同时是【路径穿越】那条防线的钉子: 版本号可能从外部收进来当目录名,
// 格式整串锁死之后 ".." 和 "/" 在语法上就进不来。
// ★能比较 ≠ IsValid: 比较器认任何 (带 v、完整三段的) semver, IsValid 只认本库格式。
func TestSharedCases(t *testing.T) {
	data, err := ioutil.ReadFile(filepath.Join("testdata", "cases.json"))
	testOk(t, err == nil, "读 testdata/cases.json 失败")
	var cases testCases_t
	testOk(t, json.Unmarshal(data, &cases) == nil, "解析 testdata/cases.json 失败")
	testOk(t, len(cases.IsValid) > 0 && len(cases.NotComparable) > 0 && len(cases.CompareOrder) > 0 && len(cases.OrderChain) > 0, "用例为空")
	for _, c := range cases.IsValid {
		testOk(t, IsValid(c.Version) == c.Want, fmt.Sprintf("IsValid(%q) 应为 %v: %s", c.Version, c.Want, c.Note))
		if c.Want {
			_, ok := CompareOrder(verA, c.Version)
			testOk(t, ok, "合法版本号应能比较: "+c.Note)
		}
	}
	for _, c := range cases.NotComparable {
		v := testExpandCase(c.Version)
		testOk(t, !IsValid(v), "比不了的串不应 IsValid: "+c.Note)
		// 看不懂 -> 比不出来, 也不升 (看不懂的时候不动比乱动安全)。
		_, ok := CompareOrder(verA, v)
		testOk(t, !ok, fmt.Sprintf("CompareOrder(verA, %q) 应 ok=false: %s", v, c.Note))
		_, ok = CompareOrder(v, verA)
		testOk(t, !ok, fmt.Sprintf("CompareOrder(%q, verA) 应 ok=false: %s", v, c.Note))
		_, ok = CompareOrder(v, v)
		testOk(t, !ok, fmt.Sprintf("CompareOrder(%q, 自己) 应 ok=false: %s", v, c.Note))
		testOk(t, !IsUpgrade(verA, v), "看不懂的目标版本不许升: "+c.Note)
		testOk(t, !IsUpgrade(v, verA), "当前版本看不懂时不许升: "+c.Note)
	}
	for _, c := range cases.CompareOrder {
		a, b := testExpandCase(c.A), testExpandCase(c.B)
		cmp, ok := CompareOrder(a, b)
		testOk(t, cmp == c.Cmp && ok == c.Ok, fmt.Sprintf("CompareOrder(%q, %q) = (%d, %v), 应为 (%d, %v): %s", a, b, cmp, ok, c.Cmp, c.Ok, c.Note))
		testOk(t, IsUpgrade(a, b) == (c.Ok && c.Cmp < 0), fmt.Sprintf("IsUpgrade(%q, %q) 和 CompareOrder 对不上: %s", a, b, c.Note))
	}
	for _, c := range cases.OrderChain {
		for i, a := range c.List {
			for j, b := range c.List {
				want := 0
				if i < j {
					want = -1
				} else if i > j {
					want = 1
				}
				cmp, ok := CompareOrder(a, b)
				testOk(t, ok && cmp == want, fmt.Sprintf("CompareOrder(%q, %q) = (%d, %v), 应为 (%d, true): %s", a, b, cmp, ok, want, c.Note))
			}
		}
	}
}

// ★比较器零分配: 自更新 / 排序会频繁调它。本库格式、外来 semver、非法串各一。
func TestCompareOrderZeroAlloc(t *testing.T) {
	n := testing.AllocsPerRun(100, func() {
		CompareOrder(verA, verANew)
		CompareOrder(verA, verADup)
		CompareOrder("v1.0.0-alpha.beta.11+build.7", "v1.0.0-alpha.beta.2")
		CompareOrder("dev", verA)
		IsUpgrade(verA, verANew)
		IsValid(verA)
	})
	testEqual(t, n, float64(0))
}

// 快路径要不要写看这里的数字 (见 doc/design.md)。
func BenchmarkCompareOrder(b *testing.B) {
	b.Run("own", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			CompareOrder(verA, verANew)
		}
	})
	b.Run("ownSameSecond", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			CompareOrder(verA, verADup)
		}
	})
	b.Run("ownVsForeign", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			CompareOrder(verA, "v0.0.1-rc.1+build.5")
		}
	})
	b.Run("foreign", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			CompareOrder("v1.0.0-alpha.beta.11", "v1.0.0-alpha.beta.2")
		}
	})
	b.Run("invalid", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			CompareOrder(verA, "v0.0.1-260901140715-a1b2c3d4e5-r\n")
		}
	})
	b.Run("isValid", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			IsValid(verA)
		}
	})
}

// TS 实现跑同一份用例。本机没有 node (或 node 太老, 不能直接跑 .ts) 时跳过。
func TestTsImpl(t *testing.T) {
	nodePath, err := exec.LookPath("node")
	if err != nil {
		t.Skip("本机没有 node, 跳过 TS 实现的测试")
	}
	out, err := exec.Command(nodePath, "-e", "process.exit(typeof process.features.typescript === 'string' ? 0 : 1)").CombinedOutput()
	if err != nil {
		t.Skip("node 不能直接跑 .ts (要 node 22.18 / 23.6 以上), 跳过 TS 实现的测试: " + string(out))
	}
	out, err = exec.Command(nodePath, filepath.Join("ts", "hgmBuildVersion_test.ts")).CombinedOutput()
	testOk(t, err == nil, "TS 实现的测试失败:\n"+string(out))
}

// GetCommitId 取出的就是版本号里那 10 位; 非法版本号取不出。
func TestGetCommitId(t *testing.T) {
	id, ok := GetCommitId(verA)
	testOk(t, ok, "合法版本号应取得出 commitId")
	testEqual(t, id, "a1b2c3d4e5")
	id, ok = GetCommitId(verADup)
	testOk(t, ok, "合法版本号应取得出 commitId")
	testEqual(t, id, "ffffffffff")
	// commitId 全是数字且以 0 开头: 照样合法 (换成 '-' 分隔就是为了它, 见 validVersionSuffix)。
	id, ok = GetCommitId("v0.0.1-260901140715-0123456789-r")
	testOk(t, ok, "全数字 commitId 应判合法")
	testEqual(t, id, "0123456789")
	// 合法 semver 但不是本库格式 (含旧格式) 也取不出: commitId 只从本库格式里取。
	for _, bad := range []string{"", "dev", "v0.0.1-260901140715-a1b2c3d4e5", "v0.0.1-260901140715-../etc/pw-r",
		"v1.2.3", "v1.2.3-a1b2c3d4e5", "v0.1.260901140715-a1b2c3d4e5-r"} {
		id, ok = GetCommitId(bad)
		testOk(t, !ok && id == "", "非法版本号不应取出 commitId: "+bad)
	}
}

// makeVersion 是格式的生成侧真源: 拼出来的东西必须能被本文件的解析侧原样收回去。
func TestMakeVersion(t *testing.T) {
	// 完整 sha 自动截前 10 位。
	testEqual(t, makeVersion("v0.0.1", "261111123244", "e64c21ea0512345678901234567890abcdefabcd", false), "v0.0.1-261111123244-e64c21ea05-r")
	testEqual(t, makeVersion("v0.0.1", "260901140715", "a1b2c3d4e5", false), verA)
	// 脏了 -> -d
	testEqual(t, makeVersion("v0.0.1", "260901140715", "a1b2c3d4e5", true), "v0.0.1-260901140715-a1b2c3d4e5-d")
	testOk(t, IsValid(makeVersion("v12.34.56", "260901140715", "a1b2c3d4e5", true)), "拼出来的应当自洽")

	// 前缀写错 -> 拼出来判非法 (打包口据此当场炸, 而不是等到部署后升不了级)。
	testOk(t, !IsValid(makeVersion("0.0.1", "260901140715", "a1b2c3d4e5", false)), "少了 v 应判非法")
	testOk(t, !IsValid(makeVersion("v0.1", "260901140715", "a1b2c3d4e5", false)), "前缀两段应判非法")
	testOk(t, !IsValid(makeVersion("v0.0.1.2", "260901140715", "a1b2c3d4e5", false)), "前缀四段应判非法")
	testOk(t, !IsValid(makeVersion("vx", "260901140715", "a1b2c3d4e5", false)), "前缀带字母应判非法")
}

// ★字符集: 合法版本号里只能出现 RFC 3986 的 unreserved character。
// 它会进 URL、进 shell 脚本、进文件名和目录名 —— 这条塌了, 那几处全要重新论证。
func TestVersionCharsetIsUrlSafe(t *testing.T) {
	for _, v := range []string{verA, verADup, verANew} {
		for i := 0; i < len(v); i++ {
			c := v[i]
			ok := (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
				c == '.' || c == '-' || c == '_' || c == '~'
			testOk(t, ok, "版本号 "+v+" 里出现了非 unreserved 字符 "+string(rune(c))+
				" —— 它会进 URL/shell/文件名, 不许引入需要转义的字符 (见 version.go 文件头)")
		}
	}
}

package hgmBuildVersion

// version_test.go — 格式 (makeVersion / IsValid)、比序 (IsUpgrade / CompareOrder)。

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os/exec"
	"path/filepath"
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
	verA    = "v0.1.260901140715-a1b2c3d4e5-r" // 基准
	verANew = "v0.1.260901140716-a1b2c3d4e5-r" // 只比 verA 晚一秒
	verADup = "v0.1.260901140715-ffffffffff-d" // 与 verA 【序相同身份不同】
)

// testCases_t testdata/cases.json: Go 和 TS (ts/hgmBuildVersion_test.ts) 共用的用例。
type testCases_t struct {
	IsValid []struct {
		Version string
		Want    bool
		Note    string
	}
	CompareOrder []struct {
		A    string
		B    string
		Cmp  int
		Ok   bool
		Note string
	}
}

// 共用用例: 格式校验 + 比序。
//
// ★IsValid 那张表同时是【路径穿越】那条防线的钉子: 版本号可能从外部收进来当目录名,
// 格式整串锁死之后 ".." 和 "/" 在语法上就进不来 (见 version.go 的 validVersionSuffix)。
// ★后两段 (commitId / 脏否) 【不参与比大小】: 同一秒打的两个不同 commit 的包序相等, 互相都不算升级。
func TestSharedCases(t *testing.T) {
	data, err := ioutil.ReadFile(filepath.Join("testdata", "cases.json"))
	testOk(t, err == nil, "读 testdata/cases.json 失败")
	var cases testCases_t
	testOk(t, json.Unmarshal(data, &cases) == nil, "解析 testdata/cases.json 失败")
	testOk(t, len(cases.IsValid) > 0 && len(cases.CompareOrder) > 0, "用例为空")
	for _, c := range cases.IsValid {
		testOk(t, IsValid(c.Version) == c.Want, fmt.Sprintf("IsValid(%q) 应为 %v: %s", c.Version, c.Want, c.Note))
		if c.Want {
			continue
		}
		// 任何一边格式不合法 -> 比不出来, 也不升 (看不懂的时候不动比乱动安全)。
		_, ok := CompareOrder(verA, c.Version)
		testOk(t, !ok, "非法版本号不应比得出序: "+c.Note)
		_, ok = CompareOrder(c.Version, verA)
		testOk(t, !ok, "非法版本号不应比得出序: "+c.Note)
		testOk(t, !IsUpgrade(verA, c.Version), "非法目标版本不许升: "+c.Note)
		testOk(t, !IsUpgrade(c.Version, verA), "当前版本非法时不许升: "+c.Note)
	}
	for _, c := range cases.CompareOrder {
		cmp, ok := CompareOrder(c.A, c.B)
		testOk(t, cmp == c.Cmp && ok == c.Ok, fmt.Sprintf("CompareOrder(%q, %q) = (%d, %v), 应为 (%d, %v): %s", c.A, c.B, cmp, ok, c.Cmp, c.Ok, c.Note))
		testOk(t, IsUpgrade(c.A, c.B) == (c.Ok && c.Cmp < 0), fmt.Sprintf("IsUpgrade(%q, %q) 和 CompareOrder 对不上: %s", c.A, c.B, c.Note))
	}
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
	id, ok = GetCommitId("v0.1.260901140715-0123456789-r")
	testOk(t, ok, "全数字 commitId 应判合法")
	testEqual(t, id, "0123456789")
	for _, bad := range []string{"", "dev", "v0.1.260901140715-a1b2c3d4e5", "v0.1.260901140715-../etc/pw-r"} {
		id, ok = GetCommitId(bad)
		testOk(t, !ok && id == "", "非法版本号不应取出 commitId: "+bad)
	}
}

// makeVersion 是格式的生成侧真源: 拼出来的东西必须能被本文件的解析侧原样收回去。
func TestMakeVersion(t *testing.T) {
	// 完整 sha 自动截前 10 位。
	testEqual(t, makeVersion("v0.1", "260901140715", "a1b2c3d4e51234567890abcdef", false), verA)
	testEqual(t, makeVersion("v0.1", "260901140715", "a1b2c3d4e5", false), verA)
	// 脏了 -> -d
	testEqual(t, makeVersion("v0.1", "260901140715", "a1b2c3d4e5", true), "v0.1.260901140715-a1b2c3d4e5-d")
	testOk(t, IsValid(makeVersion("v0.1", "260901140715", "a1b2c3d4e5", true)), "拼出来的应当自洽")

	// 前缀写错 -> 拼出来判非法 (打包口据此当场炸, 而不是等到部署后升不了级)。
	testOk(t, !IsValid(makeVersion("0.1", "260901140715", "a1b2c3d4e5", false)), "少了 v 应判非法")
	testOk(t, !IsValid(makeVersion("v0.1.2", "260901140715", "a1b2c3d4e5", false)), "前缀三段应判非法")
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

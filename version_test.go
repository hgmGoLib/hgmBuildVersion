package hgmBuildVersion

// version_test.go — 格式 (makeVersion / IsValid)、比序 (IsUpgrade)。

import (
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
	verA    = "v0.1.260901140715-8d8358905d.r" // 基准
	verANew = "v0.1.260901140716-8d8358905d.r" // 只比 verA 晚一秒
	verADup = "v0.1.260901140715-ffffffffff.d" // 与 verA 【序相同身份不同】
)

// 【序】= 前三段 uint64 从前往后比: 前面的段不同就以它定胜负, 相等才看下一段; 序全等 = 同版本, 不升。
func TestIsUpgradeComparesOrderOnly(t *testing.T) {
	// 第三段 (打包时间戳) 递增 —— 日常唯一会变的那一段。
	testEqual(t, IsUpgrade(verA, verANew), true)
	testEqual(t, IsUpgrade(verANew, verA), false) // 降级不升
	testEqual(t, IsUpgrade(verA, verA), false)    // 同版本不升

	// 前两段 (versionPrefix.txt) 优先级更高: 段1 更大就算更新, 哪怕时间戳更早。
	testEqual(t, IsUpgrade("v0.9.269999999999-aaaaaaaaaa.r", "v1.0.260101000000-bbbbbbbbbb.r"), true)
	testEqual(t, IsUpgrade("v1.0.260101000000-bbbbbbbbbb.r", "v0.9.269999999999-aaaaaaaaaa.r"), false)
	// 段1 相等则看段2。
	testEqual(t, IsUpgrade(verA, "v0.2.260101000000-aaaaaaaaaa.r"), true)
	testEqual(t, IsUpgrade("v0.2.260101000000-aaaaaaaaaa.r", "v0.1.269999999999-aaaaaaaaaa.r"), false)

	// 不是按字符串比: "10" > "9" 在数字上成立, 在字典序上不成立。
	testEqual(t, IsUpgrade("v0.9.260901140715-aaaaaaaaaa.r", "v0.10.260901140715-aaaaaaaaaa.r"), true)
}

// ★后两段 (commitId / 脏否) 【不参与比大小】—— 这正是把它们放在序后面的意义。
// 同一秒打的两个不同 commit 的包互相都不算升级; 脏包也顶不掉同一秒的干净包 (那件事得在打包口拦)。
func TestIsUpgradeIgnoresIdentitySuffix(t *testing.T) {
	testOk(t, verA != verADup, "这两个串本来就该不相等 (身份不同), 否则下面的断言没有意义")
	testEqual(t, IsUpgrade(verA, verADup), false)
	testEqual(t, IsUpgrade(verADup, verA), false)
	// 但只要序更新, 后缀怎么变都算升级。
	testEqual(t, IsUpgrade(verADup, verANew), true)
}

// 任何一边格式不合法 -> 不升 (看不懂的时候不动比乱动安全), 且取序取不出东西。
//
// ★这张表同时是【路径穿越】那条防线的钉子: 版本号可能从外部收进来当目录名,
// 格式整串锁死之后 ".." 和 "/" 在语法上就进不来 (见 version.go 的 validVersionSuffix)。
func TestIsUpgradeRefusesMalformed(t *testing.T) {
	for _, bad := range []string{
		"", "dev", "260901140715", // 根本不是版本号
		"v0.1", "v0.1.2.3", // 序的段数不对
		"v0.1.260901140715", "v0.10.269999999999", // 缺身份后缀
		"v0.1.", "v.1.2", // 空段
		"v0.1.26090114071a", "v0.1. 260901140715", "v0.1.+260901140715", // 序里有非数字
		"v0.1.26090114071", "v0.1.2609011407150", // 时间戳不是 12 位
		"v0.01.260901140715-8d8358905d.r", // 段2 前导零 (同一版本会有两种写法)
		"v0.99999999999999999999999.260901140715-8d8358905d.r", // 段2 溢出 uint64
		"0.1.260901140715-8d8358905d.r",                        // 少了 v

		// ── 身份后缀那半: 恰好 <10位小写hex>.<r|d>, 差一点都不行 ──
		"v0.1.260901140715-",               // 后缀为空
		"v0.1.260901140715-8d8358905d",     // 少了脏否那段
		"v0.1.260901140715-8d8358905d.",    // 脏否为空
		"v0.1.260901140715-8d8358905d.x",   // 脏否不是 r/d
		"v0.1.260901140715-8d8358905d.rd",  // 脏否多一个字符
		"v0.1.260901140715-8d8358905.r",    // commitId 只有 9 位
		"v0.1.260901140715-8d8358905dd.r",  // commitId 11 位
		"v0.1.260901140715-8D8358905D.r",   // commitId 大写 (只认小写 hex, 免得同一个包两种写法)
		"v0.1.260901140715-8d8358905g.r",   // commitId 不是 hex
		"v0.1.260901140715-8d8358905d.r.d", // 后缀三段

		// ── 拿去当路径片段的危险形态: 必须一个都进不来 ──
		"../../etc", "v0/1/2", "v0.1.26/../..",
		"v0.1.260901140715-..",
		"v0.1.260901140715-8d8358905d..r", // 空段 (".." 的来源)
		"v0.1.260901140715-../etc/pw.r",
		"v0.1.260901140715-8d8358905d.r/x",
		"v0.1.260901140715-8d8358905d.r\x00", // 尾部有 NUL
	} {
		testOk(t, !IsValid(bad), "应判非法: "+bad)
		testOk(t, !IsUpgrade(verA, bad), "非法目标版本不许升: "+bad)
		testOk(t, !IsUpgrade(bad, verA), "当前版本非法时不许升: "+bad)
	}
	testOk(t, IsValid(verA), "正常版本号应判合法")
}

// GetCommitId 取出的就是版本号里那 10 位; 非法版本号取不出。
func TestGetCommitId(t *testing.T) {
	id, ok := GetCommitId(verA)
	testOk(t, ok, "合法版本号应取得出 commitId")
	testEqual(t, id, "8d8358905d")
	id, ok = GetCommitId(verADup)
	testOk(t, ok, "合法版本号应取得出 commitId")
	testEqual(t, id, "ffffffffff")
	for _, bad := range []string{"", "dev", "v0.1.260901140715-8d8358905d", "v0.1.260901140715-../etc/pw.r"} {
		id, ok = GetCommitId(bad)
		testOk(t, !ok && id == "", "非法版本号不应取出 commitId: "+bad)
	}
}

// makeVersion 是格式的生成侧真源: 拼出来的东西必须能被本文件的解析侧原样收回去。
func TestMakeVersion(t *testing.T) {
	// 完整 sha 自动截前 10 位。
	testEqual(t, makeVersion("v0.1", "260901140715", "8d8358905d1234567890abcdef", false), verA)
	testEqual(t, makeVersion("v0.1", "260901140715", "8d8358905d", false), verA)
	// 脏了 -> .d
	testEqual(t, makeVersion("v0.1", "260901140715", "8d8358905d", true), "v0.1.260901140715-8d8358905d.d")
	testOk(t, IsValid(makeVersion("v0.1", "260901140715", "8d8358905d", true)), "拼出来的应当自洽")

	// 前缀写错 -> 拼出来判非法 (打包口据此当场炸, 而不是等到部署后升不了级)。
	testOk(t, !IsValid(makeVersion("0.1", "260901140715", "8d8358905d", false)), "少了 v 应判非法")
	testOk(t, !IsValid(makeVersion("v0.1.2", "260901140715", "8d8358905d", false)), "前缀三段应判非法")
	testOk(t, !IsValid(makeVersion("vx", "260901140715", "8d8358905d", false)), "前缀带字母应判非法")
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

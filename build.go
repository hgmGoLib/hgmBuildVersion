package hgmBuildVersion

// build.go — 打包侧: 生成本次打包的版本号。
//
// ★这里是【唯一】从零生成版本号的地方 (AddTimeSecond 只挪已有版本号的时间戳)。同一次打包出来的所有二进制必须共用同一个版本号:
// 调用方只调一次 NewBuildVersion, 然后把结果传给每个 target 的 go build。
// 怎么注入进二进制 / 运行期怎么读, 本包不管 (现成的做法见子包 hgmBuildVersionCurrent)。

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// GBuildTimeZoneOffsetSecond 版本号时间戳用的时区, 相对 UTC 的固定偏移秒数。默认 8*3600 = UTC+8。
// 要用别的时区 (例 UTC 设成 0): 在打包程序里、调 NewBuildVersion 之前设一次。
//
// ★只给固定偏移, 不接受地名时区 (*time.Location): 带夏令时的时区在回拨那一小时里时间戳会倒退,
// 后打的包反而不算升级。固定偏移也不依赖打包机上有没有 tzdata。
//
// ★时区不写进版本号, 而时间戳决定新旧: 一个项目定下来之后就【不要再改】。
// 非改不可时 (例 UTC+8 改成 UTC): 改完后那 8 小时内打的包时间戳比改之前的小, 不算升级;
// 所以必须同时把 versionPrefix.txt 的版本号升一级顶过去。
var GBuildTimeZoneOffsetSecond = 8 * 3600

// NewBuildVersion 读 versionPrefixPath + 取当前时间 (时区见 GBuildTimeZoneOffsetSecond) + 问 git 要 commitId/脏否, 生成本次打包的版本号。
//
// versionPrefixPath 版本号前缀文件 (一般是 <仓库根>/versionPrefix.txt) 的路径, 由调用方给。
// ★本包不假设当前目录在哪: git 也在这个文件所在的目录里跑, 取的是【这个文件所在仓库】的 HEAD/脏否。
// ★需要本机有 git 命令且该文件已被 git 跟踪: 没有 git (例如从源码压缩包构建) / 文件没 git add / 被 .gitignore 掉 直接返回 error, 不猜。
//
// ★工作区脏的时候【照样打】, 只是版本号末尾变成 -d: 开发期工作区基本总是脏的,
// 在这里拦一刀等于没法测试。"-d" 只给人事后溯源看 —— 只在同一秒同一 commit 时影响先后 (semver 按 ASCII 比, d < r), 拦不住任何事。
// 脏否用 git status --porcelain 判: 没被 .gitignore 掉的未跟踪文件也算脏。
//
// 升级测试里同一秒连打几个包要拉开序: 拿结果去 AddTimeSecond (见 timestamp.go)。
func NewBuildVersion(versionPrefixPath string) (string, error) {
	data, err := ioutil.ReadFile(versionPrefixPath)
	if err != nil {
		return "", fmt.Errorf("read version prefix %s failed: %w", versionPrefixPath, err)
	}
	prefix := strings.TrimSpace(string(data))
	if prefix == "" {
		return "", fmt.Errorf("%s is empty, it should contain the version prefix, e.g. v0.0.1", versionPrefixPath)
	}
	// 前缀先校验再跑 git: 写错了不用等 git, 报错信息也更准。
	err = checkVersionPrefix(prefix)
	if err != nil {
		return "", fmt.Errorf("%s: %w", versionPrefixPath, err)
	}
	gitDir := filepath.Dir(versionPrefixPath)
	out, err := runGit(gitDir, "rev-parse", "HEAD")
	if err != nil {
		return "", fmt.Errorf("get git commitId failed: %w", err)
	}
	commitId := strings.TrimSpace(out)
	// ★前缀文件本身必须被 git 跟踪: git status 不报被 ignore 的文件, 被 ignore 时照样会出 -r;
	// 放在没有 .git 的被 ignore 子目录里时, git 往上找到的是外层仓库, commitId 也是外层的。
	// --literal-pathspecs: 文件名按字面匹配, 否则 "v[ab].txt" 会被当通配模式, 匹配到别的已跟踪文件就误放过。
	_, err = runGit(gitDir, "--literal-pathspecs", "ls-files", "--error-unmatch", "--", filepath.Base(versionPrefixPath))
	if err != nil {
		return "", fmt.Errorf("%s is not tracked by git (not git added, or ignored by .gitignore); the version prefix must be committed to git: %w", versionPrefixPath, err)
	}
	st, err := runGit(gitDir, "status", "--porcelain")
	if err != nil {
		return "", fmt.Errorf("get git status failed: %w", err)
	}
	dirty := strings.TrimSpace(st) != ""
	stamp := time.Now().In(time.FixedZone("", GBuildTimeZoneOffsetSecond)).Format("060102150405")
	version := makeVersion(prefix, stamp, commitId, dirty)
	// 拼完立刻校一遍: 前缀写错 (段数不对/带字母) 在这里就炸, 而不是等到部署后升不了级。
	if !IsValid(version) {
		return "", fmt.Errorf("assembled version %q is invalid: check the content of %s, it should look like v0.0.1 (three decimal parts, no leading zeros)",
			version, versionPrefixPath)
	}
	return version, nil
}

// checkVersionPrefix 前缀必须是 v<a>.<b>.<c> (三段十进制, 不许前导零)。
//
// ★两段 (v0.0.6 及以前的格式, 例 v0.19) 直接报错, 不自动补成 v0.19.0:
// 旧格式的包是 v0.19.<时间戳>-..., 在 semver 里比 v0.19.0-... 大, 自动补 .0 之后新包反而比所有旧包 "旧" ——
// 新的服务端判 "不算升级", 部署被静默挡住; 新二进制看到旧包还会当成升级降回去。
// 报错信息直接给出能用的值: 第二段 +1 再补 .0 (v0.19 -> v0.20.0, 比所有旧格式的包都新);
// 旧格式的包都清理干净了的话也可以从 v0.0.1 重新开始。
func checkVersionPrefix(prefix string) error {
	p, ok := parseSemver(prefix)
	if ok && p.pre == "" && !p.hasBuild {
		return nil
	}
	if a, b, ok := splitTwoPartPrefix(prefix); ok {
		suggest := "v" + a + "." + incDecimal(b) + ".0"
		return fmt.Errorf("version prefix %q is the old two-part format, the prefix must have three parts now: "+
			"change it to %s (newer than every package built with the old %s.<timestamp> format), "+
			"or start again from v0.0.1 if no package of the old format is still in use",
			prefix, suggest, prefix)
	}
	return fmt.Errorf("version prefix %q is invalid, it should look like v0.0.1 (three decimal parts, no leading zeros)", prefix)
}

// splitTwoPartPrefix "v<a>.<b>" (两段十进制, 不许前导零) -> (a, b, true)。
func splitTwoPartPrefix(prefix string) (string, string, bool) {
	if len(prefix) < 2 || prefix[0] != 'v' {
		return "", "", false
	}
	a, i, ok := scanCoreNum(prefix, 1)
	if !ok || i >= len(prefix) || prefix[i] != '.' {
		return "", "", false
	}
	b, i, ok := scanCoreNum(prefix, i+1)
	if !ok || i != len(prefix) {
		return "", "", false
	}
	return a, b, true
}

// incDecimal 十进制数字串 +1 (不限位数): "19" -> "20", "99" -> "100"。
func incDecimal(s string) string {
	buf := []byte(s)
	for i := len(buf) - 1; i >= 0; i-- {
		if buf[i] < '9' {
			buf[i]++
			return string(buf)
		}
		buf[i] = '0'
	}
	return "1" + string(buf)
}

// runGit 在 dir 里跑一条 git 命令, 返回 stdout。
// ★失败时把 git 的 stderr 带进 error: 只有 exit status 128 看不出原因
// (不在仓库里 / dubious ownership (safe.directory) / 空仓库没有 HEAD 都是 128)。
func runGit(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("run git %[2]s in %[1]s failed: %[3]w: %[4]s", dir, strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return string(out), nil
}

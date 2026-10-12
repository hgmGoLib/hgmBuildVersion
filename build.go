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
// 在这里拦一刀等于没法测试。"-d" 只给人事后溯源看 —— 它不参与比大小, 也拦不住任何事。
// 脏否用 git status --porcelain 判: 没被 .gitignore 掉的未跟踪文件也算脏。
//
// 升级测试里同一秒连打几个包要拉开序: 拿结果去 AddTimeSecond (见 timestamp.go)。
func NewBuildVersion(versionPrefixPath string) (string, error) {
	data, err := ioutil.ReadFile(versionPrefixPath)
	if err != nil {
		return "", fmt.Errorf("读版本号前缀 %s 失败: %w", versionPrefixPath, err)
	}
	prefix := strings.TrimSpace(string(data))
	if prefix == "" {
		return "", fmt.Errorf("%s 是空的, 里面应该是版本号前缀, 例 v0.1", versionPrefixPath)
	}
	gitDir := filepath.Dir(versionPrefixPath)
	out, err := runGit(gitDir, "rev-parse", "HEAD")
	if err != nil {
		return "", fmt.Errorf("取 git commitId 失败: %w", err)
	}
	commitId := strings.TrimSpace(out)
	// ★前缀文件本身必须被 git 跟踪: git status 不报被 ignore 的文件, 被 ignore 时照样会出 -r;
	// 放在没有 .git 的被 ignore 子目录里时, git 往上找到的是外层仓库, commitId 也是外层的。
	// --literal-pathspecs: 文件名按字面匹配, 否则 "v[ab].txt" 会被当通配模式, 匹配到别的已跟踪文件就误放过。
	_, err = runGit(gitDir, "--literal-pathspecs", "ls-files", "--error-unmatch", "--", filepath.Base(versionPrefixPath))
	if err != nil {
		return "", fmt.Errorf("%s 没被 git 跟踪 (没 git add, 或被 .gitignore 掉了), 版本号前缀必须提交进 git: %w", versionPrefixPath, err)
	}
	st, err := runGit(gitDir, "status", "--porcelain")
	if err != nil {
		return "", fmt.Errorf("取 git status 失败: %w", err)
	}
	dirty := strings.TrimSpace(st) != ""
	stamp := time.Now().In(time.FixedZone("", GBuildTimeZoneOffsetSecond)).Format("060102150405")
	version := makeVersion(prefix, stamp, commitId, dirty)
	// 拼完立刻校一遍: 前缀写错 (段数不对/带字母) 在这里就炸, 而不是等到部署后升不了级。
	if !IsValid(version) {
		return "", fmt.Errorf("拼出来的版本号 %q 不合法: 检查 %s 的内容, 应形如 v0.1 (两段纯数字, 不许前导零)",
			version, versionPrefixPath)
	}
	return version, nil
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
		return "", fmt.Errorf("在 %s 跑 git %s 失败: %w: %s", dir, strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return string(out), nil
}

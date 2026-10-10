package hgmBuildVersion

// build_test.go — NewBuildVersion / NewReleaseVersion: 在系统临时目录真建 git 仓库来跑 (需要本机有 git)。

import (
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// testGitRepo_t 系统临时目录里的一个真 git 仓库: 已提交一份 versionPrefix.txt (内容 v0.1)。
type testGitRepo_t struct {
	t          *testing.T
	Dir        string
	PrefixPath string
	CommitId   string // HEAD 的前 versionCommitIdLen 位
}

func newTestGitRepo(t *testing.T) *testGitRepo_t {
	dir, err := ioutil.TempDir("", "hgmBuildVersionTest")
	if err != nil {
		t.Fatal(err)
	}
	r := &testGitRepo_t{t: t, Dir: dir, PrefixPath: filepath.Join(dir, "versionPrefix.txt")}
	r.Git("init", "-q")
	testOk(t, ioutil.WriteFile(r.PrefixPath, []byte("v0.1\n"), 0644) == nil, "write versionPrefix.txt")
	r.Git("add", "versionPrefix.txt")
	r.Git("commit", "-q", "-m", "init")
	r.CommitId = r.Git("rev-parse", "HEAD")[:versionCommitIdLen]
	return r
}

// Git 在仓库目录里跑一条 git 命令, 失败即 t.Fatal。
func (r *testGitRepo_t) Git(args ...string) string {
	r.t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=test", "-c", "user.email=test@example.com", "-c", "commit.gpgsign=false"}, args...)...)
	cmd.Dir = r.Dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %v 失败: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestNewBuildVersion(t *testing.T) {
	r := newTestGitRepo(t)
	defer os.RemoveAll(r.Dir)

	testEqual(t, GBuildTimeZoneOffsetSecond, 8*3600) // 默认 UTC+8 是需求, 不许改
	defer func(old int) { GBuildTimeZoneOffsetSecond = old }(GBuildTimeZoneOffsetSecond)
	for _, offset := range []int{0, 8 * 3600, -5 * 3600} {
		GBuildTimeZoneOffsetSecond = offset
		ver, err := NewBuildVersion(r.PrefixPath)
		testOk(t, err == nil, "NewBuildVersion 失败")
		testOk(t, IsValid(ver), "生成的版本号应合法: "+ver)
		testOk(t, strings.HasSuffix(ver, "-"+r.CommitId+".r"), "干净工作区应以 -<commitId>.r 结尾: "+ver)
		// 时间戳按给定偏移解析回来, 应当就是现在。
		stamp := strings.TrimPrefix(ver[:strings.IndexByte(ver, '-')], "v0.1.")
		tm, err := time.ParseInLocation("060102150405", stamp, time.FixedZone("", offset))
		testOk(t, err == nil, "时间戳解析失败: "+stamp)
		diff := time.Since(tm)
		testOk(t, diff > -time.Second && diff < time.Minute, "时间戳和当前时间对不上 (时区偏移没生效?): "+ver)
	}

	// 未跟踪文件也算脏 -> .d
	testOk(t, ioutil.WriteFile(filepath.Join(r.Dir, "untracked.txt"), []byte("x"), 0644) == nil, "write untracked")
	ver, err := NewBuildVersion(r.PrefixPath)
	testOk(t, err == nil, "NewBuildVersion 失败")
	testOk(t, strings.HasSuffix(ver, "-"+r.CommitId+".d"), "有未跟踪文件应以 .d 结尾: "+ver)

	// 前缀写错 -> 打包口当场报错。
	testOk(t, ioutil.WriteFile(r.PrefixPath, []byte("v0.1.2"), 0644) == nil, "write bad prefix")
	_, err = NewBuildVersion(r.PrefixPath)
	testOk(t, err != nil, "前缀三段应返回 error")
}

// NewReleaseVersion: 没推送 -> error; 推送了但工作区脏 -> error; 干净且已推送 -> .r 版本号。
func TestNewReleaseVersion(t *testing.T) {
	r := newTestGitRepo(t)
	defer os.RemoveAll(r.Dir)

	_, err := NewReleaseVersion(r.PrefixPath)
	testOk(t, err != nil, "没有远端 (HEAD 没推送) 应返回 error")

	// 真建一个本地 bare 仓库当远端, 推上去 (推送会顺带更新 refs/remotes/origin/main)。
	remoteDir := filepath.Join(r.Dir, "remote.git")
	r.Git("init", "-q", "--bare", remoteDir)
	r.Git("remote", "add", "origin", remoteDir)
	// remote.git 在工作区里, 本身就会让工作区变脏: 让 git 忽略它。
	testOk(t, ioutil.WriteFile(filepath.Join(r.Dir, ".git", "info", "exclude"), []byte("/remote.git/\n"), 0644) == nil, "write exclude")
	r.Git("push", "-q", "origin", "HEAD:refs/heads/main")

	ver, err := NewReleaseVersion(r.PrefixPath)
	testOk(t, err == nil, "干净且已推送应成功")
	testOk(t, IsValid(ver) && strings.HasSuffix(ver, "-"+r.CommitId+".r"), "应是 .r 版本号: "+ver)

	// 本地新提交一个没推送的 commit -> error
	testOk(t, ioutil.WriteFile(filepath.Join(r.Dir, "a.txt"), []byte("a"), 0644) == nil, "write a.txt")
	r.Git("add", "a.txt")
	r.Git("commit", "-q", "-m", "local only")
	_, err = NewReleaseVersion(r.PrefixPath)
	testOk(t, err != nil, "HEAD 没推送应返回 error")
	r.Git("push", "-q", "origin", "HEAD:refs/heads/main")

	// 已推送但工作区脏 -> error; NewBuildVersion 照样能打 (.d)
	testOk(t, ioutil.WriteFile(filepath.Join(r.Dir, "untracked.txt"), []byte("x"), 0644) == nil, "write untracked")
	_, err = NewReleaseVersion(r.PrefixPath)
	testOk(t, err != nil, "工作区脏应返回 error")
	ver, err = NewBuildVersion(r.PrefixPath)
	testOk(t, err == nil && strings.HasSuffix(ver, ".d"), "NewBuildVersion 脏工作区应照样打出 .d: "+ver)
}

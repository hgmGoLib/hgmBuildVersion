package hgmBuildVersionCurrent

import (
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// 真编一个 import 了本包的小程序, 用 GetVersionVarFullName 注入, 跑起来看 GetCurrentVersion 读到的是不是那一串。
// ★-X 目标写错 go build 是静默忽略的, 只比字符串钉不住这件事, 必须真编真跑。
//
// 小程序放在系统临时目录, 自带 go.mod 用 replace 指回本 module:
// 不往本 module 目录里写东西 (从只读的 module cache 里跑 go test 也行), import path 也不写死 (fork 之后照样对)。
func TestGetVersionVarFullName(t *testing.T) {
	const ver = "v0.1.260901140715-8d8358905d.r"
	pkgPath := strings.TrimSuffix(GetVersionVarFullName(), ".GVersion")
	modPath := strings.TrimSuffix(pkgPath, "/hgmBuildVersionCurrent")
	if modPath == pkgPath {
		t.Fatalf("取不到 module path: %s", pkgPath)
	}
	modDir, err := filepath.Abs("..") // go test 的当前目录 = 本包目录, 上一级 = module 根
	if err != nil {
		t.Fatal(err)
	}
	dir, err := ioutil.TempDir("", "hgmBuildVersionCurrentTest")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	goMod := "module printversion\n\ngo 1.13\n\nrequire " + modPath + " v0.0.0\n\nreplace " + modPath + " => " + modDir + "\n"
	if err := ioutil.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0644); err != nil {
		t.Fatal(err)
	}
	src := "package main\n\nimport (\n\t\"fmt\"\n\n\t\"" + pkgPath + "\"\n)\n\nfunc main() { fmt.Print(hgmBuildVersionCurrent.GetCurrentVersion()) }\n"
	if err := ioutil.WriteFile(filepath.Join(dir, "main.go"), []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	binPath := filepath.Join(dir, "printVersion.bin")
	build := exec.Command("go", "build", "-ldflags", "-X "+GetVersionVarFullName()+"="+ver, "-o", binPath, ".")
	build.Dir = dir
	build.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=")
	out, err := build.CombinedOutput()
	if err != nil {
		t.Fatalf("go build 失败: %v\n%s", err, out)
	}
	got, err := exec.Command(binPath).Output()
	if err != nil {
		t.Fatalf("运行失败: %v", err)
	}
	if string(got) != ver {
		t.Fatalf("got %q want %q", got, ver)
	}
}

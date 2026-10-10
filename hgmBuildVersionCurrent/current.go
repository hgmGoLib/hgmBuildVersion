// Package hgmBuildVersionCurrent holds the version string baked into the binary
// (via go build -ldflags "-X ...", see GetVersionVarFullName) and reads it back at runtime.
//
// 本二进制的版本号: 打包时注入, 运行期用 GetCurrentVersion 读。
package hgmBuildVersionCurrent

// current.go — 版本号烤进二进制用的那个变量 + 运行期读取 (现成的一种做法)。
//
// 父包 hgmBuildVersion 只管生成/校验/比大小, 不管版本号放在哪。本子包给"不想自己声明全局变量"的项目直接用。
// GVersion 两种设置方式, 任选:
//
//	1) go build -ldflags "-X "+GetVersionVarFullName()+"="+ver ...   (被编的 main 只要(间接) import 了本包就生效)
//	2) 代码在程序开始的位置直接赋值 hgmBuildVersionCurrent.GVersion = "..." (例如生成代码写进 main 包)
//
// 只依赖标准库。

import "reflect"

// GVersion 本二进制的版本号。没设置 = 空串。
var GVersion = ""

// GetCurrentVersion 返回 GVersion, 原样返回; 没设置 = 空串。
// 不校验: 拿去做判定的地方 (hgmBuildVersion.IsUpgrade / IsValid) 对非法串和空串一律判 false。
func GetCurrentVersion() string {
	return GVersion
}

// GetVersionVarFullName GVersion 变量的全名 (import path + 变量名), 给 go build -ldflags "-X <它>=<版本号>" 用。
//
// ★-X 的目标写错 go build 不报错, 编出来就是空串; 所以 import path 不写死, 运行期从本包里的类型取:
// 本包被 fork / 换 module path / vendor 之后照样对 (current_test.go 真编真跑钉着)。
func GetVersionVarFullName() string {
	return reflect.TypeOf(pkgPathMarker{}).PkgPath() + ".GVersion"
}

// pkgPathMarker 只用来取本包的 import path。
type pkgPathMarker struct{}

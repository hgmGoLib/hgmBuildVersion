hgmBuildVersionCurrent — 把版本号烤进二进制, 运行期读出来

import path: github.com/hgmGoLib/hgmBuildVersion/hgmBuildVersionCurrent
只依赖标准库.

什么时候用
--------
打包时生成了版本号 (父包 hgmBuildVersion), 想让二进制自己知道"我是哪一版", 又不想在自己的 main 里
声明全局变量 + 记 -X 的全名. 自己已经有一个版本号变量的项目用不着它.

怎么用
--------
打包 (两种, 任选):

    // 1) -ldflags -X. 被编的 main 只要 (间接) import 了本包就生效.
    exec.Command("go", "build",
        "-ldflags", "-X "+hgmBuildVersionCurrent.GetVersionVarFullName()+"="+ver, ...)

    // 2) 不用 -X: 在程序开始的地方直接赋值 (例如生成代码写进 main 包).
    hgmBuildVersionCurrent.GVersion = ver

运行期:

    cur := hgmBuildVersionCurrent.GetCurrentVersion()   // 原样返回; 没注入 = ""

⚠️ -X 的目标写错 go build 不报错, 编出来就是空串. 所以别手写 import path, 用 GetVersionVarFullName
   (它运行期从本包的类型取 import path, fork / 换 module path / vendor 之后照样对, current_test.go 真编真跑钉着).
⚠️ GetCurrentVersion 不校验. 拿去判升级 / 比大小用父包的 IsUpgrade / IsValid, 它们对空串和非法串一律判 false.

更多
--------
版本号格式 · 生成 · 比大小: ../readme.md (父包 hgmBuildVersion).

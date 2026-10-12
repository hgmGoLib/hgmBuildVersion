package hgmBuildVersion

// timestamp.go — 第三段 (打包时间戳 YYMMDDhhmmss) 的读 (GetBuildTime) 和改 (AddTimeSecond)。
//
// ★两位年份一律当 20YY (2000..2099)。不用 time.Parse 的 "06": 它把 69..99 解析成 19xx,
// 2068 年加 1 秒格式化出 "69", 再解析回来就成了 1969 年。

import (
	"fmt"
	"time"
)

// GetBuildTime 取版本号里的打包时刻。时间戳按 GBuildTimeZoneOffsetSecond 解释, 返回值的时区就是这个固定偏移。
// 版本号非法 (含空串) / 时间戳不是真实存在的时刻 (例 13 月) -> (time.Time{}, false)。
//
// ★时区不写进版本号: 调它的进程 (常常是运行期的二进制, 不是打包程序) 里 GBuildTimeZoneOffsetSecond 必须和打包时一样,
// 否则读出来的时刻差几个小时。最省事的办法是全程用默认值不改。
//
// ★用 AddTimeSecond 挪过的版本号, 这里读到的是挪过之后的时刻, 和真实打包时刻差那么多秒。
//
// 只拿去显示 / 存档。比新旧用 IsUpgrade / CompareOrder, 不要比这个时间。
func GetBuildTime(version string) (time.Time, bool) {
	stamp, ok := getVersionStamp(version)
	if !ok {
		return time.Time{}, false
	}
	return parseStamp(stamp, time.FixedZone("", GBuildTimeZoneOffsetSecond))
}

// AddTimeSecond 把版本号的时间戳加 second 秒 (可正可负), 按真实时间进位 (…153059 加 1 秒是 …153100, 不是 …153060)。
// 前缀 / commitId / 脏否原样不动。结果和时区无关 (固定偏移下加减秒数不受偏移影响)。
//
// 返回 error:
//   - version 非法 (含空串);
//   - 时间戳不是真实存在的时刻 (例 13 月);
//   - 结果超出两位年份能表示的范围 (2000..2099)。
//
// 用途: 升级的自动测试里拉开两个包的序 —— 测试会在几秒内连着打好几个包, 常常落在同一秒, 序相等就不算升级:
//   - 同一份代码的升级测试: base 装好后再打 next, next 往后挪到比 base 新;
//   - 从 git 打旧版本再升到新版本的测试: 新包往后挪到比旧版本新;
//   - 拒绝降级的测试: 往前挪, 造一个比已装版本严格更旧的包。
//
//	next, _ := NewBuildVersion("versionPrefix.txt")
//	if !IsUpgrade(base, next) {
//		next, err = AddTimeSecond(next, 1) // next 是 base 之后打的, 时间戳不会比 base 小, 加 1 秒就够
//	}
//
// ★正式打包不用它: 正常打包间隔远大于 1 秒, 靠当前时间就够, 不要在打包流程里自动挪。
// ★挪【本次打出来的】版本号, 不要挪别的包的: commitId 原样保留, 挪别的包的版本号会让新包顶着别人的 commitId, 拿着它找到的是别的代码。
// ★前缀不同时挪时间戳追不上 (前缀优先比)。
// ★挪过的时间戳不再等于打包时刻。
func AddTimeSecond(version string, second int64) (string, error) {
	stamp, ok := getVersionStamp(version)
	if !ok {
		return "", fmt.Errorf("AddTimeSecond: 版本号不合法: %q", version)
	}
	t, ok := parseStamp(stamp, time.UTC)
	if !ok {
		return "", fmt.Errorf("AddTimeSecond: 版本号 %q 的时间戳不是真实存在的时刻", version)
	}
	// 2000..2099 一共约 3.2e9 秒: 先挡掉离谱的 second, 免得 Unix() + second 溢出 int64。
	const maxSecond = 200 * 366 * 24 * 3600
	if second > maxSecond || second < -maxSecond {
		return "", fmt.Errorf("AddTimeSecond: %q 加 %d 秒超出 2000..2099 年", version, second)
	}
	newT := time.Unix(t.Unix()+second, 0).UTC()
	if newT.Year() < 2000 || newT.Year() > 2099 {
		return "", fmt.Errorf("AddTimeSecond: %q 加 %d 秒超出 2000..2099 年 (时间戳只有两位年份)", version, second)
	}
	end := len(version) - versionSuffixLen // 时间戳之后就是 -<commitId>-<r|d>
	return version[:end-versionStampLen] + formatStamp(newT) + version[end:], nil
}

// versionSuffixLen 身份后缀 "-<10位commitId>-<r|d>" 的固定长度。
const versionSuffixLen = 1 + versionCommitIdLen + 2

// getVersionStamp 取合法版本号里的 12 位时间戳; 非法 -> ok=false。
func getVersionStamp(version string) (string, bool) {
	if !IsValid(version) {
		return "", false
	}
	// 合法版本号的结尾定长: <12位时间戳>-<10位commitId>-<r|d>。
	end := len(version) - versionSuffixLen
	return version[end-versionStampLen : end], true
}

// parseStamp 把 12 位数字的时间戳按 20YY 解析成 loc 时区里的时刻。
// 不是真实存在的时刻 (13 月 / 2 月 30 日 / 25 点 ...) -> ok=false: time.Date 会静默进位, 所以格式化回去比一遍。
func parseStamp(stamp string, loc *time.Location) (time.Time, bool) {
	n := func(i int) int { return int(stamp[i]-'0')*10 + int(stamp[i+1]-'0') }
	t := time.Date(2000+n(0), time.Month(n(2)), n(4), n(6), n(8), n(10), 0, loc)
	if formatStamp(t) != stamp {
		return time.Time{}, false
	}
	return t, true
}

// formatStamp 把 2000..2099 年的时刻格式化成 12 位时间戳 YYMMDDhhmmss (按 t 自己的时区)。
func formatStamp(t time.Time) string {
	return fmt.Sprintf("%02d%02d%02d%02d%02d%02d", t.Year()-2000, int(t.Month()), t.Day(), t.Hour(), t.Minute(), t.Second())
}

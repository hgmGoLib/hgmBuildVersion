package hgmBuildVersion

// timestamp_test.go — GetBuildTime / AddTimeSecond。

import (
	"testing"
	"time"
)

func TestAddTimeSecond(t *testing.T) {
	for _, c := range []struct {
		In     string
		Second int64
		Want   string
	}{
		{"v0.1.260901153059-a1b2c3d4e5-r", 1, "v0.1.260901153100-a1b2c3d4e5-r"}, // 按时间进位, 不是 …153060
		{"v0.1.260901153100-a1b2c3d4e5-r", -1, "v0.1.260901153059-a1b2c3d4e5-r"},
		{"v0.1.260901153059-a1b2c3d4e5-d", 0, "v0.1.260901153059-a1b2c3d4e5-d"},
		{"v0.1.261231235959-a1b2c3d4e5-r", 1, "v0.1.270101000000-a1b2c3d4e5-r"},  // 跨年
		{"v0.1.280228235959-a1b2c3d4e5-r", 1, "v0.1.280229000000-a1b2c3d4e5-r"},  // 闰年 2 月 29
		{"v0.1.270228235959-a1b2c3d4e5-r", 1, "v0.1.270301000000-a1b2c3d4e5-r"},  // 平年
		{"v0.1.681231235959-a1b2c3d4e5-r", 1, "v0.1.690101000000-a1b2c3d4e5-r"},  // 69 是 2069 不是 1969
		{"v0.1.690101000000-a1b2c3d4e5-r", -1, "v0.1.681231235959-a1b2c3d4e5-r"}, // 反过来也一样
		{"v12.34.260901153059-0123456789-r", 3600, "v12.34.260901163059-0123456789-r"},
		{"v0.1.260901153059-a1b2c3d4e5-r", -86400, "v0.1.260831153059-a1b2c3d4e5-r"},
		{"v0.1.000101000000-a1b2c3d4e5-r", 0, "v0.1.000101000000-a1b2c3d4e5-r"},
		{"v0.1.991231235959-a1b2c3d4e5-r", 0, "v0.1.991231235959-a1b2c3d4e5-r"},
	} {
		got, err := AddTimeSecond(c.In, c.Second)
		testOk(t, err == nil, "AddTimeSecond 不应失败: "+c.In)
		testEqual(t, got, c.Want)
		// 前缀 / commitId / 脏否不动, 只有序变了。
		cmp, ok := CompareOrder(got, c.In)
		testOk(t, ok, "结果应合法: "+got)
		testOk(t, (cmp > 0) == (c.Second > 0) && (cmp < 0) == (c.Second < 0), "序的方向不对: "+got)
		inId, _ := GetCommitId(c.In)
		gotId, _ := GetCommitId(got)
		testEqual(t, gotId, inId)
	}

	// 和时区无关。
	defer func(old int) { GBuildTimeZoneOffsetSecond = old }(GBuildTimeZoneOffsetSecond)
	GBuildTimeZoneOffsetSecond = -11 * 3600
	got, err := AddTimeSecond("v0.1.260901235959-a1b2c3d4e5-r", 1)
	testOk(t, err == nil && got == "v0.1.260902000000-a1b2c3d4e5-r", "时区不应影响 AddTimeSecond: "+got)

	for _, c := range []struct {
		In     string
		Second int64
		Note   string
	}{
		{"", 1, "空串"},
		{"dev", 1, "非法版本号"},
		{"v0.1.260901140715-a1b2c3d4e5", 1, "缺脏否"},
		{"v0.1.261301000000-a1b2c3d4e5-r", 1, "13 月"},
		{"v0.1.270229000000-a1b2c3d4e5-r", 0, "平年 2 月 29"},
		{"v0.1.260901240000-a1b2c3d4e5-r", 0, "24 点"},
		{"v0.1.999999999999-a1b2c3d4e5-r", -1, "全 9"},
		{"v0.1.991231235959-a1b2c3d4e5-r", 1, "超出 2099 年"},
		{"v0.1.000101000000-a1b2c3d4e5-r", -1, "早于 2000 年"},
		{"v0.1.260901140715-a1b2c3d4e5-r", 1 << 62, "second 离谱地大"},
		{"v0.1.260901140715-a1b2c3d4e5-r", -1 << 63, "second 离谱地小"},
	} {
		got, err := AddTimeSecond(c.In, c.Second)
		testOk(t, err != nil && got == "", "应返回 error: "+c.Note)
	}
}

func TestGetBuildTime(t *testing.T) {
	defer func(old int) { GBuildTimeZoneOffsetSecond = old }(GBuildTimeZoneOffsetSecond)
	for _, offset := range []int{8 * 3600, 0, -5 * 3600} {
		GBuildTimeZoneOffsetSecond = offset
		tm, ok := GetBuildTime("v0.1.690102030405-a1b2c3d4e5-r")
		testOk(t, ok, "合法版本号应取得出打包时刻")
		want := time.Date(2069, 1, 2, 3, 4, 5, 0, time.FixedZone("", offset))
		testOk(t, tm.Equal(want), "打包时刻不对: "+tm.String())
		_, gotOffset := tm.Zone()
		testEqual(t, gotOffset, offset)
	}
	for _, bad := range []string{"", "dev", "v0.1.261301000000-a1b2c3d4e5-r", "v0.1.999999999999-a1b2c3d4e5-r"} {
		tm, ok := GetBuildTime(bad)
		testOk(t, !ok && tm.IsZero(), "非法版本号 / 不存在的时刻不应取出打包时刻: "+bad)
	}
}

// NewBuildVersion 出来的串, GetBuildTime 读回去应当就是现在 (同一个时区设置)。
func TestGetBuildTimeRoundTrip(t *testing.T) {
	ver := makeVersion("v0.1", time.Now().In(time.FixedZone("", GBuildTimeZoneOffsetSecond)).Format("060102150405"), "a1b2c3d4e5", false)
	tm, ok := GetBuildTime(ver)
	testOk(t, ok, "应取得出打包时刻: "+ver)
	diff := time.Since(tm)
	testOk(t, diff > -time.Second && diff < time.Minute, "读回来的时刻和现在对不上: "+ver)
}

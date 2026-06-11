package slicex

import "testing"

// go test -bench=. -benchmem -count=3

func TestMap(t *testing.T) {
	got := Map([]int{1, 2, 3}, func(x int) int { return x * 2 })
	want := []int{2, 4, 6}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got[%d] = %d, want %d", i, got[i], want[i])
		}
	}
	if out := Map(nil, func(x int) int { return x }); len(out) != 0 {
		t.Fatalf("nil 入参应返回空切片, got len=%d", len(out))
	}
}

func TestMapPtr(t *testing.T) {
	in := []bigElem{{ID: 1}, {ID: 2}}
	got := MapPtr(in, func(e *bigElem) int64 { return e.ID * 10 })
	if got[0] != 10 || got[1] != 20 {
		t.Fatalf("got %v, want [10 20]", got)
	}
}

// bigElem 模拟 dao 行模型尺寸（若干 string + 数值列，约 200B），
// 用于对比 Map（每次调用值拷贝整个元素）与 MapPtr（传指针零拷贝）。
type bigElem struct {
	ID                     int64
	Name, Mode, Kind, Note string
	Payload                [12]int64
}

// sink 防止编译器把基准循环优化掉。
var (
	sinkInts []int
	sinkI64s []int64
	sinkStrs []string
)

func BenchmarkMap_Int(b *testing.B) {
	in := make([]int, 100000)
	for i := range in {
		in[i] = i
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sinkInts = Map(in, func(x int) int { return x * 2 })
	}
}

// BenchmarkHandLoop_Int 手写循环基线：与 Map_Int 同负载，验证泛型封装零额外开销（应基本持平）。
func BenchmarkHandLoop_Int(b *testing.B) {
	in := make([]int, 100000)
	for i := range in {
		in[i] = i
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		out := make([]int, len(in))
		for j := range in {
			out[j] = in[j] * 2
		}
		sinkInts = out
	}
}

func BenchmarkMap_BigStruct(b *testing.B) {
	in := make([]bigElem, 10000)
	for i := range in {
		in[i] = bigElem{ID: int64(i), Name: "n", Mode: "cdc", Kind: "k", Note: "x"}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sinkI64s = Map(in, func(e bigElem) int64 { return e.ID })
		//sinkStrs = Map(in, func(e bigElem) string { return e.Name })
	}
}

func BenchmarkMapPtr_BigStruct(b *testing.B) {
	in := make([]bigElem, 10000)
	for i := range in {
		in[i] = bigElem{ID: int64(i), Name: "n", Mode: "cdc", Kind: "k", Note: "x"}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sinkI64s = MapPtr(in, func(e *bigElem) int64 { return e.ID })
		//sinkStrs = MapPtr(in, func(e *bigElem) string { return e.Name })
	}
}

package slicex

// Map 把 []F 逐元素经 f 转成 []T。适用于元素较小的场景。
func Map[F, T any](in []F, f func(F) T) []T {
	out := make([]T, len(in))
	for i := range in {
		out[i] = f(in[i])
	}
	return out
}

// MapPtr 把 []F 逐元素经 f 转成 []T；f 接收元素指针，避免大结构体的值拷贝。
func MapPtr[F, T any](in []F, f func(*F) T) []T {
	out := make([]T, len(in))
	for i := range in {
		out[i] = f(&in[i])
	}
	return out
}

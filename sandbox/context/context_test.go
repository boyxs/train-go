package context

import (
	"context"
	"testing"
	"time"
)

type Key1 struct{}

func TestContext(t *testing.T) {
	ctx := context.WithValue(context.Background(),
		Key1{}, "value1")
	val := ctx.Value(Key1{})
	t.Log(val)
	// 反例：生产代码应使用自定义类型（如 Key1{}）作 key，字符串字面值会被 go vet 警告
	ctx = context.WithValue(ctx, "key2", "value2") //nolint:staticcheck // 学习演示：展示字符串 key 的反模式
	val = ctx.Value(Key1{})
	t.Log(val)
	val = ctx.Value("key2") //nolint:staticcheck
	t.Log(val)
	ctx = context.WithValue(ctx, Key1{}, "value1-1")
	val = ctx.Value(Key1{})
	t.Log(val)
}

func TestContext_Cancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	// 为什么一定要 cancel 呢？
	// 防止 goroutine 泄露
	defer cancel()

	// 防止有些人使用了 Done，在等待ctx结束信号
	go func() {
		ch := ctx.Done()
		<-ch
	}()

	// 在这里用 ctx

	ctx = context.WithValue(ctx, Key1{}, "value1-1")
	val := ctx.Value(Key1{})
	t.Log(val)

	ctx, cancel = context.WithTimeout(ctx, time.Second)
	cancel()
	ctx, cancel = context.WithDeadline(ctx, time.Now().Add(time.Second))
	cancel()
}

func TestContextErr(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	cancel()
	ctx.Err()
	// 你怎么区别被取消，还是超时了呢？
	if ctx.Err() == context.Canceled {
		t.Log("取消了")
	} else if ctx.Err() == context.DeadlineExceeded {
		t.Log("超时了")
	}
}

// TestContextSub 演示：父 ctx cancel 后，子 ctx 也会 Done
func TestContextSub(t *testing.T) {
	ctx, cancel0 := context.WithCancel(context.Background())
	defer cancel0()
	subCtx, cancelSub := context.WithCancel(ctx)
	defer cancelSub()

	done := make(chan struct{})

	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel0() // 取消父，subCtx 也会 Done
	}()

	go func() {
		t.Log("等待信号...")
		<-subCtx.Done()
		t.Log("收到信号：父 cancel 传导到子")
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("超时：父 cancel 未能传导到子 ctx")
	}
}

// TestContextSubCancel 演示：子 ctx cancel 不会影响父 ctx
func TestContextSubCancel(t *testing.T) {
	ctx, cancel0 := context.WithCancel(context.Background())
	defer cancel0()
	_, cancel1 := context.WithCancel(ctx)

	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel1() // 只取消子
	}()

	// 等一会儿后验证父 ctx 未被影响
	time.Sleep(300 * time.Millisecond)
	select {
	case <-ctx.Done():
		t.Fatal("父 ctx 不应被子的 cancel 影响")
	default:
		t.Log("父 ctx 正常：子 cancel 不会反向传导")
	}
}

//func MockIO() {
//	select {
//
//	case <-ctx.Done():
//		// 监听超时 或者用户主动取消
//
//	case <-biz.Signal():
//		// 监听你的正常业务
//	}
//}

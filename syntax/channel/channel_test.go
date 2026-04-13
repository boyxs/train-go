package channel

import (
	"context"
	"runtime"
	"sync"
	"testing"
	"time"
)

func TestChannel(t *testing.T) {
	// 声明一个放 int 类型的 channel
	// 但是没有初始化，读写这个都会崩溃
	//var ch chan int
	//ch <- 123
	//val := <-ch
	//t.Log(val)

	// 放空结构体，一般用来做信号
	//var ch1 chan struct{}

	// 固定容量
	ch2 := make(chan int, 2)
	ch2 <- 123
	ch2 <- 456
	// 第一次读
	val, ok := <-ch2
	t.Log(val, ok)

	// 这里关闭了，只能读，不能写
	close(ch2)
	//ch2 <- 789

	// 第二次读
	val, ok = <-ch2
	t.Log(val, ok)

	// 第三次读
	// 1.channel没有关闭，会读崩溃
	// 2.channel关闭了，读不到数据
	val, ok = <-ch2
	t.Log(val, ok)

}

type ChanStruct struct {
	//Ch        chan struct{}
	ch    chan struct{}
	ctx   context.Context
	close sync.Once
}

// 用户会多次调用，或者多个 goroutine 调用
func (m *ChanStruct) Close() error {
	m.close.Do(func() {
		// 确保整个代码只会执行一次
		close(m.ch)
	})
	return nil
}

func TestChannelLoop(t *testing.T) {
	ch := make(chan int)
	go func() {
		for i := 0; i < 10; i++ {
			ch <- i
			time.Sleep(time.Millisecond * 100)
		}
		close(ch)
	}()

	for val := range ch {
		t.Log(val)
	}

	//for {
	//	val, ok := <-ch
	//	if !ok {
	//		break
	//	}
	//	t.Log(val)
	//}

	t.Log("channel 被关了")
}

func TestChannelBlock(t *testing.T) {
	ch := make(chan int, 1)

	val := <-ch
	t.Log(val)

	runtime.NumGoroutine()
}

func TestSelect(t *testing.T) {
	ch1 := make(chan int, 1)
	ch2 := make(chan int, 1)

	//ch1 <- 123
	//ch2 <- 456

	go func() {
		time.Sleep(time.Millisecond * 100)
		ch1 <- 123
	}()
	go func() {
		time.Sleep(time.Millisecond * 100)
		ch2 <- 456
	}()

	// 当多个条件同时满足，会随机选一个分支执行
	select {
	case val := <-ch1:
		t.Log("ch1 ", val)
		val = <-ch2
		t.Log("ch2 ", val)
	case val := <-ch2:
		t.Log("ch2 ", val)
		val = <-ch1
		t.Log("ch1 ", val)
	}

}

func TestGoroutineCh(t *testing.T) {
	ch := make(chan int)
	// 这一个就泄露掉了
	go func() {
		// 永久阻塞在这里
		ch <- 123
	}()

	// 这里后面没人往 ch 里面读数据
}

func TestGoroutineChRead(t *testing.T) {
	ch := make(chan int, 100000)
	// 这一个就泄露掉了
	def := new(BigObj)
	go func() {
		// 永久阻塞在这里
		for i := 0; i < 100000; i++ {
			ch <- i
		}
		abc := new(BigObj)
		t.Log(abc)
		t.Log(def)
		// 永久阻塞在这里，ch 占据的内存，永远不会被回收
		ch <- 1
	}()

	// 这里后面没人往 ch 里面读数据
}

type BigObj struct {
}

package queue

import (
	"go.uber.org/atomic"
	"runtime"
	"sync"
	"testing"
)

func yield() {
	runtime.Gosched()
}

func BenchmarkChannelQueue(b *testing.B) {
	b.ReportAllocs()

	q := NewBoundedQueue(1000, func(item interface{}) {
		yield()
	})
	defer q.Stop()

	q.StartConsumers(10, func(item interface{}) {
		yield()
	})

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		q.Produce(n)
		yield()
	}
}

func BenchmarkRingBuffer(b *testing.B) {
	b.ReportAllocs()

	q := NewRingBufferQueue(1000)
	qq := NewConcurrentQueue(q, func(item interface{}) {
		yield()
	})
	defer qq.Close()

	qq.StartConsumers(10, func(item interface{}) {
		yield()
	})

	for n := 0; n < b.N; n++ {
		qq.Produce(n)
		yield()
	}
}

func BenchmarkChannel(b *testing.B) {
	b.ReportAllocs()

	ch := make(chan struct{}, 1000)
	defer close(ch)
	for n := 0; n < 10; n++ {
		go func() {
			for range ch {
				yield()
			}
		}()
	}

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		ch <- struct{}{}
		yield()
	}
}

func BenchmarkCond(b *testing.B) {
	b.ReportAllocs()

	stop := atomic.NewBool(false)
	c := sync.NewCond(&sync.Mutex{})

	for n := 0; n < 10; n++ {
		go func() {
			for !stop.Load() {
				c.L.Lock()
				c.Wait()
				c.L.Unlock()
				yield()
			}
		}()
	}

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		c.L.Lock()
		c.Broadcast()
		c.L.Unlock()
		yield()
	}
	stop.Store(true)
	c.L.Lock()
	c.Broadcast()
	c.L.Unlock()
}

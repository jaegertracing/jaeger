package queue

import "testing"

func BenchmarkChannelQueue(b *testing.B) {
	q := NewBoundedQueue(1000, func(item interface{}) {
	})
	defer q.Stop()

	q.StartConsumers(10, func(item interface{}) {
	})

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		q.Produce(n)
	}
}

func BenchmarkRingBuffer(b *testing.B) {
	q := NewRingBufferQueue(1000)
	qq := NewConcurrentQueue(q, func(item interface{}) {
	})
	defer qq.Close()

	qq.StartConsumers(10, func(item interface{}) {
	})

	for n := 0; n < b.N; n++ {
		qq.Produce(n)
	}
}

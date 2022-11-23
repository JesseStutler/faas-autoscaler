package sender

import "github.com/emirpasic/gods/queues/arrayqueue"

type SlideWindow struct {
	Data    *arrayqueue.Queue
	MaxSize int
}

func NewSlideWindow(maxSize int) *SlideWindow {
	return &SlideWindow{
		Data:    arrayqueue.New(),
		MaxSize: maxSize,
	}
}

func (s *SlideWindow) Enqueue(value interface{}) {
	s.Data.Enqueue(value)
	if s.Data.Size() > s.MaxSize {
		s.Data.Dequeue()
	}
}

// Values returns all elements in the queue (FIFO order).
func (s *SlideWindow) Values() []interface{} {
	return s.Data.Values()
}

// IsFull 用来判断SlideWindow是否有了足够元素
func (s *SlideWindow) IsFull() bool {
	if s.Data.Size() < s.MaxSize {
		return false
	}
	return true
}

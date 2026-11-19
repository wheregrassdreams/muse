package shell

// ResultPump bounds work performed while draining asynchronous results on the
// UI thread. The channel producer and result policy stay with its owner.
type ResultPump[T any] struct{ results <-chan T }

func NewResultPump[T any](results <-chan T) *ResultPump[T] {
	return &ResultPump[T]{results: results}
}

func (p *ResultPump[T]) Drain(limit int, accept func(T)) (drained int, open bool) {
	if p == nil || p.results == nil {
		return 0, false
	}
	if limit <= 0 {
		limit = 32
	}
	for drained < limit {
		select {
		case result, ok := <-p.results:
			if !ok {
				p.results = nil
				return drained, false
			}
			drained++
			if accept != nil {
				accept(result)
			}
		default:
			return drained, true
		}
	}
	return drained, true
}

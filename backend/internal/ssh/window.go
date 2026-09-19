package ssh

import "time"

const windowSlots = 30

type windowBucket struct {
	start       time.Time
	total, hits int
}

type slidingWindow struct {
	span, step time.Duration
	keys       map[string][]windowBucket
}

func newSlidingWindow(span time.Duration) *slidingWindow {
	step := span / windowSlots
	if step < time.Second {
		step = time.Second
	}
	return &slidingWindow{span: span, step: step, keys: map[string][]windowBucket{}}
}

func (w *slidingWindow) add(key string, now time.Time, hit bool) (total, hits int) {
	cutoff := now.Add(-w.span)
	buckets := expire(w.keys[key], cutoff, w.step)

	start := now.Truncate(w.step)
	if n := len(buckets); n > 0 && buckets[n-1].start.Equal(start) {
		buckets[n-1].total++
		if hit {
			buckets[n-1].hits++
		}
	} else {
		b := windowBucket{start: start, total: 1}
		if hit {
			b.hits = 1
		}
		buckets = append(buckets, b)
	}
	w.keys[key] = buckets

	if len(w.keys) > 1000 {
		w.sweep(cutoff)
	}

	for _, b := range buckets {
		total += b.total
		hits += b.hits
	}
	return total, hits
}

func (w *slidingWindow) sweep(cutoff time.Time) {
	for k, buckets := range w.keys {
		if rest := expire(buckets, cutoff, w.step); len(rest) == 0 {
			delete(w.keys, k)
		} else {
			w.keys[k] = rest
		}
	}
}

func expire(buckets []windowBucket, cutoff time.Time, step time.Duration) []windowBucket {
	i := 0
	for i < len(buckets) && !buckets[i].start.Add(step).After(cutoff) {
		i++
	}
	return buckets[i:]
}

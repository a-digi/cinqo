package download

import "time"

// rate estimates a transfer speed over the last few seconds, so the number follows what is happening now and is not
// dominated by the first second or by an average over the whole transfer. It is pure (the time is passed in), so it can be
// tested with a fake clock.
type rate struct {
	window  time.Duration
	minSpan time.Duration
	step    time.Duration // at most one sample per step
	samples []rateSample
}

type rateSample struct {
	at    time.Time
	bytes int64 // bytes received so far in this run
}

func newRate() *rate {
	return &rate{window: 5 * time.Second, minSpan: time.Second, step: 100 * time.Millisecond}
}

// add records the number of bytes received so far.
func (r *rate) add(at time.Time, received int64) {
	if n := len(r.samples); n > 0 && at.Sub(r.samples[n-1].at) < r.step {
		r.samples[n-1] = rateSample{at: r.samples[n-1].at, bytes: received}
		return
	}
	r.samples = append(r.samples, rateSample{at: at, bytes: received})
	r.trim(at)
}

func (r *rate) trim(now time.Time) {
	cut := 0
	for cut < len(r.samples)-1 && now.Sub(r.samples[cut].at) > r.window {
		cut++
	}
	if cut > 0 {
		r.samples = append(r.samples[:0], r.samples[cut:]...)
	}
}

// perSecond is the speed over the last window, or 0 while it cannot be told: fewer than two samples, a span shorter than
// minSpan, or no byte for a whole window (a stall).
func (r *rate) perSecond(now time.Time) float64 {
	if len(r.samples) < 2 {
		return 0
	}
	last := r.samples[len(r.samples)-1]
	if now.Sub(last.at) > r.window {
		return 0
	}
	first := r.samples[0]
	for _, s := range r.samples {
		if now.Sub(s.at) <= r.window {
			first = s
			break
		}
	}
	span := last.at.Sub(first.at)
	if span < r.minSpan || last.bytes <= first.bytes {
		return 0
	}
	return float64(last.bytes-first.bytes) / span.Seconds()
}

package livelog

import "context"

// Poll asks a follower for one batch under the limits: it clamps the number of lines, bounds the call by the limit's timeout and by
// the caller's own context (a closed tab stops the work), and checks the batch against the contract so a broken follower cannot send
// more than was asked or a cursor that does not parse. A follower that ignores its context is still given up on at the timeout (its
// goroutine ends when it returns; the caller does not wait for it).
func Poll(ctx context.Context, f Follower, cursor string, asked int, lim Limits) (Batch, error) {
	if _, _, err := ParseCursor(cursor); err != nil {
		return Batch{}, err
	}
	lim = lim.normalized()
	limit := lim.Lines(asked)
	ctx, cancel := context.WithTimeout(ctx, lim.PollTimeout)
	defer cancel()

	type result struct {
		b   Batch
		err error
	}
	done := make(chan result, 1)
	go func() {
		b, err := f.Poll(ctx, cursor, limit)
		done <- result{b, err}
	}()
	select {
	case r := <-done:
		if r.err != nil {
			return Batch{}, r.err
		}
		return checked(r.b, limit)
	case <-ctx.Done():
		if ctx.Err() == context.DeadlineExceeded {
			return Batch{}, ErrTimeout
		}
		return Batch{}, ctx.Err()
	}
}

// checked makes sure a batch keeps the contract and never serializes as null.
func checked(b Batch, limit int) (Batch, error) {
	if len(b.Lines) > limit {
		return Batch{}, ErrBadBatch
	}
	if _, _, err := ParseCursor(b.Cursor); err != nil {
		return Batch{}, ErrBadBatch
	}
	if b.Cursor == "" {
		return Batch{}, ErrBadBatch // a follower always says where to continue
	}
	if b.Lines == nil {
		b.Lines = []Line{}
	}
	return b, nil
}

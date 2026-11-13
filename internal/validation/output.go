package validation

// boundedOutput avoids unbounded memory consumption from noisy checkers.
type boundedOutput struct {
	limit     int
	data      []byte
	truncated bool
}

func (w *boundedOutput) Write(p []byte) (int, error) {
	n := len(p)
	available := w.limit - len(w.data)
	if available < n {
		w.truncated = true
		if available > 0 {
			w.data = append(w.data, p[:available]...)
		}
	} else {
		w.data = append(w.data, p...)
	}
	return n, nil
}

func (w *boundedOutput) String() string {
	if w.truncated {
		return string(w.data) + "\n[output truncated]"
	}
	return string(w.data)
}

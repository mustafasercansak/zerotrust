package audit

import (
	"sync/atomic"
	"testing"
)

func TestWriteFailuresCounter(t *testing.T) {
	atomic.StoreUint64(&writeFailures, 0)
	t.Cleanup(func() {
		atomic.StoreUint64(&writeFailures, 0)
	})

	if got := WriteFailures(); got != 0 {
		t.Fatalf("initial WriteFailures=%d want=0", got)
	}

	RecordWriteFailure()
	RecordWriteFailure()

	if got := WriteFailures(); got != 2 {
		t.Fatalf("WriteFailures=%d want=2", got)
	}
}

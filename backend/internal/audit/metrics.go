package audit

import "sync/atomic"

var writeFailures uint64

func RecordWriteFailure() {
	atomic.AddUint64(&writeFailures, 1)
}

func WriteFailures() uint64 {
	return atomic.LoadUint64(&writeFailures)
}

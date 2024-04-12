package main

// MockTailer implements the LogTailer interface, for testing, and for services
// we will not follow.
type MockTailer struct {
	FlushOffsetsWasCalled bool
	RunWasCalled          bool
	StopWasCalled         bool

	PodTailed *Pod
}

func (t *MockTailer) TailLogs(logFiles []string) error { return nil }
func (t *MockTailer) Run()                             { t.RunWasCalled = true }
func (t *MockTailer) FlushOffsets()                    { t.FlushOffsetsWasCalled = true }
func (t *MockTailer) Stop()                            { t.StopWasCalled = true }

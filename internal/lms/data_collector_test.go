package lms

import (
	"fmt"
	"strconv"
	"testing"
	"time"
)

func TestProcessMessage_hoooksCalled(t *testing.T) {
	const (
		msgCount   int = 20
		hooksCount int = 3
	)
	hookInvocations := map[string]bool{}
	processHooks := []ProcessHookFunc{}
	for i := 1; i <= hooksCount; i++ {
		idx := i
		processHooks = append(processHooks, func(m *Message, mb MessageBuffer) bool {
			hookInvocations[fmt.Sprintf("hook%d-%s", idx, m.ServiceName)] = true
			return true
		})
	}
	dc := NewDataCollector(newTestLogger(t), NewSliceBuffer()).
		WithProcessHooks(
			processHooks...,
		)
	for i := 1; i <= msgCount; i++ {
		err := dc.processMessage(&Message{
			ServiceName: "service-" + strconv.Itoa(i),
			Timestamp:   time.Now().UTC(),
		})

		if err != nil {
			t.Fatalf("error is not expected, but got: %v", err)
		}
	}
	expectedBufferLen := msgCount
	bufferLen := dc.Buffer().Len()
	if bufferLen != expectedBufferLen {
		t.Errorf("expected buffer length to be %d got %d", expectedBufferLen, bufferLen)
	}
	for i := 1; i <= hooksCount; i++ {
		for j := 1; j <= msgCount; j++ {
			invocationID := fmt.Sprintf("hook%d-service-%d", i, j)
			if _, ok := hookInvocations[invocationID]; !ok {
				t.Errorf("expected %s to be hooked but it wasn't", invocationID)
			}
		}
	}
}

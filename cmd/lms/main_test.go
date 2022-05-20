package main

import (
	"testing"

	"github.com/maxim-nazarenko/fiskil-lms/internal/lms/storage"
	"github.com/stretchr/testify/assert"
)

func TestFlatten(t *testing.T) {

	in := []*storage.LogStat{
		{
			ServiceName: "service-1",
			Severity:    "info",
			Count:       3,
		},
		{
			ServiceName: "service-1",
			Severity:    "debug",
		},
		{
			ServiceName: "service-1",
			Severity:    "info",
		},
		{
			ServiceName: "service-2",
			Severity:    "info",
		},
		{
			ServiceName: "service-2",
			Severity:    "info",
		},
		{
			ServiceName: "service-2",
			Severity:    "info",
		},
		{
			ServiceName: "service-3",
			Severity:    "error",
		},
	}

	expectedResult := map[string]int{
		"service-1,info":  4,
		"service-1,debug": 1,
		"service-2,info":  3,
		"service-3,error": 1,
	}
	res := flattenLogStats(in)
	assert.Equal(t, expectedResult, res)
}

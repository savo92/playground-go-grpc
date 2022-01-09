package utils

import (
	"fmt"
	"testing"
)

type stringerMock struct {
	s string
}

func (s stringerMock) String() string {
	return s.s
}

func TestAfterEvent(t *testing.T) {
	testsTable := []struct {
		Name     string
		Stringer fmt.Stringer
		Want     string
	}{
		{
			Name:     "after_something",
			Stringer: stringerMock{s: "something"},
			Want:     "after_something",
		},
	}

	for _, tt := range testsTable {
		t.Run(tt.Name, func(t *testing.T) {
			if s := AfterEvent(tt.Stringer); s != tt.Want {
				t.Errorf("AfterEvent(%v)=%s; want %s", tt.Stringer, s, tt.Want)
			}
		})
	}
}

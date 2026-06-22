package sched

import (
	"sort"
	"testing"
)

func keys(m map[int]bool) []int {
	out := make([]int, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Ints(out)
	return out
}

func eq(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func allActive(int) bool { return true }

func TestMaxDroplets(t *testing.T) {
	// (70 - 1) / 3 = 23
	if got := MaxDroplets(1); got != 23 {
		t.Fatalf("MaxDroplets(1) = %d, want 23", got)
	}
	if got := MaxDroplets(1000); got != 1 {
		t.Fatalf("MaxDroplets(huge) = %d, want floor of 1", got)
	}
}

func TestSelectWindowCenteredOnCursor(t *testing.T) {
	// 10 rows, all active, budget 3, cursor in the middle -> {4,5,6}.
	got := SelectWindow(10, allActive, 5, 3)
	if !eq(keys(got), []int{4, 5, 6}) {
		t.Fatalf("center window = %v, want [4 5 6]", keys(got))
	}
	// Cursor at the top clamps the window downward -> {0,1,2}.
	got = SelectWindow(10, allActive, 0, 3)
	if !eq(keys(got), []int{0, 1, 2}) {
		t.Fatalf("top window = %v, want [0 1 2]", keys(got))
	}
	// Cursor at the bottom clamps upward -> {7,8,9}.
	got = SelectWindow(10, allActive, 9, 3)
	if !eq(keys(got), []int{7, 8, 9}) {
		t.Fatalf("bottom window = %v, want [7 8 9]", keys(got))
	}
}

func TestSelectWindowSkipsInactiveForFree(t *testing.T) {
	// Only even indices are active. Budget 3, cursor at 4.
	active := func(i int) bool { return i%2 == 0 }
	got := SelectWindow(11, active, 4, 3)
	// Walk from 4: 4(✓), 3(✗),5(✗), 2(✓),6(✓) -> {2,4,6}; inactive cost nothing.
	if !eq(keys(got), []int{2, 4, 6}) {
		t.Fatalf("window = %v, want [2 4 6] (inactive skipped for free)", keys(got))
	}
}

func TestSelectWindowBudgetNeverExceeded(t *testing.T) {
	got := SelectWindow(1000, allActive, 500, MaxDroplets(1))
	if len(got) > 23 {
		t.Fatalf("selected %d, must not exceed budget of 23", len(got))
	}
	// Total calls stay under the per-minute ceiling.
	if len(got)*CallsPerDroplet+1 > CallsPerMinute {
		t.Fatalf("calls %d exceed budget %d", len(got)*CallsPerDroplet+1, CallsPerMinute)
	}
}

func TestSelectWindowSmallList(t *testing.T) {
	// Fewer active rows than budget -> select them all, no panic.
	got := SelectWindow(3, allActive, 0, 23)
	if !eq(keys(got), []int{0, 1, 2}) {
		t.Fatalf("small list = %v, want [0 1 2]", keys(got))
	}
	if len(SelectWindow(0, allActive, 0, 23)) != 0 {
		t.Fatal("empty list should select nothing")
	}
}

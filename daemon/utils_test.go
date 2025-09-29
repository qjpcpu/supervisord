package daemon

import "testing"

func TestFileContains(t *testing.T) {
	assertContains(t, "./log", "./log/stdout.log", true)
	assertContains(t, "./log", "./log/stdout.log*", true)
	assertContains(t, "./log*", "./log/stdout.log*", true)
	assertContains(t, "./log*", "./log/stdout.log", true)
	assertContains(t, "./log1", "./log12", false)
	assertContains(t, "./log1", "./log12*", false)
	assertContains(t, "./log1", "./log1*", false)
	assertContains(t, "./log1*", "./log1", true)
	assertContains(t, "./log1*", "./log1*", true)
	assertContains(t, "./log1", "./log1", true)
}

func assertContains(t *testing.T, a, b string, v bool) {
	if ret := filePathContains(a, b); ret != v {
		t.Fatalf("%v contains %v %v", a, b, !v)
	}
}

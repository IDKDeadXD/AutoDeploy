package secrets

import "testing"

func TestNew(t *testing.T) {
	a, err := New()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := New()
	if len(a) < 40 || a == b {
		t.Fatalf("weak secret %q", a)
	}
}

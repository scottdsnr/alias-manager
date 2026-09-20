package update

import "testing"

func TestNewer(t *testing.T) {
	cases := []struct {
		local, remote string
		want          bool
	}{
		{"0.1.0", "v0.1.1", true},
		{"0.1.0", "v0.1.0", false},
		{"0.2.0", "v0.1.9", false},
		{"1.0.0", "v1.0.0", false},
		{"0.9.0", "v1.0.0", true},
		{"dev", "v0.1.0", true},
		{"0.1.0", "v0.2.0-rc1", true},
	}
	for _, c := range cases {
		if got := Newer(c.local, c.remote); got != c.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", c.local, c.remote, got, c.want)
		}
	}
}

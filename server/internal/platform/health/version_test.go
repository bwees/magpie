package health

import "testing"

func TestVersionAtLeast(t *testing.T) {
	cases := []struct {
		version string
		minimum []int
		want    bool
	}{
		{"2.15.0", []int{2, 15}, true},
		{"2.15", []int{2, 15, 0}, true},
		{"v5.2.1", []int{5, 2}, true},
		{"2.14.9", []int{2, 15}, false},
		{"3.0", []int{2, 15}, true},
		{"10.11.2", []int{12}, false},
		{"12.0.0", []int{12}, true},
	}
	for _, tc := range cases {
		got, err := VersionAtLeast(tc.version, tc.minimum...)
		if err != nil || got != tc.want {
			t.Errorf("VersionAtLeast(%q, %v) = %v, %v; want %v", tc.version, tc.minimum, got, err, tc.want)
		}
	}
	if _, err := VersionAtLeast("unknown", 2); err == nil {
		t.Error("an unreadable version passed")
	}
}

package clientip

import "testing"

func TestRange(t *testing.T) {
	cases := []struct{ in, first, last string }{
		{"192.0.2.1", "192.0.2.1", "192.0.2.1"},
		{"::ffff:192.0.2.1", "192.0.2.1", "192.0.2.1"},
		{"2001:db8:1:2:3:4:5:6", "2001:db8:1:2::", "2001:db8:1:2:ffff:ffff:ffff:ffff"},
		{"not-an-ip", "not-an-ip", "not-an-ip"},
	}
	for _, tc := range cases {
		if first, last := Range(tc.in); first != tc.first || last != tc.last {
			t.Fatalf("%s: got %s %s", tc.in, first, last)
		}
	}
}

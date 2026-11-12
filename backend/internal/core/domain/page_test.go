package domain

import "testing"

func TestNewPageBringsValuesIntoRange(t *testing.T) {
	for _, tc := range []struct {
		number, limit int
		want          Page
	}{
		{0, 0, Page{1, 20}},
		{-5, -5, Page{1, 20}},
		{3, 50, Page{3, 50}},
		{1, 1000, Page{1, 100}},
		{1 << 62, 100, Page{maxPageNumber, 100}},
	} {
		if got := NewPage(tc.number, tc.limit); got != tc.want {
			t.Errorf("NewPage(%d, %d) = %+v", tc.number, tc.limit, got)
		}
	}
	if got := NewPage(3, 20).Offset(); got != 40 {
		t.Errorf("offset %d", got)
	}
}

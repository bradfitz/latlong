package latlong

import (
	"strconv"
	"testing"
)

func TestNewTileKey(t *testing.T) {
	cases := []struct {
		size, x, y int
	}{
		{0, 1<<14 - 1, 1<<14 - 1},
		{0, 1<<14 - 1, 0},
		{0, 0, 1<<14 - 1},
		{0, 0, 0},
		{1, 1, 1},
		{1, 2, 3},
		{2, 3, 1},
		{3, 3, 3},
	}
	for i, tt := range cases {
		tk := newTileKey(byte(tt.size), uint16(tt.x), uint16(tt.y))
		if tk.size() != byte(tt.size) {
			t.Errorf("%d. size = %d; want %d", i, tk.size(), tt.size)
		}
		if tk.x() != uint16(tt.x) {
			t.Errorf("%d. x = %d; want %d", i, tk.x(), tt.x)
		}
		if tk.y() != uint16(tt.y) {
			t.Errorf("%d. y = %d; want %d", i, tk.y(), tt.y)
		}
	}

	t.Logf("Tile key = %32q", strconv.FormatInt(int64(
		newTileKey(3, 1<<13+7, 1<<13+3)),
		2))
}

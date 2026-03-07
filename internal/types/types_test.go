package types

import "testing"

func TestSortFieldString(t *testing.T) {
	tests := []struct {
		field SortField
		want  string
	}{
		{SortByTime, "time"},
		{SortByContext, "ctx%"},
		{SortBySize, "size"},
		{SortByName, "name"},
		{SortField(99), ""},
	}
	for _, tt := range tests {
		if got := tt.field.String(); got != tt.want {
			t.Errorf("SortField(%d).String() = %q, want %q", tt.field, got, tt.want)
		}
	}
}

func TestSortFieldNext(t *testing.T) {
	tests := []struct {
		field SortField
		want  SortField
	}{
		{SortByTime, SortByContext},
		{SortByContext, SortBySize},
		{SortBySize, SortByName},
		{SortByName, SortByTime}, // wraps around
	}
	for _, tt := range tests {
		if got := tt.field.Next(); got != tt.want {
			t.Errorf("SortField(%d).Next() = %d, want %d", tt.field, got, tt.want)
		}
	}
}

func TestSortFieldNextFullCycle(t *testing.T) {
	f := SortByTime
	for i := 0; i < 4; i++ {
		f = f.Next()
	}
	if f != SortByTime {
		t.Errorf("full cycle ended at %d, want %d (SortByTime)", f, SortByTime)
	}
}

func TestSortDirToggle(t *testing.T) {
	tests := []struct {
		dir  SortDir
		want SortDir
	}{
		{SortDesc, SortAsc},
		{SortAsc, SortDesc},
	}
	for _, tt := range tests {
		if got := tt.dir.Toggle(); got != tt.want {
			t.Errorf("SortDir(%d).Toggle() = %d, want %d", tt.dir, got, tt.want)
		}
	}
}

func TestSortDirToggleDoubleFlip(t *testing.T) {
	if got := SortDesc.Toggle().Toggle(); got != SortDesc {
		t.Errorf("double toggle = %d, want %d (SortDesc)", got, SortDesc)
	}
}

func TestSortDirString(t *testing.T) {
	tests := []struct {
		dir  SortDir
		want string
	}{
		{SortAsc, "↑"},
		{SortDesc, "↓"},
	}
	for _, tt := range tests {
		if got := tt.dir.String(); got != tt.want {
			t.Errorf("SortDir(%d).String() = %q, want %q", tt.dir, got, tt.want)
		}
	}
}

package session

import "testing"

func TestDetectActive_ReturnsInitializedMap(t *testing.T) {
	info := DetectActive()
	if info.ProjectDirs == nil {
		t.Error("ProjectDirs should be initialized, got nil")
	}
}

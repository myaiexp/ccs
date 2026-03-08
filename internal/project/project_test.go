package project

import (
	"ccs/internal/types"
	"testing"
	"time"
)

func TestDiscoverProjects_Empty(t *testing.T) {
	got := DiscoverProjects(nil, &types.Config{})
	if len(got) != 0 {
		t.Fatalf("expected 0 projects, got %d", len(got))
	}
}

func TestDiscoverProjects_Deduplicates(t *testing.T) {
	now := time.Now()
	sessions := []types.Session{
		{ProjectName: "foo", ProjectDir: "/p/foo", LastActive: now},
		{ProjectName: "foo", ProjectDir: "/p/foo", LastActive: now.Add(-time.Hour)},
	}
	got := DiscoverProjects(sessions, &types.Config{})
	if len(got) != 1 {
		t.Fatalf("expected 1 project, got %d", len(got))
	}
	if got[0].Name != "foo" {
		t.Errorf("expected name foo, got %s", got[0].Name)
	}
}

func TestDiscoverProjects_MostRecentLastActive(t *testing.T) {
	now := time.Now()
	older := now.Add(-2 * time.Hour)
	newer := now.Add(-1 * time.Hour)
	sessions := []types.Session{
		{ProjectName: "proj", ProjectDir: "/p/proj", LastActive: older},
		{ProjectName: "proj", ProjectDir: "/p/proj", LastActive: newer},
	}
	got := DiscoverProjects(sessions, &types.Config{})
	if len(got) != 1 {
		t.Fatalf("expected 1 project, got %d", len(got))
	}
	if !got[0].LastActive.Equal(newer) {
		t.Errorf("expected LastActive=%v, got %v", newer, got[0].LastActive)
	}
}

func TestDiscoverProjects_HasActiveFromSession(t *testing.T) {
	now := time.Now()
	sessions := []types.Session{
		{ProjectName: "active", ProjectDir: "/p/active", LastActive: now, IsActive: true},
		{ProjectName: "inactive", ProjectDir: "/p/inactive", LastActive: now},
	}
	got := DiscoverProjects(sessions, &types.Config{})
	byName := make(map[string]types.Project)
	for _, p := range got {
		byName[p.Name] = p
	}
	if !byName["active"].HasActive {
		t.Error("expected 'active' project to have HasActive=true")
	}
	if byName["inactive"].HasActive {
		t.Error("expected 'inactive' project to have HasActive=false")
	}
}

func TestDiscoverProjects_HasActiveFromAnySession(t *testing.T) {
	now := time.Now()
	sessions := []types.Session{
		{ProjectName: "proj", ProjectDir: "/p/proj", LastActive: now.Add(-time.Hour), IsActive: false},
		{ProjectName: "proj", ProjectDir: "/p/proj", LastActive: now, IsActive: true},
	}
	got := DiscoverProjects(sessions, &types.Config{})
	if !got[0].HasActive {
		t.Error("expected HasActive=true when any session is active")
	}
}

func TestDiscoverProjects_HiddenFromConfig(t *testing.T) {
	now := time.Now()
	sessions := []types.Session{
		{ProjectName: "visible", ProjectDir: "/p/visible", LastActive: now},
		{ProjectName: "secret", ProjectDir: "/p/secret", LastActive: now},
	}
	cfg := &types.Config{HiddenProjects: []string{"secret"}}
	got := DiscoverProjects(sessions, cfg)
	byName := make(map[string]types.Project)
	for _, p := range got {
		byName[p.Name] = p
	}
	if byName["visible"].Hidden {
		t.Error("expected 'visible' to not be hidden")
	}
	if !byName["secret"].Hidden {
		t.Error("expected 'secret' to be hidden")
	}
}

func TestDiscoverProjects_SortOrder(t *testing.T) {
	now := time.Now()
	sessions := []types.Session{
		{ProjectName: "old", ProjectDir: "/p/old", LastActive: now.Add(-3 * time.Hour)},
		{ProjectName: "recent", ProjectDir: "/p/recent", LastActive: now.Add(-1 * time.Hour)},
		{ProjectName: "active-old", ProjectDir: "/p/active-old", LastActive: now.Add(-2 * time.Hour), IsActive: true},
		{ProjectName: "middle", ProjectDir: "/p/middle", LastActive: now.Add(-2 * time.Hour)},
	}
	got := DiscoverProjects(sessions, &types.Config{})

	// Active projects come first
	if got[0].Name != "active-old" {
		t.Errorf("expected first project to be active-old, got %s", got[0].Name)
	}
	// Then by recency descending
	if got[1].Name != "recent" {
		t.Errorf("expected second project to be recent, got %s", got[1].Name)
	}
	// middle and old are both inactive; middle is more recent
	if got[2].Name != "middle" {
		t.Errorf("expected third project to be middle, got %s", got[2].Name)
	}
	if got[3].Name != "old" {
		t.Errorf("expected fourth project to be old, got %s", got[3].Name)
	}
}

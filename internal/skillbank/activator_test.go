package skillbank

import (
	"context"
	"sort"
	"testing"
)

type activatorTestService struct {
	metas    []SkillMeta
	recorded map[string]int
}

func (s *activatorTestService) Search(context.Context, QueryOptions) ([]Skill, error) {
	return nil, nil
}
func (s *activatorTestService) SearchMetadata(context.Context, QueryOptions) ([]SkillSearchHit, error) {
	return nil, nil
}
func (s *activatorTestService) View(context.Context, string) (*Skill, error)  { return nil, nil }
func (s *activatorTestService) Get(context.Context, string) (*Skill, error)   { return nil, nil }
func (s *activatorTestService) List(context.Context, string) ([]Skill, error) { return nil, nil }
func (s *activatorTestService) ListMetadata(context.Context) ([]SkillMeta, error) {
	return s.metas, nil
}
func (s *activatorTestService) Create(context.Context, Skill) error         { return nil }
func (s *activatorTestService) Update(context.Context, string, Skill) error { return nil }
func (s *activatorTestService) Delete(context.Context, string) error        { return nil }
func (s *activatorTestService) RecordUsage(_ context.Context, id string, _ bool) error {
	if s.recorded == nil {
		s.recorded = map[string]int{}
	}
	s.recorded[id]++
	return nil
}
func (s *activatorTestService) ImportFromDir(context.Context, string) (int, []error) { return 0, nil }
func (s *activatorTestService) Reindex(context.Context) error                        { return nil }
func (s *activatorTestService) SetLLMCaller(LLMCaller)                               {}

func TestPathsActivator_ActivateByPaths(t *testing.T) {
	svc := &activatorTestService{
		metas: []SkillMeta{
			{ID: "workflow/docs", Paths: []string{"docs/**/*.md"}},
			{ID: "workflow/skillbank", Paths: []string{"internal/skillbank/**"}},
			{ID: "workflow/none"},
		},
	}
	activator := NewPathsActivator(svc)

	activated, err := activator.ActivateByPaths(context.Background(), "/repo", []string{
		"/repo/docs/guide/intro.md",
		"/repo/internal/skillbank/service.go",
	})
	if err != nil {
		t.Fatalf("activate: %v", err)
	}
	if activated != 2 {
		t.Fatalf("activated=%d, want 2", activated)
	}

	got := make([]string, 0, len(svc.recorded))
	for id := range svc.recorded {
		got = append(got, id)
	}
	sort.Strings(got)
	want := []string{"workflow/docs", "workflow/skillbank"}
	if len(got) != len(want) {
		t.Fatalf("recorded ids=%v, want=%v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("recorded ids=%v, want=%v", got, want)
		}
	}
}

func TestPathsActivator_DedupSameSkill(t *testing.T) {
	svc := &activatorTestService{
		metas: []SkillMeta{
			{ID: "workflow/go", Paths: []string{"**/*.go"}},
		},
	}
	activator := NewPathsActivator(svc)

	activated, err := activator.ActivateByPaths(context.Background(), "/repo", []string{
		"/repo/internal/a.go",
		"/repo/internal/b.go",
	})
	if err != nil {
		t.Fatalf("activate: %v", err)
	}
	if activated != 1 {
		t.Fatalf("activated=%d, want 1", activated)
	}
	if svc.recorded["workflow/go"] != 1 {
		t.Fatalf("usage count=%d, want 1", svc.recorded["workflow/go"])
	}
}

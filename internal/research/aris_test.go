package research

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNormalizeResearchRunPolicy_DerivesSubmissionForMaxEffort(t *testing.T) {
	p := NormalizeResearchRunPolicy(ResearchRunPolicy{Effort: "max"})
	if p.Assurance != "submission" {
		t.Fatalf("expected submission assurance, got %q", p.Assurance)
	}
}

func TestDefaultResearchRunPolicy_MatchesARISDefaults(t *testing.T) {
	p := DefaultResearchRunPolicy()
	if !p.AutoProceed || p.HumanCheckpoint {
		t.Fatalf("unexpected checkpoint defaults: %#v", p)
	}
	if p.Effort != "balanced" || p.Assurance != "draft" || p.ReviewerDifficulty != "medium" {
		t.Fatalf("unexpected policy defaults: %#v", p)
	}
	if p.MaxReviewRounds != 4 || p.BatchPolicy != "auto" || p.TraceMode != "full" {
		t.Fatalf("unexpected run control defaults: %#v", p)
	}
}

func TestInitARISWorkspace_CreatesLayoutAndRunConfig(t *testing.T) {
	dir := t.TempDir()
	if err := InitARISWorkspace(dir, ResearchRunPolicy{Effort: "balanced"}); err != nil {
		t.Fatalf("init aris workspace: %v", err)
	}
	for _, rel := range []string{
		".aris", ".aris/traces", "idea-stage", "refine-logs", "experiments", "experiments/nodes", "review-stage", "research-wiki", ".handoff", "paper", "paper/sections", "code",
	} {
		if st, err := os.Stat(filepath.Join(dir, rel)); err != nil || !st.IsDir() {
			t.Fatalf("expected dir %s", rel)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, ".aris", "run_config.json")); err != nil {
		t.Fatalf("run config missing: %v", err)
	}
	manifest, err := os.ReadFile(filepath.Join(dir, "MANIFEST.md"))
	if err != nil {
		t.Fatalf("manifest missing: %v", err)
	}
	if !strings.Contains(string(manifest), ".aris/run_config.json") {
		t.Fatalf("manifest missing run config entry:\n%s", manifest)
	}
}

func TestInitARISWorkspace_RejectsSymlinkedARISDir(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(dir, ".aris")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	if err := InitARISWorkspace(dir, ResearchRunPolicy{}); err == nil {
		t.Fatal("expected symlinked .aris directory to be rejected")
	}
}

func TestWriteVersionedArtifact_WritesVersionedAndLatest(t *testing.T) {
	dir := t.TempDir()
	versioned, latest, err := WriteVersionedArtifact(dir, "review-stage/result.json", []byte(`{"ok":true}`), time.Date(2026, 4, 29, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("write versioned artifact: %v", err)
	}
	if !strings.Contains(versioned, "20260429T100000Z") {
		t.Fatalf("unexpected versioned path: %s", versioned)
	}
	if latest != "review-stage/result.json" {
		t.Fatalf("unexpected latest path: %s", latest)
	}
	if _, err := os.Stat(filepath.Join(dir, versioned)); err != nil {
		t.Fatalf("versioned file missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, latest)); err != nil {
		t.Fatalf("latest file missing: %v", err)
	}
}

func TestWriteVersionedArtifact_RejectsSymlinkPathComponent(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(dir, "link")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	_, _, err := WriteVersionedArtifact(dir, "link/result.json", []byte(`{"ok":true}`), time.Date(2026, 4, 29, 10, 0, 0, 0, time.UTC))
	if err == nil {
		t.Fatal("expected symlink path component to be rejected")
	}
}

func TestAppendOutputManifest_RejectsSymlinkedManifest(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "MANIFEST.md")
	if err := os.WriteFile(outside, []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "MANIFEST.md")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	err := AppendOutputManifest(dir, OutputManifestEntry{Phase: "phase", ArtifactPath: "artifact.md"})
	if err == nil {
		t.Fatal("expected symlinked manifest to be rejected")
	}
}

func TestAppendOutputManifest_AppendsDeterministicRows(t *testing.T) {
	dir := t.TempDir()
	err := AppendOutputManifest(dir, OutputManifestEntry{
		TimestampUTC: time.Date(2026, 4, 29, 10, 0, 0, 0, time.UTC),
		Phase:        "phase|one",
		ArtifactPath: "artifact.md",
		Note:         "ok",
	})
	if err != nil {
		t.Fatalf("append manifest: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(dir, "MANIFEST.md"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	got := string(body)
	if !strings.Contains(got, "2026-04-29T10:00:00Z") || !strings.Contains(got, "phase/one") || !strings.Contains(got, "artifact.md") {
		t.Fatalf("manifest row not written as expected:\n%s", got)
	}
}

func TestTemplates_ContainARISEmpirical(t *testing.T) {
	phases, ok := Templates["aris_empirical"]
	if !ok {
		t.Fatal("expected aris_empirical template")
	}
	if len(phases) != 7 {
		t.Fatalf("expected 7 ARIS phases, got %d", len(phases))
	}
	for _, phase := range phases {
		for _, deliverable := range phase.Deliverables {
			if strings.Contains(deliverable, "**") {
				t.Fatalf("ARIS deliverable should not use recursive glob %q", deliverable)
			}
		}
	}
}

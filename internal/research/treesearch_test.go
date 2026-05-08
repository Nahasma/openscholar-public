package research

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// memNodeStore 内存实现，用于测试
type memNodeStore struct {
	nodes []*ExperimentNode
}

func (s *memNodeStore) CreateNode(node *ExperimentNode) error {
	s.nodes = append(s.nodes, node)
	return nil
}

func (s *memNodeStore) UpdateNode(node *ExperimentNode) error {
	for i, n := range s.nodes {
		if n.ID == node.ID {
			s.nodes[i] = node
			return nil
		}
	}
	return nil
}

func (s *memNodeStore) GetNodesByPipeline(pipelineID string) ([]*ExperimentNode, error) {
	var result []*ExperimentNode
	for _, n := range s.nodes {
		if n.PipelineID == pipelineID {
			result = append(result, n)
		}
	}
	return result, nil
}

func (s *memNodeStore) GetNodeByID(id string) (*ExperimentNode, error) {
	for _, n := range s.nodes {
		if n.ID == id {
			return n, nil
		}
	}
	return nil, nil
}

func newTestTreeSearch(t *testing.T) *TreeSearch {
	t.Helper()
	dir := t.TempDir()
	store := &memNodeStore{}
	config := DefaultTreeSearchConfig()
	return NewTreeSearch("pipeline-1", config, store, dir)
}

func TestTreeSearch_CreateRoot(t *testing.T) {
	ts := newTestTreeSearch(t)
	root, err := ts.CreateRoot("baseline experiment")
	require.NoError(t, err)
	assert.NotEmpty(t, root.ID)
	assert.Equal(t, NodePreliminary, root.Type)
	assert.Equal(t, NodePending, root.Status)
	assert.Equal(t, 0, root.Depth)
	assert.NotEmpty(t, root.CodePath)
	assert.DirExists(t, root.CodePath)
}

func TestTreeSearch_Expand_FromCompleted(t *testing.T) {
	ts := newTestTreeSearch(t)
	root, _ := ts.CreateRoot("baseline")
	root.Status = NodeCompleted
	root.Score = 7.0
	ts.store.UpdateNode(root)

	candidates, err := ts.Expand()
	require.NoError(t, err)
	assert.Len(t, candidates, 1)
	assert.Equal(t, NodeHyperparameter, candidates[0].Type) // depth 0 → hyperparameter
	assert.Equal(t, root.ID, candidates[0].ParentID)
	assert.Equal(t, 1, candidates[0].Depth)
}

func TestTreeSearch_Expand_BuggyPriority(t *testing.T) {
	ts := newTestTreeSearch(t)
	root, _ := ts.CreateRoot("baseline")
	root.Status = NodeBuggy
	root.ErrorLog = "ImportError: no module"
	ts.store.UpdateNode(root)

	candidates, err := ts.Expand()
	require.NoError(t, err)
	assert.Len(t, candidates, 1)
	assert.Equal(t, NodeDebug, candidates[0].Type)
}

func TestTreeSearch_Expand_MaxNodes(t *testing.T) {
	ts := newTestTreeSearch(t)
	ts.config.MaxNodes = 1
	ts.CreateRoot("baseline")

	_, err := ts.Expand()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "max nodes limit")
}

func TestTreeSearch_Expand_MaxDepth(t *testing.T) {
	ts := newTestTreeSearch(t)
	ts.config.MaxDepth = 1
	root, _ := ts.CreateRoot("baseline")
	root.Status = NodeCompleted
	root.Score = 8.0
	ts.store.UpdateNode(root)

	// Expand should create depth=1 child
	candidates, err := ts.Expand()
	require.NoError(t, err)
	assert.Len(t, candidates, 1)
	child := candidates[0]
	child.Status = NodeCompleted
	child.Score = 9.0
	ts.store.UpdateNode(child)

	// Expand again — depth=1 is max, no more expansion
	candidates, err = ts.Expand()
	require.NoError(t, err)
	assert.Len(t, candidates, 0)
}

func TestTreeSearch_SelectBest(t *testing.T) {
	ts := newTestTreeSearch(t)
	root, _ := ts.CreateRoot("baseline")
	root.Status = NodeCompleted
	root.Score = 6.0
	ts.store.UpdateNode(root)

	// Manually add a second completed node
	child := &ExperimentNode{
		ID: "node-better", ParentID: root.ID, PipelineID: "pipeline-1",
		Type: NodeHyperparameter, Status: NodeCompleted, Score: 9.0,
	}
	ts.store.CreateNode(child)

	best, err := ts.SelectBest()
	require.NoError(t, err)
	assert.Equal(t, "node-better", best.ID)
	assert.Equal(t, 9.0, best.Score)
}

func TestTreeSearch_SelectBest_NoCompleted(t *testing.T) {
	ts := newTestTreeSearch(t)
	ts.CreateRoot("baseline") // pending, not completed

	_, err := ts.SelectBest()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no completed nodes")
}

func TestTreeSearch_Prune(t *testing.T) {
	ts := newTestTreeSearch(t)
	root, _ := ts.CreateRoot("baseline")
	root.Status = NodeCompleted
	root.Score = 3.0
	ts.store.UpdateNode(root)

	good := &ExperimentNode{
		ID: "node-good", PipelineID: "pipeline-1",
		Status: NodeCompleted, Score: 8.0,
	}
	ts.store.CreateNode(good)

	pruned, err := ts.Prune(5.0)
	require.NoError(t, err)
	assert.Equal(t, 1, pruned)

	// Check root is pruned
	nodes, _ := ts.GetTree()
	for _, n := range nodes {
		if n.ID == root.ID {
			assert.Equal(t, NodePruned, n.Status)
		}
		if n.ID == "node-good" {
			assert.Equal(t, NodeCompleted, n.Status)
		}
	}
}

func TestTreeSearch_NodeCount(t *testing.T) {
	ts := newTestTreeSearch(t)
	count, _ := ts.NodeCount()
	assert.Equal(t, 0, count)

	ts.CreateRoot("baseline")
	count, _ = ts.NodeCount()
	assert.Equal(t, 1, count)
}

func TestNextNodeType(t *testing.T) {
	// ML default (nil progression)
	assert.Equal(t, NodeHyperparameter, nextNodeType(0, nil))
	assert.Equal(t, NodeResearchAgenda, nextNodeType(1, nil))
	assert.Equal(t, NodeAblation, nextNodeType(2, nil))
	assert.Equal(t, NodeReplication, nextNodeType(3, nil))
	assert.Equal(t, NodeReplication, nextNodeType(10, nil))

	// Custom progression
	cfd := []string{"mesh_sensitivity", "research", "validation"}
	assert.Equal(t, NodeType("mesh_sensitivity"), nextNodeType(0, cfd))
	assert.Equal(t, NodeType("validation"), nextNodeType(2, cfd))
	assert.Equal(t, NodeType("validation"), nextNodeType(5, cfd))
}

func TestExperimentNode_IsLeaf(t *testing.T) {
	parent := &ExperimentNode{ID: "parent"}
	child := &ExperimentNode{ID: "child", ParentID: "parent"}
	allNodes := []*ExperimentNode{parent, child}

	assert.False(t, parent.IsLeaf(allNodes))
	assert.True(t, child.IsLeaf(allNodes))
}

func TestExperimentNode_IsBuggy(t *testing.T) {
	n := &ExperimentNode{Status: NodeBuggy}
	assert.True(t, n.IsBuggy())
	n.Status = NodeCompleted
	assert.False(t, n.IsBuggy())
}

func TestExperimentNode_IsTerminal(t *testing.T) {
	for _, status := range []NodeStatus{NodeCompleted, NodeBuggy, NodePruned} {
		n := &ExperimentNode{Status: status}
		assert.True(t, n.IsTerminal())
	}
	for _, status := range []NodeStatus{NodePending, NodeRunning} {
		n := &ExperimentNode{Status: status}
		assert.False(t, n.IsTerminal())
	}
}

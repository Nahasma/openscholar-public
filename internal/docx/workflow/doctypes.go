package workflow

import "sort"

// PhaseSpec describes one phase in a document pipeline.
type PhaseSpec struct {
	Name       string   // Phase name
	Checkpoint bool     // Whether to pause for human approval
	Tasks      []string // Task descriptions within this phase
}

// PipelineTemplates maps document types to their pipeline phase sequences.
var PipelineTemplates = map[string][]PhaseSpec{
	"patent": {
		{Name: "材料解析", Checkpoint: false, Tasks: []string{"读取交底书", "抽取发明点", "术语表构建"}},
		{Name: "结构生成", Checkpoint: true, Tasks: []string{"权利要求", "说明书", "摘要", "附图说明"}},
		{Name: "格式校验", Checkpoint: false, Tasks: []string{"编号检查", "术语一致性", "引用完整性"}},
		{Name: "人工审阅", Checkpoint: true, Tasks: []string{"建议模式输出", "审阅意见收集"}},
		{Name: "导出交付", Checkpoint: false, Tasks: []string{"DOCX导出", "PDF导出", "归档"}},
	},
	"paper": {
		{Name: "材料解析", Checkpoint: false, Tasks: []string{"读取参考文献", "抽取研究方法", "数据整理"}},
		{Name: "结构生成", Checkpoint: true, Tasks: []string{"摘要", "引言", "方法", "实验", "结论"}},
		{Name: "格式校验", Checkpoint: false, Tasks: []string{"引用检查", "图表编号", "摘要字数"}},
		{Name: "导出交付", Checkpoint: false, Tasks: []string{"DOCX导出", "PDF导出"}},
	},
	"proposal": {
		{Name: "材料解析", Checkpoint: false, Tasks: []string{"读取指南", "字段提取", "预算整理"}},
		{Name: "结构生成", Checkpoint: true, Tasks: []string{"项目概述", "技术路线", "预期成果", "经费预算"}},
		{Name: "格式校验", Checkpoint: false, Tasks: []string{"字段完整性", "预算一致性", "附件齐全"}},
		{Name: "人工审阅", Checkpoint: true, Tasks: []string{"建议模式输出", "审阅意见收集"}},
		{Name: "导出交付", Checkpoint: false, Tasks: []string{"DOCX导出", "PDF导出", "归档"}},
	},
}

// GetTemplate returns the pipeline template for the given document type.
// Returns nil if the doc type is not recognized.
func GetTemplate(docType string) []PhaseSpec {
	t, ok := PipelineTemplates[docType]
	if !ok {
		return nil
	}
	// Return a copy to prevent mutation of the template.
	result := make([]PhaseSpec, len(t))
	for i, p := range t {
		result[i] = PhaseSpec{
			Name:       p.Name,
			Checkpoint: p.Checkpoint,
			Tasks:      append([]string(nil), p.Tasks...),
		}
	}
	return result
}

// SupportedDocTypes returns all document types that have pipeline templates.
func SupportedDocTypes() []string {
	types := make([]string, 0, len(PipelineTemplates))
	for k := range PipelineTemplates {
		types = append(types, k)
	}
	sort.Strings(types)
	return types
}

package modules

// VLMReviewModule 图表质量审查提示词
const VLMReviewModule = `
## 图表质量审查

在实验和写作阶段，对生成的图表执行以下质量检查：

### 实验阶段（每次生成图表后）
1. 检查坐标轴标签是否清晰、单位是否标注
2. 检查图例是否完整、位置是否合理
3. 检查数据呈现是否清晰（不遮挡、不误导）
4. 检查颜色方案是否适合黑白印刷
5. 检查分辨率是否足够（≥300 DPI）

如果发现严重问题（缺少标签、数据遮挡），修复后重新生成。
如果是轻微问题（颜色偏好），记录到实验日志但不阻塞。

### 写作阶段（引用 figure 后）
1. 检查 caption 是否准确描述图表内容
2. 检查 caption 是否提及关键发现
3. 检查正文引用是否与图表内容一致
4. 检查图表编号是否连续、交叉引用是否正确

对每张图表生成简要审查备注，附在 caption 注释中。
`

// ShouldInjectVLMReview 判断是否注入 VLM 审查模块
func ShouldInjectVLMReview(isResearchMode bool, vlmEnabled bool) bool {
	return isResearchMode && vlmEnabled
}

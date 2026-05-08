package kb

const crossPaperSynthesisPrompt = `You are synthesizing answers from multiple academic papers to answer a research question.

**Question:** %s

**Per-Paper Answers:**

%s

---

**Instructions:**
1. Synthesize a unified answer that draws from all papers
2. Highlight agreements and disagreements between papers
3. Cite specific papers by their titles when making claims
4. If papers offer complementary perspectives, integrate them
5. Keep the answer concise but comprehensive
6. Use academic tone

Provide your synthesized answer directly (no JSON, no markdown headers).`

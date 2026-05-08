package kb

const treeSearchPrompt = `You are given a question and a tree structure of a document.
Each node contains a node_id, title, and summary of its content.
Your task is to find all nodes that are most likely to contain the answer to the question.

Document structure:
%s

Question: %s

Output a JSON object with:
- "thinking": your reasoning process for selecting nodes
- "node_list": array of node_id strings that likely contain the answer

Example output:
{"thinking": "The question asks about experimental results, which are typically in the Experiments section...", "node_list": ["0015", "0016"]}

Output JSON only, no other text.`

const treeSearchRepairPrompt = `The previous response could not be parsed as the required JSON object.

Question: %s

Previous response snippet:
%s

Return ONLY one valid JSON object with this exact shape:
{"thinking":"brief reason","node_list":["node_id"]}

If no node is relevant, return:
{"thinking":"no relevant node found","node_list":[]}`

const answerGenerationPrompt = `Answer the following question based ONLY on the provided context.
Include page references for every factual claim in the format (p. X) or (pp. X-Y).

Question: %s

Context (from paper "%s"):
%s

Requirements:
- Answer ONLY based on the provided context
- Include page references for every claim
- If the context doesn't contain the answer, say so explicitly
- Be concise but complete`

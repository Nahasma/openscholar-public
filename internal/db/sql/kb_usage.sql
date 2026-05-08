-- name: InsertUsageStat :exec
INSERT INTO kb_usage_stats (paper_id, node_id, query, search_type, created_at)
VALUES (?, ?, ?, ?, ?);

-- name: GetTopAccessedPapers :many
SELECT paper_id, COUNT(*) as access_count
FROM kb_usage_stats
WHERE paper_id IS NOT NULL
GROUP BY paper_id
ORDER BY access_count DESC
LIMIT ?;

-- name: CountKBUsageStats :one
SELECT COUNT(*) FROM kb_usage_stats;

-- name: CountNodeSummaries :one
SELECT COUNT(*) FROM node_summaries;

-- name: UpsertPaperRelation :exec
INSERT INTO paper_relations (paper_id_a, paper_id_b, relation_type, weight, updated_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(paper_id_a, paper_id_b, relation_type) DO UPDATE SET
    weight = weight + excluded.weight,
    updated_at = excluded.updated_at;

-- name: GetPaperRelations :many
SELECT * FROM paper_relations
WHERE paper_id_a = ? OR paper_id_b = ?
ORDER BY weight DESC
LIMIT ?;

-- name: InsertResearchTopic :exec
INSERT INTO research_topics (name, description, paper_ids, created_at, updated_at)
VALUES (?, ?, ?, ?, ?);

-- name: ListResearchTopics :many
SELECT * FROM research_topics ORDER BY updated_at DESC LIMIT ?;

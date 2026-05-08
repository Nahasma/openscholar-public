-- name: UpsertMemorySkillStat :exec
INSERT INTO memory_skill_stats (skill_name, usage_count, success_count, updated_at)
VALUES (?, 1, ?, ?)
ON CONFLICT(skill_name) DO UPDATE SET
    usage_count = usage_count + 1,
    success_count = success_count + excluded.success_count,
    updated_at = excluded.updated_at;

-- name: GetMemorySkillStats :many
SELECT * FROM memory_skill_stats ORDER BY usage_count DESC;

-- name: InsertPaper :exec
INSERT INTO papers (
    paper_id, title, authors, year, venue, abstract, doi, arxiv_id, pdf_path, indexed_at, file_path, doc_type
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetPaper :one
SELECT * FROM papers WHERE paper_id = ?;

-- name: GetPaperByDOI :one
SELECT * FROM papers WHERE doi = ? LIMIT 1;

-- name: GetPaperByArxivID :one
SELECT * FROM papers WHERE arxiv_id = ? LIMIT 1;

-- name: ListPapers :many
SELECT * FROM papers ORDER BY indexed_at DESC LIMIT ? OFFSET ?;

-- name: ListPapersWithIndexStateAndTask :many
SELECT
    p.paper_id,
    p.title,
    p.authors,
    p.year,
    p.venue,
    p.abstract,
    p.doi,
    p.arxiv_id,
    p.pdf_path,
    p.indexed_at,
    p.file_path,
    p.doc_type,
    pis.index_level,
    pis.raw_parse_status,
    pis.content_available,
    pis.content_bytes,
    pis.flat_page_index_available,
    pis.fts_status,
    pis.semantic_tree_status,
    pis.semantic_tree_available,
    pis.semantic_tree_error,
    pis.semantic_tree_task_id,
    pis.extractor,
    pis.fallback_reason,
    pis.total_pages,
    pis.total_tokens,
    pis.source_file_available,
    pis.file_hash,
    pis.updated_at AS state_updated_at,
    COALESCE(
        (SELECT task_id FROM kb_tasks kt WHERE kt.paper_id = p.paper_id AND kt.status IN ('queued', 'running') ORDER BY kt.updated_at DESC, kt.created_at DESC LIMIT 1),
        (SELECT task_id FROM kb_tasks kt WHERE kt.paper_id = p.paper_id ORDER BY kt.updated_at DESC, kt.created_at DESC LIMIT 1)
    ) AS task_id,
    COALESCE(
        (SELECT task_type FROM kb_tasks kt WHERE kt.paper_id = p.paper_id AND kt.status IN ('queued', 'running') ORDER BY kt.updated_at DESC, kt.created_at DESC LIMIT 1),
        (SELECT task_type FROM kb_tasks kt WHERE kt.paper_id = p.paper_id ORDER BY kt.updated_at DESC, kt.created_at DESC LIMIT 1)
    ) AS task_type,
    COALESCE(
        (SELECT status FROM kb_tasks kt WHERE kt.paper_id = p.paper_id AND kt.status IN ('queued', 'running') ORDER BY kt.updated_at DESC, kt.created_at DESC LIMIT 1),
        (SELECT status FROM kb_tasks kt WHERE kt.paper_id = p.paper_id ORDER BY kt.updated_at DESC, kt.created_at DESC LIMIT 1)
    ) AS task_status,
    COALESCE(
        (SELECT progress FROM kb_tasks kt WHERE kt.paper_id = p.paper_id AND kt.status IN ('queued', 'running') ORDER BY kt.updated_at DESC, kt.created_at DESC LIMIT 1),
        (SELECT progress FROM kb_tasks kt WHERE kt.paper_id = p.paper_id ORDER BY kt.updated_at DESC, kt.created_at DESC LIMIT 1)
    ) AS task_progress,
    COALESCE(
        (SELECT error FROM kb_tasks kt WHERE kt.paper_id = p.paper_id AND kt.status IN ('queued', 'running') ORDER BY kt.updated_at DESC, kt.created_at DESC LIMIT 1),
        (SELECT error FROM kb_tasks kt WHERE kt.paper_id = p.paper_id ORDER BY kt.updated_at DESC, kt.created_at DESC LIMIT 1)
    ) AS task_error,
    COALESCE(
        (SELECT updated_at FROM kb_tasks kt WHERE kt.paper_id = p.paper_id AND kt.status IN ('queued', 'running') ORDER BY kt.updated_at DESC, kt.created_at DESC LIMIT 1),
        (SELECT updated_at FROM kb_tasks kt WHERE kt.paper_id = p.paper_id ORDER BY kt.updated_at DESC, kt.created_at DESC LIMIT 1)
    ) AS task_updated_at
FROM papers p
LEFT JOIN paper_index_states pis ON pis.paper_id = p.paper_id
ORDER BY p.indexed_at DESC
LIMIT ? OFFSET ?;

-- name: SearchPapersByTitle :many
SELECT * FROM papers
WHERE title LIKE '%' || ? || '%'
ORDER BY year DESC
LIMIT ?;

-- name: DeletePaper :exec
DELETE FROM papers WHERE paper_id = ?;

-- name: CountPapers :one
SELECT COUNT(*) FROM papers;

-- name: GetPaperByFilePath :one
SELECT * FROM papers WHERE file_path = ? LIMIT 1;

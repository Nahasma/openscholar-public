-- name: CreateBibEntry :exec
INSERT INTO bib_entries (
    id, session_id, title, authors, year, venue, abstract,
    doi, arxiv_id, semantic_scholar_id, cite_key, bib_type,
    created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetBibEntry :one
SELECT * FROM bib_entries WHERE id = ?;

-- name: GetBibEntryByCiteKey :one
SELECT * FROM bib_entries WHERE cite_key = ? AND session_id = ?;

-- name: GetBibEntryByDOI :one
SELECT * FROM bib_entries WHERE doi = ? AND session_id = ?;

-- name: ListBibEntriesBySession :many
SELECT * FROM bib_entries WHERE session_id = ? ORDER BY created_at DESC;

-- name: SearchBibEntriesByTitle :many
SELECT * FROM bib_entries
WHERE session_id = ? AND title LIKE '%' || ? || '%'
ORDER BY year DESC
LIMIT ?;

-- name: DeleteBibEntry :exec
DELETE FROM bib_entries WHERE id = ?;

-- name: DeleteBibEntriesBySession :exec
DELETE FROM bib_entries WHERE session_id = ?;

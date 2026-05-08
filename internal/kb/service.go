package kb

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/openscholar/openscholar/internal/db"
)

// Service is the full KB service interface.
type Service interface {
	PaperStore
	PaperSearch
	PaperAnalyzer
}

// Indexer builds tree structures from documents (PDF, DOCX, PPTX, XLSX).
type Indexer interface {
	BuildTree(ctx context.Context, filePath string, opts IndexOptions) (*IndexResult, error)
}

type service struct {
	q   db.Querier
	db  *sql.DB      // raw DB for FTS5 queries
	fts *FTSSearcher // FTS5 full-text search
}

// NewService creates a new knowledge base service.
func NewService(q db.Querier, rawDB ...*sql.DB) Service {
	s := &service{q: q}
	if len(rawDB) > 0 && rawDB[0] != nil {
		s.db = rawDB[0]
		s.fts = NewFTSSearcher(rawDB[0])
	}
	return s
}

func (s *service) AddPaper(ctx context.Context, paper Paper, tree PaperTree) error {
	return s.addPaperWithIndexMetadata(ctx, paper, tree, IndexMetadataFromTree(tree))
}

// AddParsedPaper stores paper metadata + raw chunks + index state in one transaction when raw DB is available.
func (s *service) AddParsedPaper(ctx context.Context, paper Paper, parseResult ParseResult, semanticTreeStatus string) (PaperIndexState, error) {
	if paper.PaperID == "" {
		paper.PaperID = uuid.New().String()
	}
	now := time.Now().Unix()
	if parseResult.PaperTitle != "" && paper.Title == "" {
		paper.Title = parseResult.PaperTitle
	}
	if parseResult.DocType != "" && paper.DocType == "" {
		paper.DocType = parseResult.DocType
	}
	if paper.DocType == "pdf" && paper.PDFPath == "" {
		paper.PDFPath = paper.FilePath
	}
	if semanticTreeStatus == "" {
		semanticTreeStatus = "not_requested"
	}

	contentAvailable := false
	flatPageIndexAvailable := false
	totalTokens := 0
	contentBytes := 0
	for i := range parseResult.Chunks {
		ch := parseResult.Chunks[i]
		if strings.TrimSpace(ch.Content) != "" {
			contentAvailable = true
			contentBytes += len(ch.Content)
		}
		if ch.Kind == "page" {
			flatPageIndexAvailable = true
		}
		totalTokens += ch.TokenCount
	}
	if parseResult.ContentBytes <= 0 {
		parseResult.ContentBytes = contentBytes
	}
	if parseResult.TotalTokens <= 0 {
		parseResult.TotalTokens = totalTokens
	}
	if parseResult.TotalPages <= 0 {
		maxPage := 0
		for _, ch := range parseResult.Chunks {
			if ch.PageEnd > maxPage {
				maxPage = ch.PageEnd
			}
		}
		parseResult.TotalPages = maxPage
	}

	indexLevel := IndexLevelSummaryOnly
	if contentAvailable {
		indexLevel = IndexLevelSimpleFullText
	}
	rawStatus := "ready"
	if !contentAvailable && len(parseResult.Chunks) > 0 {
		rawStatus = "partial"
	}
	if len(parseResult.Chunks) == 0 {
		rawStatus = "failed"
	}
	ftsStatus := "pending"
	if len(parseResult.Chunks) > 0 {
		ftsStatus = "ready"
	}

	if s.db != nil {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return PaperIndexState{}, fmt.Errorf("begin transaction: %w", err)
		}
		defer tx.Rollback() //nolint:errcheck
		qtx := s.q.(*db.Queries).WithTx(tx)

		authorsJSON, _ := json.Marshal(paper.Authors)
		if err := qtx.InsertPaper(ctx, db.InsertPaperParams{
			PaperID:   paper.PaperID,
			Title:     paper.Title,
			Authors:   toNullString(string(authorsJSON)),
			Year:      toNullInt64(int64(paper.Year)),
			Venue:     toNullString(paper.Venue),
			Abstract:  toNullString(paper.Abstract),
			Doi:       toNullString(paper.DOI),
			ArxivID:   toNullString(paper.ArxivID),
			PdfPath:   toNullString(paper.PDFPath),
			IndexedAt: sql.NullInt64{Int64: now, Valid: true},
			FilePath:  toNullString(paper.FilePath),
			DocType:   toNullString(paper.DocType),
		}); err != nil {
			return PaperIndexState{}, fmt.Errorf("insert paper: %w", err)
		}

		for i := range parseResult.Chunks {
			ch := parseResult.Chunks[i]
			if ch.ChunkID == "" {
				ch.ChunkID = fmt.Sprintf("chunk_%d", i+1)
			}
			if ch.Kind == "" {
				ch.Kind = "section"
			}
			if err := qtx.InsertPaperChunk(ctx, db.InsertPaperChunkParams{
				PaperID:    paper.PaperID,
				ChunkID:    ch.ChunkID,
				Kind:       ch.Kind,
				PageStart:  sql.NullInt64{Int64: int64(ch.PageStart), Valid: ch.PageStart > 0},
				PageEnd:    sql.NullInt64{Int64: int64(ch.PageEnd), Valid: ch.PageEnd > 0},
				Title:      toNullString(ch.Title),
				Content:    ch.Content,
				TokenCount: sql.NullInt64{Int64: int64(ch.TokenCount), Valid: ch.TokenCount > 0},
				Source:     toNullString(ch.Source),
				CreatedAt:  now,
			}); err != nil {
				return PaperIndexState{}, fmt.Errorf("insert paper chunk %s: %w", ch.ChunkID, err)
			}
		}

		if err := qtx.UpsertPaperIndexState(ctx, db.UpsertPaperIndexStateParams{
			PaperID:                paper.PaperID,
			IndexLevel:             toNullString(indexLevel),
			RawParseStatus:         toNullString(rawStatus),
			ContentAvailable:       boolToInt64(contentAvailable),
			ContentBytes:           int64(parseResult.ContentBytes),
			FlatPageIndexAvailable: boolToInt64(flatPageIndexAvailable),
			FtsStatus:              toNullString(ftsStatus),
			SemanticTreeStatus:     toNullString(semanticTreeStatus),
			SemanticTreeAvailable:  0,
			SemanticTreeError:      sql.NullString{},
			SemanticTreeTaskID:     sql.NullString{},
			Extractor:              toNullString(parseResult.Extractor),
			FallbackReason:         sql.NullString{},
			TotalPages:             sql.NullInt64{Int64: int64(parseResult.TotalPages), Valid: parseResult.TotalPages > 0},
			TotalTokens:            sql.NullInt64{Int64: int64(parseResult.TotalTokens), Valid: parseResult.TotalTokens > 0},
			SourceFileAvailable:    sql.NullInt64{Int64: boolToInt64(paper.FilePath != ""), Valid: true},
			FileHash:               sql.NullString{},
			UpdatedAt:              now,
		}); err != nil {
			return PaperIndexState{}, fmt.Errorf("upsert paper index state: %w", err)
		}

		if err := tx.Commit(); err != nil {
			return PaperIndexState{}, fmt.Errorf("commit transaction: %w", err)
		}
	} else {
		authorsJSON, _ := json.Marshal(paper.Authors)
		if err := s.q.InsertPaper(ctx, db.InsertPaperParams{
			PaperID:   paper.PaperID,
			Title:     paper.Title,
			Authors:   toNullString(string(authorsJSON)),
			Year:      toNullInt64(int64(paper.Year)),
			Venue:     toNullString(paper.Venue),
			Abstract:  toNullString(paper.Abstract),
			Doi:       toNullString(paper.DOI),
			ArxivID:   toNullString(paper.ArxivID),
			PdfPath:   toNullString(paper.PDFPath),
			IndexedAt: sql.NullInt64{Int64: now, Valid: true},
			FilePath:  toNullString(paper.FilePath),
			DocType:   toNullString(paper.DocType),
		}); err != nil {
			return PaperIndexState{}, fmt.Errorf("insert paper: %w", err)
		}
		for i := range parseResult.Chunks {
			ch := parseResult.Chunks[i]
			if ch.ChunkID == "" {
				ch.ChunkID = fmt.Sprintf("chunk_%d", i+1)
			}
			if ch.Kind == "" {
				ch.Kind = "section"
			}
			if err := s.q.InsertPaperChunk(ctx, db.InsertPaperChunkParams{
				PaperID:    paper.PaperID,
				ChunkID:    ch.ChunkID,
				Kind:       ch.Kind,
				PageStart:  sql.NullInt64{Int64: int64(ch.PageStart), Valid: ch.PageStart > 0},
				PageEnd:    sql.NullInt64{Int64: int64(ch.PageEnd), Valid: ch.PageEnd > 0},
				Title:      toNullString(ch.Title),
				Content:    ch.Content,
				TokenCount: sql.NullInt64{Int64: int64(ch.TokenCount), Valid: ch.TokenCount > 0},
				Source:     toNullString(ch.Source),
				CreatedAt:  now,
			}); err != nil {
				return PaperIndexState{}, fmt.Errorf("insert paper chunk %s: %w", ch.ChunkID, err)
			}
		}
		if err := s.q.UpsertPaperIndexState(ctx, db.UpsertPaperIndexStateParams{
			PaperID:                paper.PaperID,
			IndexLevel:             toNullString(indexLevel),
			RawParseStatus:         toNullString(rawStatus),
			ContentAvailable:       boolToInt64(contentAvailable),
			ContentBytes:           int64(parseResult.ContentBytes),
			FlatPageIndexAvailable: boolToInt64(flatPageIndexAvailable),
			FtsStatus:              toNullString(ftsStatus),
			SemanticTreeStatus:     toNullString(semanticTreeStatus),
			SemanticTreeAvailable:  0,
			SemanticTreeError:      sql.NullString{},
			SemanticTreeTaskID:     sql.NullString{},
			Extractor:              toNullString(parseResult.Extractor),
			FallbackReason:         sql.NullString{},
			TotalPages:             sql.NullInt64{Int64: int64(parseResult.TotalPages), Valid: parseResult.TotalPages > 0},
			TotalTokens:            sql.NullInt64{Int64: int64(parseResult.TotalTokens), Valid: parseResult.TotalTokens > 0},
			SourceFileAvailable:    sql.NullInt64{Int64: boolToInt64(paper.FilePath != ""), Valid: true},
			FileHash:               sql.NullString{},
			UpdatedAt:              now,
		}); err != nil {
			return PaperIndexState{}, fmt.Errorf("upsert paper index state: %w", err)
		}
	}

	state := PaperIndexState{
		PaperID:                paper.PaperID,
		IndexLevel:             indexLevel,
		RawParseStatus:         rawStatus,
		ContentAvailable:       contentAvailable,
		ContentBytes:           parseResult.ContentBytes,
		FlatPageIndexAvailable: flatPageIndexAvailable,
		FTSStatus:              ftsStatus,
		SemanticTreeStatus:     semanticTreeStatus,
		SemanticTreeAvailable:  false,
		Extractor:              parseResult.Extractor,
		TotalPages:             parseResult.TotalPages,
		TotalTokens:            parseResult.TotalTokens,
		SourceFileAvailable:    paper.FilePath != "",
		UpdatedAt:              now,
	}
	return state, nil
}

// AttachSemanticTree stores semantic tree artifacts for an existing paper and updates state.
func (s *service) AttachSemanticTree(ctx context.Context, paperID string, result IndexResult) error {
	if s.db == nil {
		return fmt.Errorf("raw db is not available")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck
	qtx := s.q.(*db.Queries).WithTx(tx)
	now := time.Now().Unix()

	_ = qtx.DeletePaperTree(ctx, paperID)
	_ = qtx.DeleteNodeContents(ctx, paperID)
	_ = qtx.DeleteNodeSummaries(ctx, toNullString(paperID))

	meta := IndexMetadataFromResult(result)
	storedTree := StripTreeContent(result.Tree)
	treeJSON, _ := json.Marshal(storedTree)
	if err := qtx.InsertPaperTree(ctx, db.InsertPaperTreeParams{
		PaperID:        paperID,
		TreeJson:       string(treeJSON),
		DocDescription: toNullString(storedTree.DocDescription),
		TotalPages:     sql.NullInt64{Int64: int64(meta.TotalPages), Valid: meta.TotalPages > 0},
		TotalTokens:    sql.NullInt64{Int64: int64(meta.TotalTokens), Valid: meta.TotalTokens > 0},
		ModelUsed:      toNullString(meta.ModelUsed),
		Version:        sql.NullInt64{Int64: 1, Valid: true},
		CreatedAt:      sql.NullInt64{Int64: now, Valid: true},
		IndexMetadata:  marshalIndexMetadata(meta),
	}); err != nil {
		return fmt.Errorf("insert semantic tree: %w", err)
	}

	nodes := FlattenNodes(paperID, &result.Tree)
	for _, node := range nodes {
		if err := qtx.InsertNodeSummary(ctx, db.InsertNodeSummaryParams{
			PaperID:   toNullString(node.PaperID),
			NodeID:    toNullString(node.NodeID),
			Title:     toNullString(node.Title),
			StartPage: sql.NullInt64{Int64: int64(node.StartPage), Valid: true},
			EndPage:   sql.NullInt64{Int64: int64(node.EndPage), Valid: true},
			Summary:   node.Summary,
		}); err != nil {
			return fmt.Errorf("insert semantic node summary %s: %w", node.NodeID, err)
		}
		if node.Content != "" {
			if err := qtx.InsertNodeContent(ctx, db.InsertNodeContentParams{
				PaperID:    node.PaperID,
				NodeID:     node.NodeID,
				Content:    node.Content,
				TokenCount: sql.NullInt64{Int64: int64(estimateTokenCount(node.Content)), Valid: true},
				Source:     toNullString(meta.Extractor),
			}); err != nil {
				return fmt.Errorf("insert semantic node content %s: %w", node.NodeID, err)
			}
		}
	}

	if err := qtx.UpdateSemanticTreeState(ctx, db.UpdateSemanticTreeStateParams{
		SemanticTreeStatus:    toNullString("ready"),
		SemanticTreeAvailable: 1,
		SemanticTreeError:     sql.NullString{},
		SemanticTreeTaskID:    sql.NullString{},
		UpdatedAt:             now,
		PaperID:               paperID,
	}); err != nil {
		return fmt.Errorf("update semantic tree state: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit semantic tree transaction: %w", err)
	}
	return nil
}

// AddIndexedPaper stores a paper using the indexing metadata returned by an Indexer.
func (s *service) AddIndexedPaper(ctx context.Context, paper Paper, result IndexResult) error {
	return s.addPaperWithIndexMetadata(ctx, paper, result.Tree, IndexMetadataFromResult(result))
}

func (s *service) addPaperWithIndexMetadata(ctx context.Context, paper Paper, tree PaperTree, meta IndexMetadata) error {
	if paper.PaperID == "" {
		paper.PaperID = uuid.New().String()
	}

	// Wrap the three-step insert in a single transaction so that InsertPaper,
	// InsertPaperTree, and InsertNodeSummary are committed atomically.
	if s.db != nil {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin transaction: %w", err)
		}
		defer tx.Rollback() //nolint:errcheck

		qtx := s.q.(*db.Queries).WithTx(tx)
		if err := s.addPaperWithQuerier(ctx, qtx, paper, tree, meta); err != nil {
			return err
		}
		return tx.Commit()
	}

	// Fallback: no raw DB available (e.g. mock in tests), run without transaction.
	return s.addPaperWithQuerier(ctx, s.q, paper, tree, meta)
}

func (s *service) addPaperWithQuerier(ctx context.Context, q db.Querier, paper Paper, tree PaperTree, meta IndexMetadata) error {
	authorsJSON, _ := json.Marshal(paper.Authors)
	now := time.Now().Unix()

	// 1. Insert paper metadata
	if err := q.InsertPaper(ctx, db.InsertPaperParams{
		PaperID:   paper.PaperID,
		Title:     paper.Title,
		Authors:   toNullString(string(authorsJSON)),
		Year:      toNullInt64(int64(paper.Year)),
		Venue:     toNullString(paper.Venue),
		Abstract:  toNullString(paper.Abstract),
		Doi:       toNullString(paper.DOI),
		ArxivID:   toNullString(paper.ArxivID),
		PdfPath:   toNullString(paper.PDFPath),
		IndexedAt: sql.NullInt64{Int64: now, Valid: true},
		FilePath:  toNullString(paper.FilePath),
		DocType:   toNullString(paper.DocType),
	}); err != nil {
		return fmt.Errorf("insert paper: %w", err)
	}

	// 2. Insert tree index
	storedTree := StripTreeContent(tree)
	treeJSON, _ := json.Marshal(storedTree)
	if err := q.InsertPaperTree(ctx, db.InsertPaperTreeParams{
		PaperID:        paper.PaperID,
		TreeJson:       string(treeJSON),
		DocDescription: toNullString(storedTree.DocDescription),
		TotalPages:     sql.NullInt64{Int64: int64(meta.TotalPages), Valid: meta.TotalPages > 0},
		TotalTokens:    sql.NullInt64{Int64: int64(meta.TotalTokens), Valid: meta.TotalTokens > 0},
		ModelUsed:      toNullString(meta.ModelUsed),
		Version:        sql.NullInt64{Int64: 1, Valid: true},
		CreatedAt:      sql.NullInt64{Int64: now, Valid: true},
		IndexMetadata:  marshalIndexMetadata(meta),
	}); err != nil {
		return fmt.Errorf("insert tree: %w", err)
	}

	// 3. Insert flattened node summaries
	nodes := FlattenNodes(paper.PaperID, &tree)
	for _, node := range nodes {
		if err := q.InsertNodeSummary(ctx, db.InsertNodeSummaryParams{
			PaperID:   toNullString(node.PaperID),
			NodeID:    toNullString(node.NodeID),
			Title:     toNullString(node.Title),
			StartPage: sql.NullInt64{Int64: int64(node.StartPage), Valid: true},
			EndPage:   sql.NullInt64{Int64: int64(node.EndPage), Valid: true},
			Summary:   node.Summary,
		}); err != nil {
			return fmt.Errorf("insert node summary %s: %w", node.NodeID, err)
		}
		if node.Content != "" {
			if err := q.InsertNodeContent(ctx, db.InsertNodeContentParams{
				PaperID:    node.PaperID,
				NodeID:     node.NodeID,
				Content:    node.Content,
				TokenCount: sql.NullInt64{Int64: int64(estimateTokenCount(node.Content)), Valid: true},
				Source:     toNullString(meta.Extractor),
			}); err != nil {
				return fmt.Errorf("insert node content %s: %w", node.NodeID, err)
			}
		}
	}
	if err := s.addRawCompatibilityRows(ctx, q, paper, tree, meta, nodes, now); err != nil {
		return err
	}

	return nil
}

func (s *service) addRawCompatibilityRows(ctx context.Context, q db.Querier, paper Paper, tree PaperTree, meta IndexMetadata, nodes []NodeSummary, now int64) error {
	contentBytes := 0
	flatPageIndexAvailable := false
	for i, node := range nodes {
		kind := "section"
		if strings.HasPrefix(strings.ToLower(node.NodeID), "page_") {
			kind = "page"
			flatPageIndexAvailable = true
		}
		content := strings.TrimSpace(node.Content)
		contentBytes += len(content)
		if err := q.InsertPaperChunk(ctx, db.InsertPaperChunkParams{
			PaperID:    paper.PaperID,
			ChunkID:    nonEmpty(node.NodeID, fmt.Sprintf("chunk_%d", i+1)),
			Kind:       kind,
			PageStart:  sql.NullInt64{Int64: int64(node.StartPage), Valid: node.StartPage > 0},
			PageEnd:    sql.NullInt64{Int64: int64(node.EndPage), Valid: node.EndPage > 0},
			Title:      toNullString(node.Title),
			Content:    content,
			TokenCount: sql.NullInt64{Int64: int64(estimateTokenCount(content)), Valid: content != ""},
			Source:     toNullString(meta.Extractor),
			CreatedAt:  now,
		}); err != nil {
			return fmt.Errorf("insert compatibility paper chunk %s: %w", node.NodeID, err)
		}
	}

	if meta.ContentBytes > contentBytes && contentBytes > 0 {
		contentBytes = meta.ContentBytes
	}
	contentAvailable := contentBytes > 0
	rawStatus := "failed"
	if len(nodes) > 0 {
		rawStatus = "partial"
		if contentAvailable {
			rawStatus = "ready"
		}
	}
	ftsStatus := "missing"
	if len(nodes) > 0 {
		ftsStatus = "ready"
	}
	indexLevel := meta.IndexLevel
	if indexLevel == "" {
		indexLevel = IndexLevelUnknown
	}
	semanticAvailable := len(tree.Structure) > 0 && !isFlatPageLegacyTree(tree, meta)
	semanticStatus := "not_requested"
	if semanticAvailable {
		semanticStatus = "ready"
	}
	if err := q.UpsertPaperIndexState(ctx, db.UpsertPaperIndexStateParams{
		PaperID:                paper.PaperID,
		IndexLevel:             toNullString(indexLevel),
		RawParseStatus:         toNullString(rawStatus),
		ContentAvailable:       boolToInt64(contentAvailable),
		ContentBytes:           int64(contentBytes),
		FlatPageIndexAvailable: boolToInt64(flatPageIndexAvailable),
		FtsStatus:              toNullString(ftsStatus),
		SemanticTreeStatus:     toNullString(semanticStatus),
		SemanticTreeAvailable:  boolToInt64(semanticAvailable),
		SemanticTreeError:      sql.NullString{},
		SemanticTreeTaskID:     sql.NullString{},
		Extractor:              toNullString(meta.Extractor),
		FallbackReason:         toNullString(meta.FallbackReason),
		TotalPages:             sql.NullInt64{Int64: int64(meta.TotalPages), Valid: meta.TotalPages > 0},
		TotalTokens:            sql.NullInt64{Int64: int64(meta.TotalTokens), Valid: meta.TotalTokens > 0},
		SourceFileAvailable:    sql.NullInt64{Int64: boolToInt64(paper.FilePath != "" || paper.PDFPath != ""), Valid: true},
		FileHash:               sql.NullString{},
		UpdatedAt:              now,
	}); err != nil {
		return fmt.Errorf("upsert compatibility paper index state: %w", err)
	}
	return nil
}

func isFlatPageLegacyTree(tree PaperTree, meta IndexMetadata) bool {
	if strings.EqualFold(meta.Extractor, "simple_extract") || strings.EqualFold(meta.ModelUsed, "simple_extract") {
		return true
	}
	if strings.Contains(strings.ToLower(tree.DocDescription), "simple text extraction") {
		return true
	}
	if len(tree.Structure) == 0 {
		return false
	}
	for _, node := range tree.Structure {
		if !strings.HasPrefix(strings.ToLower(node.NodeID), "page_") {
			return false
		}
	}
	return true
}

func nonEmpty(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

func (s *service) GetPaper(ctx context.Context, paperID string) (*Paper, error) {
	row, err := s.q.GetPaper(ctx, paperID)
	if err != nil {
		return nil, err
	}
	return dbPaperToModel(row), nil
}

func (s *service) GetTree(ctx context.Context, paperID string) (*PaperTree, error) {
	row, err := s.q.GetPaperTree(ctx, paperID)
	if err != nil {
		return nil, err
	}
	return ParseTreeJSON(row.TreeJson)
}

func (s *service) GetIndexMetadata(ctx context.Context, paperID string) (IndexMetadata, error) {
	row, err := s.q.GetPaperTree(ctx, paperID)
	if err != nil {
		return IndexMetadata{}, err
	}
	contentCount, countErr := s.q.CountNodeContents(ctx, paperID)
	if strings.TrimSpace(row.IndexMetadata) != "" && strings.TrimSpace(row.IndexMetadata) != "{}" {
		var meta IndexMetadata
		if err := json.Unmarshal([]byte(row.IndexMetadata), &meta); err == nil && meta.IndexLevel != "" {
			if countErr == nil {
				meta.ContentAvailable = contentCount > 0
				if contentCount == 0 {
					meta.ContentBytes = 0
				}
			}
			if meta.ContentBytes == 0 && contentCount > 0 {
				if contents, err := s.q.GetNodeContents(ctx, paperID); err == nil {
					for _, content := range contents {
						meta.ContentBytes += len(content.Content)
					}
				}
			}
			meta.SummaryOnly = meta.IndexLevel == IndexLevelSummaryOnly
			meta.MissingCapabilities = missingCapabilities(meta.IndexLevel, meta.ContentAvailable)
			return meta, nil
		}
	}
	meta := IndexMetadataFromModelUsed(row.ModelUsed.String, int(row.TotalPages.Int64), int(row.TotalTokens.Int64), countErr == nil && contentCount > 0)
	return meta, nil
}

func (s *service) ListPapers(ctx context.Context, limit, offset int) ([]Paper, error) {
	rows, err := s.q.ListPapers(ctx, db.ListPapersParams{
		Limit:  int64(limit),
		Offset: int64(offset),
	})
	if err != nil {
		return nil, err
	}
	papers := make([]Paper, len(rows))
	for i, row := range rows {
		papers[i] = *dbPaperToModel(row)
	}
	return papers, nil
}

func (s *service) SearchByTitle(ctx context.Context, query string, limit int) ([]Paper, error) {
	rows, err := s.q.SearchPapersByTitle(ctx, db.SearchPapersByTitleParams{
		Column1: toNullString(query),
		Limit:   int64(limit),
	})
	if err != nil {
		return nil, err
	}
	papers := make([]Paper, len(rows))
	for i, row := range rows {
		papers[i] = *dbPaperToModel(row)
	}
	return papers, nil
}

func (s *service) RemovePaper(ctx context.Context, paperID string) error {
	return s.q.DeletePaper(ctx, paperID)
}

func (s *service) GetNodeSummaries(ctx context.Context, paperID string) ([]NodeSummary, error) {
	rows, err := s.q.GetNodeSummaries(ctx, toNullString(paperID))
	if err != nil {
		return nil, err
	}
	summaries := make([]NodeSummary, len(rows))
	for i, row := range rows {
		summaries[i] = NodeSummary{
			PaperID:   row.PaperID.String,
			NodeID:    row.NodeID.String,
			Title:     row.Title.String,
			StartPage: int(row.StartPage.Int64),
			EndPage:   int(row.EndPage.Int64),
			Summary:   row.Summary,
		}
	}
	return summaries, nil
}

func (s *service) GetNodeContents(ctx context.Context, paperID string) ([]NodeContent, error) {
	rows, err := s.q.GetNodeContents(ctx, paperID)
	if err != nil {
		return nil, err
	}
	contents := make([]NodeContent, len(rows))
	for i, row := range rows {
		contents[i] = NodeContent{
			PaperID:    row.PaperID,
			NodeID:     row.NodeID,
			Content:    row.Content,
			TokenCount: int(row.TokenCount.Int64),
			Source:     row.Source.String,
		}
	}
	return contents, nil
}

func (s *service) CountPapers(ctx context.Context) (int64, error) {
	return s.q.CountPapers(ctx)
}

func (s *service) FindByDOI(ctx context.Context, doi string) (*Paper, error) {
	row, err := s.q.GetPaperByDOI(ctx, toNullString(doi))
	if err != nil {
		return nil, err
	}
	return dbPaperToModel(row), nil
}

func (s *service) FindByArxivID(ctx context.Context, arxivID string) (*Paper, error) {
	row, err := s.q.GetPaperByArxivID(ctx, toNullString(arxivID))
	if err != nil {
		return nil, err
	}
	return dbPaperToModel(row), nil
}

// dbPaperToModel converts a SQLC-generated Paper row to a domain model.
func dbPaperToModel(row db.Paper) *Paper {
	var authors []Author
	if row.Authors.Valid {
		_ = json.Unmarshal([]byte(row.Authors.String), &authors)
	}

	return &Paper{
		PaperID:   row.PaperID,
		Title:     row.Title,
		Authors:   authors,
		Year:      int(row.Year.Int64),
		Venue:     row.Venue.String,
		Abstract:  row.Abstract.String,
		DOI:       row.Doi.String,
		ArxivID:   row.ArxivID.String,
		PDFPath:   row.PdfPath.String,
		FilePath:  row.FilePath.String,
		DocType:   row.DocType.String,
		IndexedAt: row.IndexedAt.Int64,
	}
}

func toNullString(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}

func toNullInt64(i int64) sql.NullInt64 {
	return sql.NullInt64{Int64: i, Valid: true}
}

func boolToInt64(v bool) int64 {
	if v {
		return 1
	}
	return 0
}

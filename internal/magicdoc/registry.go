package magicdoc

import (
	"sync"
	"time"
)

// Registry 按 sessionID + filePath 管理已识别的 MagicDoc
type Registry struct {
	mu   sync.RWMutex
	docs map[string]map[string]Document // sessionID → filePath → Document
}

// NewRegistry 创建新的 Registry 实例
func NewRegistry() *Registry {
	return &Registry{
		docs: make(map[string]map[string]Document),
	}
}

// Register 注册或更新一个 MagicDoc
func (r *Registry) Register(sessionID, filePath string, doc Document) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.docs[sessionID]; !ok {
		r.docs[sessionID] = make(map[string]Document)
	}
	// Preserve LastUpdatedAt if entry already exists
	if existing, exists := r.docs[sessionID][filePath]; exists {
		doc.LastUpdatedAt = existing.LastUpdatedAt
	}
	doc.SessionID = sessionID
	doc.Path = filePath
	r.docs[sessionID][filePath] = doc
}

// Get 获取指定 session + filePath 的 Document
func (r *Registry) Get(sessionID, filePath string) (Document, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if session, ok := r.docs[sessionID]; ok {
		doc, found := session[filePath]
		return doc, found
	}
	return Document{}, false
}

// List 列出指定 session 的所有 MagicDoc
func (r *Registry) List(sessionID string) []Document {
	r.mu.RLock()
	defer r.mu.RUnlock()

	session, ok := r.docs[sessionID]
	if !ok {
		return nil
	}

	docs := make([]Document, 0, len(session))
	for _, doc := range session {
		docs = append(docs, doc)
	}
	return docs
}

// MarkUpdated 更新指定文档的 LastUpdatedAt
func (r *Registry) MarkUpdated(sessionID, filePath string, at time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if session, ok := r.docs[sessionID]; ok {
		if doc, exists := session[filePath]; exists {
			doc.LastUpdatedAt = at
			session[filePath] = doc
		}
	}
}

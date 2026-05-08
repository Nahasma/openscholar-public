package testutil

import (
	"context"
	"testing"

	"github.com/Nahasma/openscholar-public/internal/bib"
	"github.com/Nahasma/openscholar-public/internal/message"
	"github.com/Nahasma/openscholar-public/internal/session"
)

// CreateTestSession creates a session with a default title for testing.
func CreateTestSession(t *testing.T, svc session.Service) session.Session {
	t.Helper()
	sess, err := svc.Create(context.Background(), "test session")
	if err != nil {
		t.Fatalf("CreateTestSession: %v", err)
	}
	return sess
}

// CreateTestSessionWithTitle creates a session with a custom title.
func CreateTestSessionWithTitle(t *testing.T, svc session.Service, title string) session.Session {
	t.Helper()
	sess, err := svc.Create(context.Background(), title)
	if err != nil {
		t.Fatalf("CreateTestSessionWithTitle: %v", err)
	}
	return sess
}

// CreateTestMessage creates a user message with the given text in the specified session.
func CreateTestMessage(t *testing.T, svc message.Service, sessionID string, role message.MessageRole, text string) message.Message {
	t.Helper()
	msg, err := svc.Create(context.Background(), sessionID, message.CreateMessageParams{
		Role:  role,
		Parts: []message.ContentPart{message.TextContent{Text: text}},
	})
	if err != nil {
		t.Fatalf("CreateTestMessage: %v", err)
	}
	return msg
}

// CreateTestBibEntry creates a bibliography entry in the specified session.
func CreateTestBibEntry(t *testing.T, svc bib.Service, sessionID string) bib.Entry {
	t.Helper()
	entry := bib.Entry{
		SessionID: sessionID,
		Title:     "Attention Is All You Need",
		Authors:   []bib.Author{{Name: "Vaswani, A."}, {Name: "Shazeer, N."}},
		Year:      2017,
		Venue:     "NeurIPS",
		DOI:       "10.5555/3295222.3295349",
		CiteKey:   "vaswani2017attention",
		BibType:   "inproceedings",
	}
	if err := svc.Create(context.Background(), entry); err != nil {
		t.Fatalf("CreateTestBibEntry: %v", err)
	}
	return entry
}

// CreateTestBibEntryCustom creates a bibliography entry with a custom title and DOI.
func CreateTestBibEntryCustom(t *testing.T, svc bib.Service, sessionID, title, doi, citeKey string) {
	t.Helper()
	entry := bib.Entry{
		SessionID: sessionID,
		Title:     title,
		Authors:   []bib.Author{{Name: "Test Author"}},
		Year:      2024,
		DOI:       doi,
		CiteKey:   citeKey,
		BibType:   "article",
	}
	if err := svc.Create(context.Background(), entry); err != nil {
		t.Fatalf("CreateTestBibEntryCustom: %v", err)
	}
}

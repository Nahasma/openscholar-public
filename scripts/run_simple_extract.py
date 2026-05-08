#!/usr/bin/env python3
"""
Simple PDF text extraction using PyMuPDF (fitz).
Fallback for when PageIndex is not available.
Outputs per-page text as a flat tree structure (JSON to stdout).
"""

import argparse
import json
import sys

SUMMARY_MAX_CHARS = 500
EMPTY_PAGE_MARKER = "(empty page)"


def emit_error(error_type, error_message, retryable=False):
    json.dump(
        {
            "ok": False,
            "error_type": error_type,
            "error_message": error_message,
            "retryable": retryable,
        },
        sys.stdout,
        ensure_ascii=False,
    )
    sys.stdout.write("\n")


def summarize_text(text: str) -> str:
    if not text:
        return EMPTY_PAGE_MARKER
    return text[:SUMMARY_MAX_CHARS]


def main():
    parser = argparse.ArgumentParser(description='Extract text from PDF (simple mode)')
    parser.add_argument('--pdf_path', required=True, help='Path to PDF file')
    args = parser.parse_args()

    try:
        import fitz  # PyMuPDF
    except ImportError:
        emit_error("dependency_error", "PyMuPDF (fitz) not found. Install with: pip install PyMuPDF")
        sys.exit(1)

    try:
        doc = fitz.open(args.pdf_path)
    except Exception as e:
        emit_error("pdf_open_error", f"Failed to open PDF: {e}")
        sys.exit(1)

    total_pages = len(doc)
    nodes = []
    total_tokens = 0

    for i, page in enumerate(doc):
        text = page.get_text("text").strip()
        token_estimate = len(text) // 4  # rough token estimate
        total_tokens += token_estimate

        nodes.append({
            "node_id": f"page_{i + 1}",
            "title": f"Page {i + 1}",
            "start_index": i + 1,
            "end_index": i + 1,
            "summary": summarize_text(text),
            "content": text,
        })

    doc.close()

    # Build output matching IndexResult JSON schema
    output = {
        "ok": True,
        "tree": {
            "doc_name": args.pdf_path.rsplit("/", 1)[-1].rsplit(".", 1)[0],
            "doc_description": f"Simple text extraction ({total_pages} pages)",
            "structure": nodes,
        },
        "total_pages": total_pages,
        "total_tokens": total_tokens,
        "model_used": "simple_extract",
    }

    json.dump(output, sys.stdout, ensure_ascii=False)
    sys.stdout.write("\n")


if __name__ == "__main__":
    main()

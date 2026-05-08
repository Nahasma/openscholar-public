#!/usr/bin/env python3
"""
Document parser for OpenScholar.
Extracts text and builds tree structures from DOCX, PPTX, and XLSX files.

Two modes:
  --extract: Output plain text to stdout (for View tool)
  --tree:    Output PageIndex-compatible JSON tree to stdout (for KB indexing)
"""

import argparse
import json
import sys
import os


# ---------------------------------------------------------------------------
# DOCX Parser
# ---------------------------------------------------------------------------

class DocxParser:
    """Parse .docx files using python-docx."""

    HEADING_MAP = {
        'Heading 1': 0,
        'Heading 2': 1,
        'Heading 3': 2,
        'Heading 4': 3,
    }

    def __init__(self, file_path: str):
        try:
            from docx import Document
        except ImportError:
            print("python-docx not found. Install with: pip install python-docx", file=sys.stderr)
            sys.exit(1)
        self.doc = Document(file_path)
        self.file_path = file_path

    def extract(self) -> str:
        """Extract plain text with heading markers."""
        lines = []
        for para in self.doc.paragraphs:
            style_name = para.style.name if para.style else ''
            text = para.text.strip()
            if not text:
                lines.append('')
                continue

            depth = self.HEADING_MAP.get(style_name)
            if depth is not None:
                prefix = '#' * (depth + 1)
                lines.append(f'{prefix} {text}')
            else:
                lines.append(text)
        return '\n'.join(lines)

    def build_tree(self) -> dict:
        """Build a PageIndex-compatible tree structure from document headings."""
        paragraphs = list(self.doc.paragraphs)
        if not paragraphs:
            return self._empty_tree()

        # Extract title from first heading or filename
        doc_name = os.path.splitext(os.path.basename(self.file_path))[0]
        for para in paragraphs:
            if para.style and para.style.name in self.HEADING_MAP and para.text.strip():
                doc_name = para.text.strip()
                break

        # Build nodes from heading hierarchy
        root_nodes = []
        stack = []  # (depth, node_dict)
        node_counter = [0]

        def make_node(title: str, start_idx: int) -> dict:
            node_counter[0] += 1
            return {
                'node_id': f'{node_counter[0]:04d}',
                'title': title,
                'start_index': start_idx,
                'end_index': start_idx,
                'summary': '',
                'nodes': [],
            }

        current_body = []

        for idx, para in enumerate(paragraphs):
            style_name = para.style.name if para.style else ''
            text = para.text.strip()
            depth = self.HEADING_MAP.get(style_name)

            if depth is not None and text:
                # Close current body text
                node = make_node(text, idx)

                # Find parent: pop stack until we find a shallower depth
                while stack and stack[-1][0] >= depth:
                    stack.pop()

                if stack:
                    stack[-1][1]['nodes'].append(node)
                else:
                    root_nodes.append(node)

                stack.append((depth, node))
            else:
                # Update end_index of current node
                if stack:
                    stack[-1][1]['end_index'] = idx

        # If no headings found, create a single root node
        if not root_nodes:
            node = make_node(doc_name, 0)
            node['end_index'] = len(paragraphs) - 1
            root_nodes = [node]

        return {
            'tree': {
                'doc_name': doc_name,
                'doc_description': '',
                'structure': root_nodes,
            },
            'total_pages': len(paragraphs),
            'total_tokens': sum(len(p.text) for p in paragraphs),
            'model_used': 'docparse',
        }

    def _empty_tree(self) -> dict:
        name = os.path.splitext(os.path.basename(self.file_path))[0]
        return {
            'tree': {'doc_name': name, 'doc_description': '', 'structure': []},
            'total_pages': 0,
            'total_tokens': 0,
            'model_used': 'docparse',
        }


# ---------------------------------------------------------------------------
# PPTX Parser
# ---------------------------------------------------------------------------

class PptxParser:
    """Parse .pptx files using python-pptx."""

    def __init__(self, file_path: str):
        try:
            from pptx import Presentation
        except ImportError:
            print("python-pptx not found. Install with: pip install python-pptx", file=sys.stderr)
            sys.exit(1)
        self.prs = Presentation(file_path)
        self.file_path = file_path

    def _slide_title(self, slide, index: int) -> str:
        """Extract slide title or fallback to 'Slide N'."""
        if slide.shapes.title and slide.shapes.title.text.strip():
            return slide.shapes.title.text.strip()
        return f'Slide {index + 1}'

    def _slide_text(self, slide) -> str:
        """Extract all text from a slide."""
        texts = []
        for shape in slide.shapes:
            if shape.has_text_frame:
                for paragraph in shape.text_frame.paragraphs:
                    text = paragraph.text.strip()
                    if text:
                        texts.append(text)
        return '\n'.join(texts)

    def extract(self) -> str:
        """Extract plain text from all slides."""
        lines = []
        for i, slide in enumerate(self.prs.slides):
            title = self._slide_title(slide, i)
            lines.append(f'--- Slide {i + 1}: {title} ---')
            text = self._slide_text(slide)
            if text:
                lines.append(text)
            lines.append('')
        return '\n'.join(lines)

    def build_tree(self) -> dict:
        """Build tree: each slide is a leaf node."""
        doc_name = os.path.splitext(os.path.basename(self.file_path))[0]
        nodes = []
        total_tokens = 0

        for i, slide in enumerate(self.prs.slides):
            title = self._slide_title(slide, i)
            text = self._slide_text(slide)
            total_tokens += len(text)

            nodes.append({
                'node_id': f'{i + 1:04d}',
                'title': title,
                'start_index': i + 1,
                'end_index': i + 1,
                'summary': text[:200] if text else '',
                'nodes': [],
            })

        return {
            'tree': {
                'doc_name': doc_name,
                'doc_description': '',
                'structure': nodes,
            },
            'total_pages': len(list(self.prs.slides)),
            'total_tokens': total_tokens,
            'model_used': 'docparse',
        }


# ---------------------------------------------------------------------------
# XLSX Parser
# ---------------------------------------------------------------------------

class XlsxParser:
    """Parse .xlsx files using openpyxl."""

    def __init__(self, file_path: str):
        try:
            from openpyxl import load_workbook
        except ImportError:
            print("openpyxl not found. Install with: pip install openpyxl", file=sys.stderr)
            sys.exit(1)
        self.wb = load_workbook(file_path, read_only=True, data_only=True)
        self.file_path = file_path

    def extract(self) -> str:
        """Extract text: each sheet with tab-separated rows."""
        lines = []
        for sheet_name in self.wb.sheetnames:
            ws = self.wb[sheet_name]
            lines.append(f'=== Sheet: {sheet_name} ===')
            for row in ws.iter_rows(values_only=True):
                values = [str(cell) if cell is not None else '' for cell in row]
                lines.append('\t'.join(values))
            lines.append('')
        return '\n'.join(lines)

    def build_tree(self) -> dict:
        """Build tree: each sheet is a top-level node."""
        doc_name = os.path.splitext(os.path.basename(self.file_path))[0]
        nodes = []
        total_tokens = 0

        for i, sheet_name in enumerate(self.wb.sheetnames):
            ws = self.wb[sheet_name]
            row_count = 0
            sheet_text = []
            for row in ws.iter_rows(values_only=True):
                row_count += 1
                values = [str(cell) if cell is not None else '' for cell in row]
                sheet_text.append('\t'.join(values))

            text = '\n'.join(sheet_text)
            total_tokens += len(text)

            nodes.append({
                'node_id': f'{i + 1:04d}',
                'title': sheet_name,
                'start_index': 1,
                'end_index': row_count,
                'summary': text[:200] if text else '',
                'nodes': [],
            })

        return {
            'tree': {
                'doc_name': doc_name,
                'doc_description': '',
                'structure': nodes,
            },
            'total_pages': len(self.wb.sheetnames),
            'total_tokens': total_tokens,
            'model_used': 'docparse',
        }


# ---------------------------------------------------------------------------
# Dispatcher
# ---------------------------------------------------------------------------

PARSERS = {
    '.docx': DocxParser,
    '.pptx': PptxParser,
    '.xlsx': XlsxParser,
}


def get_parser(file_path: str):
    ext = os.path.splitext(file_path)[1].lower()
    parser_cls = PARSERS.get(ext)
    if parser_cls is None:
        print(f"Unsupported file type: {ext}. Supported: {', '.join(PARSERS.keys())}", file=sys.stderr)
        sys.exit(1)
    return parser_cls(file_path)


def main():
    parser = argparse.ArgumentParser(description='Parse documents (DOCX/PPTX/XLSX) for OpenScholar')
    parser.add_argument('--file_path', required=True, help='Path to the document file')

    mode = parser.add_mutually_exclusive_group(required=True)
    mode.add_argument('--extract', action='store_true', help='Extract plain text to stdout')
    mode.add_argument('--tree', action='store_true', help='Build tree structure (JSON to stdout)')

    args = parser.parse_args()

    if not os.path.isfile(args.file_path):
        print(f"File not found: {args.file_path}", file=sys.stderr)
        sys.exit(1)

    try:
        doc_parser = get_parser(args.file_path)

        if args.extract:
            text = doc_parser.extract()
            sys.stdout.write(text)
        else:
            result = doc_parser.build_tree()
            json.dump(result, sys.stdout, ensure_ascii=False)

    except Exception as e:
        print(str(e), file=sys.stderr)
        sys.exit(1)


if __name__ == '__main__':
    main()

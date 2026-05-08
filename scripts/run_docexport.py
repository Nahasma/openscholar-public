#!/usr/bin/env python3
"""
Document export wrapper for OpenScholar.
Converts LaTeX or Markdown to DOCX using pandoc.
Outputs JSON result to stdout.
"""

import argparse
import json
import os
import subprocess
import sys


def find_pandoc() -> str:
    """Find pandoc in PATH."""
    import shutil
    path = shutil.which('pandoc')
    if path is None:
        print("pandoc not found. Install with: brew install pandoc (macOS) or apt install pandoc (Linux)", file=sys.stderr)
        sys.exit(1)
    return path


def build_pandoc_args(args) -> list:
    """Build the pandoc command arguments."""
    pandoc = find_pandoc()
    cmd = [pandoc, args.input, '-o', args.output]

    # Auto-detect bibliography
    bib_path = args.bib
    if not bib_path:
        # Look for refs.bib in the same directory as input
        input_dir = os.path.dirname(os.path.abspath(args.input))
        candidate = os.path.join(input_dir, 'refs.bib')
        if os.path.isfile(candidate):
            bib_path = candidate

    if bib_path and os.path.isfile(bib_path):
        cmd.extend(['--citeproc', f'--bibliography={bib_path}'])

    if args.reference_doc and os.path.isfile(args.reference_doc):
        cmd.extend([f'--reference-doc={args.reference_doc}'])

    cmd.append('--number-sections')

    return cmd


def main():
    parser = argparse.ArgumentParser(description='Export document to DOCX via pandoc')
    parser.add_argument('--input', required=True, help='Input file path (LaTeX or Markdown)')
    parser.add_argument('--output', required=True, help='Output DOCX file path')
    parser.add_argument('--reference-doc', help='Optional Word reference template for styling')
    parser.add_argument('--bib', help='Bibliography file path (.bib)')
    args = parser.parse_args()

    if not os.path.isfile(args.input):
        print(f"Input file not found: {args.input}", file=sys.stderr)
        sys.exit(1)

    try:
        cmd = build_pandoc_args(args)

        # Run pandoc
        result = subprocess.run(
            cmd,
            capture_output=True,
            text=True,
            cwd=os.path.dirname(os.path.abspath(args.input)) or '.',
        )

        warnings = []
        if result.stderr:
            warnings = [line.strip() for line in result.stderr.splitlines() if line.strip()]

        if result.returncode != 0:
            output = {
                'success': False,
                'output_path': args.output,
                'error': result.stderr,
                'warnings': warnings,
            }
        else:
            output = {
                'success': True,
                'output_path': os.path.abspath(args.output),
                'warnings': warnings,
            }

        json.dump(output, sys.stdout, ensure_ascii=False)

    except Exception as e:
        print(str(e), file=sys.stderr)
        sys.exit(1)


if __name__ == '__main__':
    main()

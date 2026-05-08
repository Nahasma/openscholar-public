#!/usr/bin/env python3
"""
Thin wrapper around PageIndex for OpenScholar.
Reads a PDF, builds the tree structure, and outputs JSON to stdout.
"""

import argparse
import json
import sys
import os
from types import SimpleNamespace

# Prefer an installed pageindex package; local development checkouts can still
# provide ref/PageIndex without requiring that private path in public builds.
local_pageindex = os.path.join(os.path.dirname(__file__), '..', 'ref', 'PageIndex')
if os.path.isdir(local_pageindex):
    sys.path.insert(0, local_pageindex)


def main():
    parser = argparse.ArgumentParser(description='Build PageIndex tree from PDF')
    parser.add_argument('--pdf_path', required=False, help='Path to PDF file')
    parser.add_argument('--model', default='gpt-4o-2024-11-20', help='LLM model name')
    parser.add_argument('--generate_summary', action='store_true', help='Generate node summaries')
    parser.add_argument('--generate_description', action='store_true', help='Generate doc description')
    parser.add_argument('--max_pages_per_node', type=int, default=10, help='Max pages per node')
    parser.add_argument('--pdf_parser', default='PyMuPDF', choices=['PyPDF2', 'PyMuPDF'], help='PDF parser backend')
    parser.add_argument('--markdown_path', help='Pre-parsed Markdown path (from MinerU)')
    args = parser.parse_args()

    if not args.pdf_path and not args.markdown_path:
        parser.error('Either --pdf_path or --markdown_path is required')

    try:
        try:
            opt = SimpleNamespace(
                model=args.model,
                pdf_parser=args.pdf_parser,
                toc_check_page_num=20,
                max_page_num_each_node=args.max_pages_per_node,
                max_token_num_each_node=20000,
                if_add_node_id='yes',
                if_add_node_summary='yes' if args.generate_summary else 'no',
                if_add_doc_description='yes' if args.generate_description else 'no',
                if_add_node_text='no',
            )

            if args.markdown_path:
                import asyncio
                from pageindex.page_index_md import md_to_tree
                result = asyncio.run(md_to_tree(
                    md_path=args.markdown_path,
                    if_add_node_id=opt.if_add_node_id,
                    if_add_node_summary=opt.if_add_node_summary,
                    if_add_doc_description=opt.if_add_doc_description,
                    if_add_node_text=opt.if_add_node_text,
                    model=opt.model,
                ))
            else:
                from pageindex.page_index import page_index_main
                result = page_index_main(args.pdf_path, opt=opt)

            output = {
                'ok': True,
                'tree': result,
                'total_pages': result.get('total_pages', 0),
                'total_tokens': result.get('total_tokens', 0),
                'model_used': args.model,
            }

            json.dump(output, sys.stdout, ensure_ascii=False)

        except ImportError as e:
            print(str(e), file=sys.stderr)
            error_output = {
                'ok': False,
                'error_type': 'import_failed',
                'error_message': f"PageIndex not installed: {e}",
                'retryable': False,
            }
            json.dump(error_output, sys.stdout, ensure_ascii=False)
            sys.exit(1)

        except Exception as e:
            err_str = str(e).lower()
            print(str(e), file=sys.stderr)

            if '429' in err_str or 'rate' in err_str:
                error_type = 'provider_rate_limited'
                retryable = True
            elif '401' in err_str or 'auth' in err_str or 'invalid_api_key' in err_str or 'insufficient_quota' in err_str:
                error_type = 'provider_auth_or_quota'
                retryable = False
            elif 'ssl' in err_str or 'python' in err_str and 'version' in err_str:
                error_type = 'env_incompatible'
                retryable = False
            elif 'parse' in err_str or 'json' in err_str or 'decode' in err_str:
                error_type = 'llm_parse_failed'
                retryable = False
            elif 'pdf' in err_str or 'fitz' in err_str or 'pymupdf' in err_str:
                error_type = 'pdf_parse_failed'
                retryable = False
            else:
                error_type = 'unknown'
                retryable = False

            error_output = {
                'ok': False,
                'error_type': error_type,
                'error_message': str(e),
                'retryable': retryable,
            }
            json.dump(error_output, sys.stdout, ensure_ascii=False)
            sys.exit(1)

    except SystemExit:
        raise
    except Exception as e:
        # Last-resort handler: argument parsing errors etc. go to stderr only
        print(str(e), file=sys.stderr)
        sys.exit(1)


if __name__ == '__main__':
    main()

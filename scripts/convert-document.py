#!/usr/bin/env -S uv run --script
# /// script
# requires-python = ">=3.13"
# dependencies = ["markitdown[docx,pdf,pptx,xlsx]==0.1.7"]
# ///
"""Convert one validated local attachment; no URL, plugin, OCR or ZIP fallback."""

import importlib
import sys
from pathlib import Path

from markitdown import MarkItDown
from markitdown.converters import (
    CsvConverter,
    DocxConverter,
    PdfConverter,
    PlainTextConverter,
    PptxConverter,
    XlsxConverter,
)


def main() -> None:
    if sys.argv[1:] == ["--check"]:
        for module in ("mammoth", "lxml", "pptx", "pandas", "openpyxl", "pdfminer.high_level", "pdfplumber"):
            importlib.import_module(module)
        return
    if len(sys.argv) != 2:
        raise ValueError("expected one local attachment path")
    path = Path(sys.argv[1])
    converters = {
        ".pdf": PdfConverter,
        ".docx": DocxConverter,
        ".pptx": PptxConverter,
        ".xlsx": XlsxConverter,
        ".csv": CsvConverter,
        ".txt": PlainTextConverter,
    }
    if not path.is_absolute() or not path.is_file():
        raise ValueError("attachment must be an existing absolute local file")
    if path.suffix not in converters:
        raise ValueError("unsupported attachment format")
    md = MarkItDown(enable_builtins=False, enable_plugins=False)
    md.register_converter(converters[path.suffix]())
    text = md.convert_local(str(path)).markdown
    if not text.strip():
        raise ValueError("no extractable text; scanned/image-only files require OCR")
    data = text.encode("utf-8")
    if len(data) > 2 << 20:
        raise ValueError("extracted text exceeds 2 MiB")
    sys.stdout.buffer.write(data)


if __name__ == "__main__":
    main()

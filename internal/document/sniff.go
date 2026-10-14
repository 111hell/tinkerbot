package document

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Sniff validates bytes before choosing a converter. Filename only distinguishes
// CSV from plain text; binary formats must identify themselves.
func Sniff(data []byte, filename string) (string, error) {
	if len(data) == 0 {
		return "", fmt.Errorf("document: empty file")
	}
	if bytes.HasPrefix(data, []byte("%PDF-")) {
		return "pdf", nil
	}
	if bytes.HasPrefix(data, []byte("PK")) {
		z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return "", fmt.Errorf("document: invalid Office archive: %w", err)
		}
		var format string
		var total uint64
		contentTypes := false
		for _, f := range z.File {
			if f.UncompressedSize64 > 100<<20 || total > 100<<20-f.UncompressedSize64 {
				return "", fmt.Errorf("document: Office archive expands beyond 100 MiB")
			}
			total += f.UncompressedSize64
			if f.Name == "[Content_Types].xml" {
				contentTypes = true
			}
			candidate := ""
			switch f.Name {
			case "word/document.xml":
				candidate = "docx"
			case "ppt/presentation.xml":
				candidate = "pptx"
			case "xl/workbook.xml":
				candidate = "xlsx"
			}
			if candidate != "" {
				if format != "" && format != candidate {
					return "", fmt.Errorf("document: ambiguous Office archive")
				}
				format = candidate
			}
		}
		if format != "" && contentTypes {
			return format, nil
		}
		return "", fmt.Errorf("document: unsupported archive; expected DOCX, PPTX or XLSX")
	}
	ext := strings.ToLower(filepath.Ext(filename))
	if ext != ".txt" && ext != ".csv" {
		return "", fmt.Errorf("document: unsupported or damaged file; expected PDF, DOCX, PPTX, XLSX, CSV or TXT")
	}
	if !utf8.Valid(data) {
		return "", fmt.Errorf("document: TXT and CSV require UTF-8 text")
	}
	for _, r := range string(data) {
		if unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' {
			return "", fmt.Errorf("document: binary content in text file")
		}
	}
	// MarkItDown pads or truncates mismatched CSV rows. Reject them explicitly
	// so a successful conversion never silently loses extra cells.
	if ext == ".csv" {
		reader := csv.NewReader(bytes.NewReader(data))
		for {
			_, err := reader.Read()
			if err == io.EOF {
				break
			}
			if err != nil {
				return "", fmt.Errorf("document: invalid CSV: %w", err)
			}
		}
	}
	return strings.TrimPrefix(ext, "."), nil
}

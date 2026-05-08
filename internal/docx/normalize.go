package docx

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// IsNormalizationAvailable checks whether unoconvert or soffice is available.
func IsNormalizationAvailable() bool {
	if _, err := exec.LookPath("unoconvert"); err == nil {
		return true
	}
	if _, err := exec.LookPath("soffice"); err == nil {
		return true
	}
	return false
}

// NormalizeOfficeInput converts .doc/.dotx to .docx using unoserver/soffice.
// Returns the path to the converted .docx file.
func NormalizeOfficeInput(ctx context.Context, inputPath string) (docxPath string, err error) {
	ext := strings.ToLower(filepath.Ext(inputPath))
	if ext == ".docx" {
		// Already a docx, nothing to do.
		return inputPath, nil
	}

	dir := filepath.Dir(inputPath)
	base := strings.TrimSuffix(filepath.Base(inputPath), filepath.Ext(inputPath))
	outPath := filepath.Join(dir, base+".docx")

	// Prefer unoconvert (unoserver) over soffice
	if unoPath, lookErr := exec.LookPath("unoconvert"); lookErr == nil {
		cmd := exec.CommandContext(ctx, unoPath, "--convert-to", "docx", inputPath, outPath)
		if out, runErr := cmd.CombinedOutput(); runErr != nil {
			return "", fmt.Errorf("docx: unoconvert failed: %w\n%s", runErr, out)
		}
		return outPath, nil
	}

	// Fallback: soffice --headless --convert-to docx
	if soPath, lookErr := exec.LookPath("soffice"); lookErr == nil {
		cmd := exec.CommandContext(ctx, soPath,
			"--headless",
			"--convert-to", "docx",
			"--outdir", dir,
			inputPath,
		)
		if out, runErr := cmd.CombinedOutput(); runErr != nil {
			return "", fmt.Errorf("docx: soffice failed: %w\n%s", runErr, out)
		}
		return outPath, nil
	}

	return "", fmt.Errorf("docx: neither unoconvert nor soffice found in PATH")
}

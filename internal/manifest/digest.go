package manifest

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
)

// Digest returns the canonical SHA-256 digest of content, formatted as
// "sha256:<hex>". The bytes are LF-normalised before hashing so digests
// remain stable across Windows (CRLF) / classic-Mac (CR) / Unix (LF) line
// endings. v0 documents LF-only output, but inputs read off disk on cross-
// platform CI may have CRLFs and we want round-trip stability.
func Digest(content []byte) string {
	sum := sha256.Sum256(NormalizeLineEndings(content))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// NormalizeLineEndings collapses CRLF and bare CR sequences to LF. Exposed
// so the writer can apply the same normalisation when persisting files
// (keeping on-disk content byte-aligned with what Digest hashed).
func NormalizeLineEndings(b []byte) []byte {
	if !bytes.ContainsAny(b, "\r") {
		return b
	}
	out := bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))
	out = bytes.ReplaceAll(out, []byte("\r"), []byte("\n"))
	return out
}

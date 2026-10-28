package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/sanhaji182/lintasan-go/internal/conversation"
)

// Attachment handling for chat.
//
// Bytes go to disk under <data dir>/attachments/<conversation id>/ and only
// metadata plus the extracted text live in SQLite. That keeps the database
// small and backup-able while still letting the model read the file contents
// on every turn without re-parsing a PDF.

// maxAttachmentBytes is the per-file upload ceiling. 25MB covers a long PDF or
// a screenshot set; anything larger should be linked rather than uploaded.
const maxAttachmentBytes = 25 * 1024 * 1024

// maxExtractedChars caps how much text we keep per file. A 300-page PDF would
// otherwise dominate the context window on every subsequent turn.
const maxExtractedChars = 60_000

// attachmentsRoot returns the on-disk directory for chat attachments, creating
// it if needed.
func (s *Server) attachmentsRoot() (string, error) {
	root := filepath.Join(s.cfg.DataDir, "attachments")
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", fmt.Errorf("create attachments dir: %w", err)
	}
	return root, nil
}

// handleUploadAttachment accepts a multipart upload and records its metadata.
//
// The file is written to disk first, then the extracted text is computed and
// stored. Extraction failures are not fatal: an unsupported binary still
// uploads, it simply contributes no text to the prompt (and the model is told
// it exists).
func (s *Server) handleUploadAttachment(w http.ResponseWriter, r *http.Request) {
	st, ok := s.convStoreReady(w)
	if !ok {
		return
	}
	convID := r.PathValue("id")
	if _, err := st.Get(convID); err != nil {
		writeErr(w, http.StatusNotFound, "conversation not found")
		return
	}

	// Cap the whole request, not just the file part, so a malicious multipart
	// body cannot stream unbounded data at us.
	r.Body = http.MaxBytesReader(w, r.Body, maxAttachmentBytes+1024*1024)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid multipart upload: "+err.Error())
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "form field 'file' is required")
		return
	}
	defer file.Close()

	if header.Size > maxAttachmentBytes {
		writeErr(w, http.StatusRequestEntityTooLarge,
			fmt.Sprintf("file is %d bytes, limit is %d", header.Size, maxAttachmentBytes))
		return
	}

	root, err := s.attachmentsRoot()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	dir := filepath.Join(root, convID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		writeErr(w, http.StatusInternalServerError, "create upload dir: "+err.Error())
		return
	}

	// Never trust the client's filename for the path: strip to a base name and
	// prefix with a random id so two uploads of "report.pdf" cannot collide or
	// escape the directory via "../".
	safeName := sanitiseFilename(header.Filename)
	a := &conversation.Attachment{
		ConversationID: convID,
		Filename:       safeName,
		Mime:           detectMime(safeName, header.Header.Get("Content-Type")),
		Size:           header.Size,
	}
	tmpPath := filepath.Join(dir, ".uploading-"+filepath.Base(safeName))
	dst, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "create file: "+err.Error())
		return
	}

	written, copyErr := io.Copy(dst, io.LimitReader(file, maxAttachmentBytes+1))
	closeErr := dst.Close()
	if copyErr != nil || written > maxAttachmentBytes {
		os.Remove(tmpPath)
		if written > maxAttachmentBytes {
			writeErr(w, http.StatusRequestEntityTooLarge, "file exceeds the size limit")
			return
		}
		writeErr(w, http.StatusInternalServerError, "write file: "+copyErr.Error())
		return
	}
	if closeErr != nil {
		os.Remove(tmpPath)
		writeErr(w, http.StatusInternalServerError, "close file: "+closeErr.Error())
		return
	}

	// Extract text before the rename so a failure leaves no half-registered
	// attachment behind.
	text, extractErr := extractText(tmpPath, a.Mime, safeName)
	text = truncate(text, maxExtractedChars)
	a.Extracted = text
	a.Size = written

	// Move into place BEFORE the insert: the row must never reference a path
	// that does not exist yet, because download and prompt assembly both trust
	// the stored path. AddAttachment assigns an id when one is absent, so
	// reserve it here to build the filename.
	if a.ID == "" {
		a.ID = conversation.NewID("a_")
	}
	finalPath := filepath.Join(dir, a.ID+"-"+safeName)
	if err := os.Rename(tmpPath, finalPath); err != nil {
		os.Remove(tmpPath)
		writeErr(w, http.StatusInternalServerError, "finalize file: "+err.Error())
		return
	}
	a.Path = finalPath

	if err := st.AddAttachment(a); err != nil {
		os.Remove(finalPath)
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	resp := map[string]any{
		"id":       a.ID,
		"filename": a.Filename,
		"mime":     a.Mime,
		"size":     a.Size,
		"has_text": strings.TrimSpace(a.Extracted) != "",
	}
	if extractErr != nil {
		// Upload succeeded but the text could not be read. Say so instead of
		// pretending the model can see it.
		resp["warning"] = "text could not be extracted: " + extractErr.Error()
	}
	writeJSONStatus(w, http.StatusCreated, resp)
}

// handleDownloadAttachment streams a stored file back. It is behind auth like
// every other /api/* path, so only a logged-in dashboard user can read it.
func (s *Server) handleDownloadAttachment(w http.ResponseWriter, r *http.Request) {
	st, ok := s.convStoreReady(w)
	if !ok {
		return
	}
	a, err := st.GetAttachment(r.PathValue("aid"))
	if err != nil {
		if errors.Is(err, conversation.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "attachment not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if a.ConversationID != r.PathValue("id") {
		writeErr(w, http.StatusNotFound, "attachment not found in this conversation")
		return
	}
	// Refuse to serve anything outside the attachments root, even if the stored
	// path was tampered with in the database.
	root, err := s.attachmentsRoot()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	clean := filepath.Clean(a.Path)
	if !strings.HasPrefix(clean, filepath.Clean(root)+string(os.PathSeparator)) {
		writeErr(w, http.StatusForbidden, "attachment path is outside the storage root")
		return
	}

	f, err := os.Open(clean)
	if err != nil {
		writeErr(w, http.StatusNotFound, "attachment file is missing from disk")
		return
	}
	defer f.Close()

	w.Header().Set("Content-Type", a.Mime)
	w.Header().Set("Content-Disposition", "attachment; filename=\""+sanitiseFilename(a.Filename)+"\"")
	if a.Size > 0 {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", a.Size))
	}
	_, _ = io.Copy(w, f)
}

// deleteAttachmentFile removes a file from disk, ignoring errors: a missing
// file means the delete already happened.
func (s *Server) deleteAttachmentFile(path string) {
	if strings.TrimSpace(path) == "" {
		return
	}
	root, err := s.attachmentsRoot()
	if err != nil {
		return
	}
	clean := filepath.Clean(path)
	if !strings.HasPrefix(clean, filepath.Clean(root)+string(os.PathSeparator)) {
		return
	}
	_ = os.Remove(clean)
}

// sanitiseFilename reduces an uploaded filename to something safe to join onto
// a directory: no path separators, no traversal, no control characters.
func sanitiseFilename(name string) string {
	// Take the base component on both separator conventions.
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." {
		return "file"
	}
	// Strip control characters and anything that would break the header value.
	var b strings.Builder
	for _, r := range name {
		if r < 0x20 || r == 0x7f || r == '"' {
			continue
		}
		b.WriteRune(r)
	}
	out := b.String()
	if out == "" {
		return "file"
	}
	// Filesystems cap names around 255 bytes.
	if len(out) > 120 {
		ext := filepath.Ext(out)
		if len(ext) > 20 {
			ext = ""
		}
		out = out[:120-len(ext)] + ext
	}
	return out
}

// detectMime prefers the extension's known type over the client's claim, which
// is frequently wrong for code and plain text files.
func detectMime(filename, claimed string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".txt", ".md", ".markdown":
		return "text/plain"
	case ".json":
		return "application/json"
	case ".csv":
		return "text/csv"
	case ".html", ".htm":
		return "text/html"
	case ".xml":
		return "text/xml"
	case ".yaml", ".yml":
		return "text/yaml"
	case ".pdf":
		return "application/pdf"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".svg":
		return "image/svg+xml"
	case ".docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case ".xlsx":
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case ".go", ".py", ".js", ".ts", ".rs", ".java", ".c", ".cpp", ".h", ".rb", ".sh",
		".sql", ".css", ".svelte", ".toml", ".ini", ".conf", ".log":
		return "text/plain"
	}
	if strings.TrimSpace(claimed) != "" && claimed != "application/octet-stream" {
		return claimed
	}
	return "application/octet-stream"
}

// extractText pulls plain text out of a file for use in the prompt.
//
// Only formats we can parse without a heavyweight dependency are supported.
// PDF and Office formats are detected but not parsed: adding a PDF library is a
// separate decision with its own dependency and CVE surface, so this returns an
// explicit error rather than silently returning nothing.
func extractText(path, mime, filename string) (string, error) {
	// Read at most a little over the extraction cap: there is no point loading
	// 25MB of text only to throw 99% of it away.
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open: %w", err)
	}
	defer f.Close()

	if isTextish(mime, filename) {
		buf := make([]byte, maxExtractedChars*4+1)
		n, err := io.ReadFull(f, buf)
		if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
			return "", fmt.Errorf("read: %w", err)
		}
		buf = buf[:n]
		// Reject binaries masquerading as text: a NUL byte in the first KB is
		// the standard giveaway.
		probe := buf
		if len(probe) > 1024 {
			probe = probe[:1024]
		}
		for _, b := range probe {
			if b == 0 {
				return "", fmt.Errorf("file looks binary despite its type")
			}
		}
		if !utf8.Valid(buf) {
			// Keep what is decodable rather than failing the upload outright.
			buf = []byte(strings.ToValidUTF8(string(buf), ""))
		}
		return string(buf), nil
	}

	switch {
	case mime == "application/pdf":
		return "", fmt.Errorf("PDF text extraction is not enabled on this gateway; the file was stored but the model cannot read its contents")
	case strings.Contains(mime, "wordprocessingml"), strings.Contains(mime, "spreadsheetml"):
		return "", fmt.Errorf("Office document extraction is not enabled on this gateway; the file was stored but the model cannot read its contents")
	case strings.HasPrefix(mime, "image/"):
		return "", fmt.Errorf("image files are stored but this gateway does not forward them to vision models yet")
	}
	return "", fmt.Errorf("no text extractor for %s", mime)
}

// isTextish reports whether the file should be read as plain text.
func isTextish(mime, filename string) bool {
	if strings.HasPrefix(mime, "text/") {
		return true
	}
	switch mime {
	case "application/json", "text/xml", "text/yaml", "image/svg+xml":
		return true
	}
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".txt", ".md", ".markdown", ".json", ".csv", ".yaml", ".yml", ".toml",
		".ini", ".conf", ".log", ".go", ".py", ".js", ".ts", ".rs", ".java",
		".c", ".cpp", ".h", ".rb", ".sh", ".sql", ".css", ".svelte", ".html", ".htm":
		return true
	}
	return false
}

// ensure json import is used even if a future edit drops the only other use.
var _ = json.Marshal

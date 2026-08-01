package handler

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

const maxSupportUploadSize = 10 << 20 // 10 MB

func supportAttachmentContentType(filename string, sample []byte) (string, bool) {
	contentType := strings.ToLower(strings.TrimSpace(http.DetectContentType(sample)))
	switch strings.ToLower(path.Ext(filename)) {
	case ".png":
		if contentType == "image/png" {
			return contentType, true
		}
	case ".jpg", ".jpeg":
		if contentType == "image/jpeg" {
			return contentType, true
		}
	case ".webp":
		if contentType == "image/webp" {
			return contentType, true
		}
	case ".pdf":
		if contentType == "application/pdf" {
			return contentType, true
		}
	}
	return "", false
}

// UploadSupportAttachment is the only upload surface exposed to a reporter.
// It binds the object to the reporter-owned support session before returning,
// never to an issue, comment, task, or another reporter's chat.
func (h *Handler) UploadSupportAttachment(w http.ResponseWriter, r *http.Request) {
	if h.Storage == nil {
		writeError(w, http.StatusServiceUnavailable, "file upload not configured")
		return
	}
	caseRow, ok := h.supportSessionForReporter(w, r)
	if !ok {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxSupportUploadSize+(1<<20))
	if err := r.ParseMultipartForm(maxSupportUploadSize); err != nil {
		writeError(w, http.StatusBadRequest, "file too large or invalid multipart form")
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("missing file field: %v", err))
		return
	}
	defer file.Close()

	filename := path.Base(strings.ReplaceAll(strings.TrimSpace(header.Filename), "\\", "/"))
	if filename == "" || filename == "." || filename == "/" {
		filename = "anexo"
	}
	data, err := io.ReadAll(io.LimitReader(file, maxSupportUploadSize+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read file")
		return
	}
	if len(data) == 0 {
		writeError(w, http.StatusBadRequest, "empty file")
		return
	}
	if len(data) > maxSupportUploadSize {
		writeError(w, http.StatusRequestEntityTooLarge, "support attachment exceeds 10 MB")
		return
	}
	sample := data
	if len(sample) > 512 {
		sample = sample[:512]
	}
	contentType, allowed := supportAttachmentContentType(filename, sample)
	if !allowed {
		writeError(w, http.StatusUnsupportedMediaType, "support attachments must be PNG, JPEG, WEBP, or PDF")
		return
	}

	id, err := uuid.NewV7()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to allocate attachment")
		return
	}
	key := "workspaces/" + uuidToString(caseRow.WorkspaceID) + "/support/" + id.String() + strings.ToLower(path.Ext(filename))
	link, err := h.Storage.Upload(r.Context(), key, data, contentType, filename)
	if err != nil {
		slog.Error("support attachment upload failed", "case_id", uuidToString(caseRow.ID), "error", err)
		writeError(w, http.StatusInternalServerError, "upload failed")
		return
	}

	attachment, err := h.Queries.CreateAttachment(r.Context(), db.CreateAttachmentParams{
		ID:            pgtype.UUID{Bytes: id, Valid: true},
		WorkspaceID:   caseRow.WorkspaceID,
		ChatSessionID: caseRow.ChatSessionID,
		UploaderType:  "member",
		UploaderID:    caseRow.ReporterUserID,
		Filename:      filename,
		Url:           link,
		ContentType:   contentType,
		SizeBytes:     int64(len(data)),
	})
	if err != nil {
		h.deleteS3Object(r.Context(), link)
		slog.Error("support attachment record failed", "case_id", uuidToString(caseRow.ID), "error", err)
		writeError(w, http.StatusInternalServerError, "failed to save attachment")
		return
	}
	writeJSON(w, http.StatusCreated, h.attachmentToResponse(attachment, attachmentURLModeFromRequest(r)))
}

package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"

	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadThumbnail(w http.ResponseWriter, r *http.Request) {
	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}

	videoMetadata, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't retrieve video data", err)
		return
	}
	if videoMetadata.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "401 Unauthorized", nil)
		return
	}

	fmt.Println("uploading thumbnail for video", videoID, "by user", userID)

	const maxMemory = 10 << 20
	err = r.ParseMultipartForm(maxMemory)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't parse thumbnail", err)
		return
	}
	key := "thumbnail"
	fileData, fileHeaders, err := r.FormFile(key)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't retrieve thumbnail data", err)
		return
	}

	mimeType := fileHeaders.Header.Get("Content-Type")
	mediaType, _, err := mime.ParseMediaType(mimeType)
	if mediaType != "image/jpeg" && mediaType != "image/png" {
		msg := fmt.Sprintf("Invalid media type '%s' provided. Media type must be 'image/jpeg' or 'image/png'", mediaType)
		respondWithError(w, http.StatusBadRequest, msg, nil)
		return
	}

	extensions, err := mime.ExtensionsByType(mimeType)
	if err != nil {
		msg := "Unable to parse file type"
		respondWithError(w, http.StatusBadRequest, msg, err)
		return
	}
	if len(extensions) == 0 {
		msg := fmt.Sprintf("No extensions found for %s", mimeType)
		respondWithError(w, http.StatusBadRequest, msg, nil)
		return
	}
	randomFilename, err := generateRandomFilename()
	if err != nil {
		msg := "Unable to generate filename"
		respondWithError(w, http.StatusBadRequest, msg, err)
		return
	}
	fileName := fmt.Sprintf("%s%s", randomFilename, extensions[0])
	filePath := filepath.Join(cfg.assetsRoot, fileName)

	newFile, err := os.Create(filePath)
	_, err = io.Copy(newFile, fileData)
	if err != nil {
		msg := "Couldn't write thumbnail to file"
		respondWithError(w, http.StatusInternalServerError, msg, err)
		return
	}

	thumbnailURL := fmt.Sprintf("http://%s:%s/%s", baseWebsiteURL, cfg.port, filePath)
	videoMetadata.ThumbnailURL = &thumbnailURL

	err = cfg.db.UpdateVideo(videoMetadata)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't update thumbnail URL", err)
		return
	}
	respondWithJSON(w, http.StatusOK, videoMetadata)
}

func generateRandomFilename() (string, error) {
	filename := make([]byte, 32)
	_, err := rand.Read(filename)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(filename), nil
}

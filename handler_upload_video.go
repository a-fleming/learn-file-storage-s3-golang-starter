package main

import (
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {
	const maxUploadSize = 1 << 30 // 1 GB
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)

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

	key := "video"
	fileData, _, err := r.FormFile(key)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't retrieve video data", err)
		return
	}
	defer fileData.Close()

	mimeType := "video/mp4"
	mediaType, _, err := mime.ParseMediaType(mimeType)
	if mediaType != mimeType {
		msg := fmt.Sprintf("Invalid media type '%s' provided. Media type must be 'video/mp4'", mediaType)
		respondWithError(w, http.StatusBadRequest, msg, nil)
		return
	}
	tmpFileName := "tubely-upload.mp4"
	tmpFile, err := os.CreateTemp("", tmpFileName)
	if err != nil {
		msg := "Couldn't create temporary file"
		respondWithError(w, http.StatusInternalServerError, msg, err)
	}
	defer os.Remove("tubely-upload.mp4")
	defer tmpFile.Close()

	_, err = io.Copy(tmpFile, fileData)
	if err != nil {
		msg := "Couldn't copy video contents"
		respondWithError(w, http.StatusInternalServerError, msg, err)
	}
	tmpFile.Seek(0, io.SeekStart)
	randomFilename, err := generateRandomFilename()
	if err != nil {
		msg := "Unable to generate filename"
		respondWithError(w, http.StatusBadRequest, msg, err)
		return
	}
	s3Key := randomFilename + ".mp4"

	putObjectInput := s3.PutObjectInput{
		Bucket:      &cfg.s3Bucket,
		Key:         &s3Key,
		Body:        tmpFile,
		ContentType: &mimeType,
	}
	_, err = cfg.s3Client.PutObject(r.Context(), &putObjectInput)
	if err != nil {
		msg := "Unable to store video in S3"
		respondWithError(w, http.StatusBadRequest, msg, err)
		return
	}

	videoURL := fmt.Sprintf(
		"https://%s.s3.%s.amazonaws.com/%s",
		cfg.s3Bucket,
		cfg.s3Region,
		s3Key,
	)
	videoMetadata.VideoURL = &videoURL

	err = cfg.db.UpdateVideo(videoMetadata)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't update video URL", err)
		return
	}
	respondWithJSON(w, http.StatusOK, videoMetadata)
}

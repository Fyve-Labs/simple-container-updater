package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	log "github.com/sirupsen/logrus"
	"net/http"
	"strings"
	"time"
)

type updateRequest struct {
	Name  string `json:"name"`
	Image string `json:"image"`
}

type updateResponse struct {
	Ok bool `json:"OK"`
}

type updateConfig struct {
	secretKey        string
	timeoutInSeconds int
	containerNames   []string
	updater          UpdaterInterface
}

func ConstructJsonRequest[T interface{}](r *http.Request, secretKey string) (*T, error) {
	if r.Method != "POST" {
		return nil, errors.New("only POST requests allowed")
	}

	if c := r.Header.Get("Content-Type"); c != "application/json" {
		return nil, errors.New("request body must be json")
	}

	signature := r.Header.Get("X-Signature")
	if signature == "" {
		return nil, errors.New("missing signature")
	}

	var req T
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, errors.New("decode error")
	}

	payload, _ := json.Marshal(req)
	h := hmac.New(sha256.New, []byte(secretKey))
	h.Write(payload)
	calculatedHMACBytes := h.Sum(nil)
	calculatedHMAC := hex.EncodeToString(calculatedHMACBytes)
	if calculatedHMAC != signature {
		return nil, errors.New("signature mismatched")
	}

	return &req, nil
}

func UpdateHandler(config *updateConfig) http.Handler {
	timeout := time.Second * time.Duration(config.timeoutInSeconds)
	updater := newUpdateHandler()

	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req, err := ConstructJsonRequest[updateRequest](r, config.secretKey)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if err := updater.Update(context.Background(), req.Name, req.Image); err != nil {
			log.Errorf("update image: %v", err)
			statusCode := http.StatusInternalServerError
			if strings.Contains(err.Error(), "not in allow list") {
				statusCode = http.StatusBadRequest
			}

			http.Error(w, "Internal server error", statusCode)
			return
		}

		log.Infof("Updated %s to image %s", req.Name, req.Image)

		bs, _ := json.Marshal(&updateResponse{true})
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(bs)
	})

	return http.TimeoutHandler(h, timeout, fmt.Sprintf(
		"Exceeded configured timeout of %v.\n",
		timeout,
	))
}

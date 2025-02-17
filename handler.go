package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/docker/docker/client"
	log "github.com/sirupsen/logrus"
	"net/http"
	"os"
	"slices"
	"time"
)

type updateRequest struct {
	Name  string `json:"name"`
	Image string `json:"image"`
}

type updateResponse struct {
	Ok bool `json:"OK"`
}

type updateHandler struct {
	cli                *client.Client
	containerAllowList []string
}

func newUpdateHandler() *updateHandler {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		log.Fatalf("can't create docker client: %v", err)
	}

	var containerAllowList []string
	if listStr, isSet := os.LookupEnv("CONTAINER_ALLOW_LIST"); isSet {
		err := json.Unmarshal([]byte(listStr), &containerAllowList)
		if err != nil {
			log.Error("Malformed CONTAINER_ALLOW_LIST")
		}
	}

	log.Debugf("CONTAINER_ALLOW_LIST: %v", containerAllowList)

	return &updateHandler{
		cli:                cli,
		containerAllowList: containerAllowList,
	}
}

func (u *updateHandler) Handler(secretKey string, timeoutInSeconds int) http.Handler {
	timeout := time.Second * time.Duration(timeoutInSeconds)

	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		signature := r.Header.Get("X-Signature")
		if signature == "" || signature != secretKey {
			http.Error(w, "", http.StatusUnauthorized)
			return
		}

		if r.Method != "POST" {
			http.Error(w, "Only POST requests allowed", http.StatusBadRequest)
			return
		}

		if c := r.Header.Get("Content-Type"); c != "application/json" {
			http.Error(w, "Request body must be json", http.StatusBadRequest)
			return
		}

		var req updateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}

		if len(u.containerAllowList) > 0 && !slices.Contains(u.containerAllowList, req.Name) {
			log.Warnf("Container is not in allow list: %s", req.Name)
			http.Error(w, "Container is not in allow list", http.StatusBadRequest)
			return
		}

		if err := u.Update(context.Background(), &req); err != nil {
			log.Errorf("update image: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

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

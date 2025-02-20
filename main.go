package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	log "github.com/sirupsen/logrus"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"time"
)

func main() {
	logLevel := os.Getenv("LOG_LEVEL")
	secretKey := os.Getenv("SECRET_KEY")
	timeoutInSeconds := 300

	if logLevel == "" {
		logLevel = "DEBUG"
	}

	ll, err := log.ParseLevel(logLevel)
	if err != nil {
		log.Fatal("invalid log level")
	}

	log.SetLevel(ll)

	if secretKey == "" {
		bytes := make([]byte, 16)
		_, _ = io.ReadFull(rand.Reader, bytes)
		secretKey = hex.EncodeToString(bytes[:])
		log.Infof("Temporarily generated sk: %s", secretKey)
	}

	if timeoutStr, isSet := os.LookupEnv("REQUEST_TIMEOUT_SECONDS"); isSet {
		if timeoutInt, err := strconv.Atoi(timeoutStr); err == nil {
			timeoutInSeconds = timeoutInt
		}
	}

	router := http.NewServeMux()
	router.Handle("/update", UpdateHandler(&updateConfig{
		secretKey:        secretKey,
		timeoutInSeconds: timeoutInSeconds,
	}))

	serverPort := 8080
	if strPort, isSet := os.LookupEnv("PORT"); isSet {
		if intPort, err := strconv.Atoi(strPort); err == nil {
			serverPort = intPort
		}
	}

	server := &http.Server{
		Addr:         fmt.Sprintf(":%v", serverPort),
		Handler:      router,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  15 * time.Second,
	}
	done := make(chan bool)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)

	go func() {
		<-quit
		log.Info("Server is shutting down...")

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := server.Shutdown(ctx); err != nil {
			log.Fatalf("Could not gracefully shutdown the server: %v\n", err)
		}
		close(done)
	}()

	log.Info("Server is ready to handle requests at :", serverPort)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("Could not listen on %d: %v\n", serverPort, err)
	}

	<-done
	log.Info("Server stopped")
}

func randString(size int) (string, error) {
	b := make([]byte, size)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

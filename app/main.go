package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
)

// Version est injectée au build via -ldflags, pour visualiser
// concrètement chaque nouveau déploiement en démo.
var Version = "dev"

type statusResponse struct {
	Message string `json:"message"`
	Version string `json:"version"`
	Node    string `json:"node,omitempty"`
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
	resp := statusResponse{
		Message: "Hello depuis le cluster K8s !",
		Version: Version,
		Node:    os.Getenv("NODE_NAME"), // rempli via downward API dans le manifest k8s
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func healthzHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "ok")
}

func main() {
	http.HandleFunc("/", rootHandler)
	http.HandleFunc("/healthz", healthzHandler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("Démarrage du serveur sur le port %s (version %s)", port, Version)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
)

const (
	webhookPort      = "8081"
	targetDeployment = "hello-app"
	targetNamespace  = "default"
	targetContainer  = "hello-app"
	imageRepo        = "ghcr.io/empty-bot/k8s-cicd-leader-election/app"
)

type githubPushPayload struct {
	After string `json:"after"`
	Ref   string `json:"ref"`
}

func setLeaderLabel(clientset *kubernetes.Clientset, podName, namespace string, isLeader bool) error {
	value := "false"
	if isLeader {
		value = "true"
	}
	patch := []byte(`{"metadata":{"labels":{"role-leader":"` + value + `"}}}`)
	_, err := clientset.CoreV1().Pods(namespace).Patch(
		context.Background(),
		podName,
		types.StrategicMergePatchType,
		patch,
		metav1.PatchOptions{},
	)
	return err
}

func startWebhookServer(ctx context.Context, clientset *kubernetes.Clientset, identity string) {
	podName := os.Getenv("POD_NAME")
	namespace := "default"

	if err := setLeaderLabel(clientset, podName, namespace, true); err != nil {
		log.Printf("[%s] Impossible de poser le label leader : %v", identity, err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/webhook", func(w http.ResponseWriter, r *http.Request) {
		handleWebhook(w, r, clientset, identity)
	})

	srv := &http.Server{
		Addr:    ":" + webhookPort,
		Handler: mux,
	}

	go func() {
		<-ctx.Done()
		log.Printf("[%s] Arrêt du serveur webhook", identity)
		if err := setLeaderLabel(clientset, podName, namespace, false); err != nil {
			log.Printf("[%s] Impossible de retirer le label leader : %v", identity, err)
		}
		srv.Close()
	}()

	log.Printf("[%s] Serveur webhook démarré sur :%s", identity, webhookPort)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Printf("[%s] Erreur serveur webhook : %v", identity, err)
	}
}

func handleWebhook(w http.ResponseWriter, r *http.Request, clientset *kubernetes.Clientset, identity string) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "lecture du corps impossible", http.StatusBadRequest)
		return
	}

	if !verifySignature(r.Header.Get("X-Hub-Signature-256"), body) {
		log.Printf("[%s] Signature webhook invalide, requête rejetée", identity)
		http.Error(w, "signature invalide", http.StatusUnauthorized)
		return
	}

	var payload githubPushPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, "payload JSON invalide", http.StatusBadRequest)
		return
	}

	if payload.After == "" {
		http.Error(w, "champ 'after' manquant", http.StatusBadRequest)
		return
	}

	log.Printf("[%s] Push reçu, commit %s — déclenchement du déploiement", identity, payload.After)

	if err := triggerDeployment(clientset, payload.After); err != nil {
		log.Printf("[%s] Échec du déploiement : %v", identity, err)
		http.Error(w, "échec du déploiement", http.StatusInternalServerError)
		return
	}

	log.Printf("[%s] Déploiement déclenché avec succès pour le commit %s", identity, payload.After)
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("déploiement déclenché"))
}

func verifySignature(signatureHeader string, body []byte) bool {
	secret := os.Getenv("GITHUB_WEBHOOK_SECRET")
	if secret == "" || signatureHeader == "" {
		return false
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(expected), []byte(signatureHeader))
}

func triggerDeployment(clientset *kubernetes.Clientset, commitSHA string) error {
	shortSHA := commitSHA
	if len(commitSHA) > 7 {
		shortSHA = commitSHA[:7]
	}
	newImage := imageRepo + ":" + shortSHA

	patch := []byte(`{"spec":{"template":{"spec":{"containers":[{"name":"` + targetContainer + `","image":"` + newImage + `"}]}}}}`)

	_, err := clientset.AppsV1().Deployments(targetNamespace).Patch(
		context.Background(),
		targetDeployment,
		types.StrategicMergePatchType,
		patch,
		metav1.PatchOptions{},
	)
	return err
}
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
)

const (
	leaseName      = "orchestrator-leader"
	leaseNamespace = "default"
	healthzPort    = "8080"
)

var Version = "dev"

func main() {
	nodeName := os.Getenv("NODE_NAME")
	if nodeName == "" {
		nodeName = "unknown-node"
	}
	identity := nodeName + "_" + string(uuid.NewUUID())[:8]

	log.Printf("[%s] Démarrage de l'orchestrateur (version %s)", identity, Version)

	// Serveur healthz permanent : tourne sur TOUS les pods, en continu,
	// dès le démarrage — indépendant de l'élection. C'est ce qui permet
	// au readinessProbe Kubernetes de savoir que le pod est vivant,
	// et donc de le garder comme endpoint valide du Service.
	go startHealthzServer(identity)

	config, err := rest.InClusterConfig()
	if err != nil {
		log.Fatalf("Impossible de charger la config in-cluster : %v", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalf("Impossible de créer le client Kubernetes : %v", err)
	}

	lock := &resourcelock.LeaseLock{
		LeaseMeta: metaObject(leaseName, leaseNamespace),
		Client:    clientset.CoordinationV1(),
		LockConfig: resourcelock.ResourceLockConfig{
			Identity: identity,
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sigCh
		log.Printf("[%s] Signal d'arrêt reçu, libération du leadership...", identity)
		cancel()
	}()

	leaderelection.RunOrDie(ctx, leaderelection.LeaderElectionConfig{
		Lock:            lock,
		ReleaseOnCancel: true,
		LeaseDuration:   15 * time.Second,
		RenewDeadline:   10 * time.Second,
		RetryPeriod:     2 * time.Second,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: func(ctx context.Context) {
				log.Printf("[%s] Je deviens LEADER — je vais piloter les déploiements", identity)
				startWebhookServer(ctx, clientset, identity)
			},
			OnStoppedLeading: func() {
				log.Printf("[%s] Je perds le leadership", identity)
			},
			OnNewLeader: func(currentLeader string) {
				if currentLeader == identity {
					return
				}
				log.Printf("[%s] Nouveau leader observé : %s", identity, currentLeader)
			},
		},
	})
}

func startHealthzServer(identity string) {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	log.Printf("[%s] Serveur healthz permanent démarré sur :%s", identity, healthzPort)
	if err := http.ListenAndServe(":"+healthzPort, mux); err != nil {
		log.Fatalf("[%s] Erreur serveur healthz : %v", identity, err)
	}
}
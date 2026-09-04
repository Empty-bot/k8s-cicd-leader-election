package main

import (
	"context"
	"log"
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
)

var Version = "dev"

func main() {
	// Identité unique de cette instance (utile pour voir qui est leader en démo)
	nodeName := os.Getenv("NODE_NAME")
	if nodeName == "" {
		nodeName = "unknown-node"
	}
	identity := nodeName + "_" + string(uuid.NewUUID())[:8]
	log.Printf("[%s] Démarrage de l'orchestrateur (version %s)", identity, Version)
	// Config in-cluster : l'orchestrateur tourne comme pod dans le cluster
	// et s'authentifie via son ServiceAccount, pas via un kubeconfig externe.
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

	// Arrêt propre sur SIGTERM (important pour simuler une panne
	// contrôlée en démo, et pour le bon fonctionnement sous Kubernetes)
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
				runAsLeader(ctx, clientset)
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

// runAsLeader contiendra la logique métier (webhook GitHub + déploiement).
// Pour l'instant, on se contente de prouver que le leadership fonctionne.
func runAsLeader(ctx context.Context, clientset *kubernetes.Clientset) {
	<-ctx.Done()
}
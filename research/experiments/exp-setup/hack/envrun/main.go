package main

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"

	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

func main() {
	env := &envtest.Environment{
		CRDDirectoryPaths:     []string{"config/crd/bases"},
		ErrorIfCRDPathMissing: true,
		DownloadBinaryAssets:  true,
		BinaryAssetsDirectory: "/tmp/draft-envtest-bin",
	}
	if _, err := env.Start(); err != nil {
		panic(err)
	}
	_ = os.WriteFile("/tmp/draft-kc", env.KubeConfig, 0o600)
	fmt.Println("READY", filepath.Join("/tmp/draft-kc"))
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	<-c
	_ = env.Stop()
}

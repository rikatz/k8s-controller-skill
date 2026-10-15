package main

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
)

func main() {
	mk := func(n, lbl string) *corev1.ConfigMap {
		return &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: n, Namespace: "d", ResourceVersion: "1", Labels: map[string]string{"k": lbl}}}
	}
	var fw *watch.FakeWatcher = watch.NewFake()
	lw := &cache.ListWatch{
		ListWithContextFunc: func(ctx context.Context, o metav1.ListOptions) (runtime.Object, error) {
			return &corev1.ConfigMapList{ListMeta: metav1.ListMeta{ResourceVersion: "1"}, Items: []corev1.ConfigMap{*mk("ours", "y"), *mk("theirs", "n")}}, nil
		},
		WatchFuncWithContext: func(ctx context.Context, o metav1.ListOptions) (watch.Interface, error) {
			if o.SendInitialEvents != nil && *o.SendInitialEvents {
				w := watch.NewFakeWithOptions(watch.FakeOptions{ChannelSize: 10})
				w.Add(mk("ours", "y"))
				w.Add(mk("theirs", "n"))
				bm := mk("", "")
				bm.Name = ""
				bm.Annotations = map[string]string{metav1.InitialEventsAnnotationKey: "true"}
				bm.ResourceVersion = "1"
				w.Action(watch.Bookmark, bm)
				fw = w
				return w, nil
			}
			return fw, nil
		},
	}
	inf := cache.NewSharedIndexInformer(lw, &corev1.ConfigMap{}, 0, cache.Indexers{})
	inf.SetTransform(func(i any) (any, error) {
		cm, ok := i.(*corev1.ConfigMap)
		if !ok || cm.Labels["k"] != "y" {
			return nil, nil
		}
		return cm, nil
	})
	inf.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    func(o any) { fmt.Printf("ADD %T %v\n", o, o != nil) },
		UpdateFunc: func(a, b any) { fmt.Printf("UPD %v %v\n", a != nil, b != nil) },
		DeleteFunc: func(o any) { fmt.Printf("DEL %T\n", o) },
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go inf.RunWithContext(ctx)
	time.Sleep(1 * time.Second)
	fmt.Println("synced:", inf.HasSynced(), "store keys:", inf.GetStore().ListKeys())
	for _, k := range inf.GetStore().List() {
		fmt.Printf("store item %T nil=%v\n", k, k == nil)
	}
	// live events
	fw.Add(mk("theirs2", "n"))
	fw.Add(mk("ours2", "y"))
	// ours flips to not-ours
	fw.Modify(mk("ours", "n"))
	fw.Delete(mk("ours2", "y"))
	time.Sleep(500 * time.Millisecond)
	fmt.Println("after events keys:", inf.GetStore().ListKeys())
}

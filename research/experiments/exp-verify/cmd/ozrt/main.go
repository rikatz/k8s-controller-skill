package main

import (
	"encoding/json"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	kjson "sigs.k8s.io/json"

	"example.com/demo/api/oz"
)

func main() {
	sch := runtime.NewScheme()
	_ = oz.AddToScheme(sch)
	o := &oz.OZ{ObjectMeta: metav1.ObjectMeta{Name: "x"}}
	fmt.Println("--- encoding/json.Marshal")
	b, _ := json.Marshal(o)
	fmt.Println(string(b))
	fmt.Println("--- scheme codec (CodecFactory.LegacyCodec JSON)")
	cf := serializer.NewCodecFactory(sch)
	codec := cf.LegacyCodec(oz.GroupVersion)
	b, err := runtime.Encode(codec, o)
	fmt.Println(string(b), err)
	fmt.Println("--- sigs.k8s.io/json decode of '{\"spec\":{}}' and re-marshal")
	var back oz.OZ
	err = kjson.UnmarshalCaseSensitivePreserveInts([]byte(`{"metadata":{"name":"x"},"spec":{"time":null,"dur":"0s","nest":{}}}`), &back)
	fmt.Println(err)
	b, _ = json.Marshal(back)
	fmt.Println(string(b))
	fmt.Println("--- DefaultUnstructuredConverter.ToUnstructured")
	u, err := runtime.DefaultUnstructuredConverter.ToUnstructured(o)
	fmt.Println(u, err)
	fmt.Println("--- with non-zero nest / time / dur")
	o2 := &oz.OZ{Spec: oz.OZSpec{Nest: oz.Nested{A: "q"}, Dur: metav1.Duration{Duration: 1}, Time: metav1.Now()}}
	b, _ = runtime.Encode(codec, o2)
	fmt.Println(string(b))
}

func init() {
	o := &oz.OZ{Spec: oz.OZSpec{Slice: []string{}, Map: map[string]string{}, SliceOE: []string{}, MapOE: map[string]string{}}}
	b, _ := json.Marshal(o.Spec)
	fmt.Println("--- empty non-nil slice/map:", string(b))
}

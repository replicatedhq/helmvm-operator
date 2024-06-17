package k8sutil

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	runtimejson "k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/apimachinery/pkg/util/wait"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func EnsureObject(ctx context.Context, cli client.Client, obj client.Object, shouldDelete func(obj client.Object) bool) error {
	log := ctrl.LoggerFrom(ctx)

	key := client.ObjectKeyFromObject(obj)
	// make a copy of the object to avoid modifying the original object if we need to delete it
	copy := obj.DeepCopyObject().(client.Object)
	err := cli.Get(ctx, key, copy)
	if err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("get object: %w", err)
		}
	} else if shouldDelete(copy) {
		log.Info("Deleting previous object...", "object", key.String())
		err := cli.Delete(ctx, copy)
		if err != nil {
			return fmt.Errorf("delete object: %w", err)
		}
		err = wait.PollUntilContextCancel(ctx, 2*time.Second, true, func(ctx context.Context) (bool, error) {
			err := cli.Get(ctx, key, copy)
			if errors.IsNotFound(err) {
				return true, nil
			} else if err != nil {
				return false, fmt.Errorf("get object: %w", err)
			}
			return false, nil
		})
		if err != nil {
			return fmt.Errorf("wait for delete: %w", err)
		}
		log.Info("Deleted previous object", "object", key.String())
	} else {
		// copy the object into the original object since we had to use a copy above.
		// i could not find a better way to do this.
		err := deepCopyInto(cli.Scheme(), obj, copy)
		if err != nil {
			return fmt.Errorf("deep copy into: %w", err)
		}
		return nil
	}

	log.Info("Creating object...", "object", key.String())
	err = cli.Create(ctx, obj)
	if err != nil {
		return fmt.Errorf("create object: %w", err)
	}
	log.Info("Created object", "object", key.String())
	return nil
}

func deepCopyInto(scheme *runtime.Scheme, obj client.Object, copy client.Object) error {
	jsonSerializer := runtimejson.NewSerializer(runtimejson.DefaultMetaFactory, scheme, scheme, false)
	buf := bytes.NewBuffer(nil)
	err := jsonSerializer.Encode(copy, buf)
	if err != nil {
		return fmt.Errorf("encode: %w", err)
	}

	decode := serializer.NewCodecFactory(scheme).UniversalDeserializer().Decode
	_, _, err = decode(buf.Bytes(), nil, obj)
	if err != nil {
		return fmt.Errorf("decode: %w", err)
	}
	return nil
}

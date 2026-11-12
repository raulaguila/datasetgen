package password_test

import (
	"context"
	"testing"

	"github.com/raulaguila/dataset/pkg/password"
)

func BenchmarkHashParallel(b *testing.B) {
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := password.Hash(context.Background(), "correct horse battery"); err != nil {
				b.Error(err)
			}
		}
	})
}
